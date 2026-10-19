package weddingmedia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.etcd.io/bbolt"
)

func TestVideoHLSProcessorRemuxesExistingCopyAndReusesUploadedLayout(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe unavailable")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "classic.mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=size=96x64:rate=25", "-f", "lavfi", "-i", "sine=sample_rate=48000", "-t", "6", "-c:v", "libx264", "-threads", "1", "-profile:v", "main", "-level:v", "3.1", "-force_key_frames", "expr:gte(t,n_forced*2)", "-c:a", "aac", "-ac", "2", "-ar", "48000", "-b:a", "64k", "-movflags", "+faststart", source)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture %v %s", err, data)
	}
	classic, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	var uploaded []byte
	var uploadURL, version string
	uploads := 0
	meta := func(id string, size int, ver string) map[string]any {
		return map[string]any{"id": id, "size": fmt.Sprint(size), "mimeType": "video/mp4", "parents": []string{"private"}, "appProperties": map[string]string{"wedding_original_id": "original", "wedding_conversion_version": ver}}
	}
	s, server := testService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/drive/v3/files/original":
			if r.URL.Query().Get("alt") == "media" {
				t.Error("HLS preparation downloaded original instead of compatible copy")
				w.WriteHeader(500)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "original", "size": "1000", "mimeType": "video/quicktime", "parents": []string{"folder"}})
		case r.URL.Path == "/drive/v3/files/classic" && r.URL.Query().Get("alt") == "media":
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(classic)-1, len(classic)))
			w.Header().Set("Content-Length", fmt.Sprint(len(classic)))
			w.WriteHeader(206)
			_, _ = w.Write(classic)
		case r.URL.Path == "/drive/v3/files/classic":
			_ = json.NewEncoder(w).Encode(meta("classic", len(classic), videoConversionVersion))
		case r.URL.Path == "/drive/v3/files/hls-copy":
			_ = json.NewEncoder(w).Encode(meta("hls-copy", len(uploaded), version))
		case r.URL.Path == "/drive/v3/files":
			q := r.URL.Query().Get("q")
			files := []any{}
			if strings.Contains(q, "value='"+videoConversionVersion+"'") {
				files = append(files, meta("classic", len(classic), videoConversionVersion))
			} else if version != "" && strings.Contains(q, "value='"+version+"'") {
				files = append(files, meta("hls-copy", len(uploaded), version))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"files": files})
		case r.URL.Path == "/upload/drive/v3/files":
			var m struct {
				AppProperties map[string]string `json:"appProperties"`
			}
			_ = json.NewDecoder(r.Body).Decode(&m)
			version = m.AppProperties["wedding_conversion_version"]
			if version == videoConversionVersion {
				t.Error("HLS copy shares classic format tag")
			}
			uploads++
			w.Header().Set("Location", uploadURL)
			w.WriteHeader(200)
		case r.URL.Path == "/upload/hls":
			uploaded, _ = io.ReadAll(r.Body)
			_ = json.NewEncoder(w).Encode(meta("hls-copy", len(uploaded), version))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(500)
		}
	})
	uploadURL = server.URL + "/upload/hls"
	db, err := bbolt.Open(filepath.Join(dir, "jobs.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.db = db
	s.cfg.VideoProcessing = VideoProcessingConfig{FolderID: "private", WorkDir: filepath.Join(dir, "work"), FFmpegPath: ffmpeg, FFprobePath: ffprobe, SystemdRunPath: filepath.Join(dir, "systemd-run"), SystemctlPath: filepath.Join(dir, "systemctl")}
	if err = os.WriteFile(s.cfg.VideoProcessing.SystemdRunPath, []byte("#!/bin/sh\nwhile [ \"$1\" != \"--\" ]; do shift; done\nshift\nexec \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(s.cfg.VideoProcessing.SystemctlPath, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	job := videoJob{ID: "original", FolderID: "folder", Status: "processing", Attempts: 1}
	if err = db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(videoJobsBucket); err != nil {
			return err
		}
		return putVideoJob(tx, job)
	}); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		id, err := s.processVideoAndHLS(ctx, job)
		if err != nil || id != "classic" {
			t.Fatalf("result %s %v", id, err)
		}
		var stored videoJob
		if err = db.View(func(tx *bbolt.Tx) error { return decodeVideoJob(tx, job.ID, &stored) }); err != nil {
			t.Fatal(err)
		}
		if stored.HLS == nil || stored.HLS.ID != "hls-copy" {
			t.Fatalf("durable HLS copy missing: %+v", stored.HLS)
		}
		p, err := parseVideoHLS(stored.HLS.Playlist)
		if err != nil {
			t.Fatal(err)
		}
		last := p.Segments[len(p.Segments)-1]
		if last.Offset+last.Length != int64(len(uploaded)) {
			t.Fatal("playlist does not cover uploaded file")
		}
		work, err := os.ReadDir(s.cfg.VideoProcessing.WorkDir)
		if err != nil || len(work) != 0 {
			t.Fatalf("temporary video files retained: %v %v", work, err)
		}
	}
	if uploads != 1 {
		t.Fatalf("uploaded %d copies for the same layout", uploads)
	}
	hlsPath := filepath.Join(dir, "fragmented.mp4")
	if err = os.WriteFile(hlsPath, uploaded, 0600); err != nil {
		t.Fatal(err)
	}
	var hashes [][]byte
	for _, path := range []string{source, hlsPath} {
		out, err := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-i", path, "-map", "0", "-c", "copy", "-f", "streamhash", "-hash", "sha256", "-").CombinedOutput()
		if err != nil {
			t.Fatalf("stream hash: %v %s", err, out)
		}
		hashes = append(hashes, out)
	}
	if !bytes.Equal(hashes[0], hashes[1]) {
		t.Fatalf("HLS changed encoded video/audio: %s != %s", hashes[0], hashes[1])
	}
	if _, ok := s.hlsState("original"); ok {
		t.Fatal("unfinished job exposed HLS")
	}
	if err = s.finishVideoJob(job, "classic", nil, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	s.db, err = bbolt.Open(filepath.Join(dir, "jobs.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	hls, ok := s.hlsState("original")
	if !ok || hls.ID != "hls-copy" {
		t.Fatal("ready HLS not readable")
	}
	if bytes.Equal(uploaded, classic) {
		t.Fatal("HLS was not fragmented")
	}
}
