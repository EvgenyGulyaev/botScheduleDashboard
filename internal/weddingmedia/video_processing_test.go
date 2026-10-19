package weddingmedia

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.etcd.io/bbolt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestVideoProcessRejectsPartialAndPersistsUnit(t *testing.T) {
	uploads := atomic.Int32{}
	s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/drive/v3/files/original" && r.URL.Query().Get("alt") == "media":
			w.Header().Set("Content-Range", "bytes 0-99/100")
			w.WriteHeader(206)
			_, _ = w.Write(make([]byte, 100))
		case r.URL.Path == "/drive/v3/files/original":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "original", "mimeType": "video/quicktime", "size": "100", "parents": []string{"folder"}})
		case r.URL.Path == "/drive/v3/files":
			_, _ = w.Write([]byte(`{"files":[]}`))
		default:
			uploads.Add(1)
			w.WriteHeader(500)
		}
	})
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "jobs.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.db = db
	job := videoJob{ID: "original", Status: "processing"}
	encoded, _ := json.Marshal(job)
	if err = db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(videoJobsBucket)
		if err != nil {
			return err
		}
		return b.Put([]byte(job.ID), encoded)
	}); err != nil {
		t.Fatal(err)
	}
	s.cfg.VideoProcessing.FolderID = "private"
	s.cfg.VideoProcessing.WorkDir = filepath.Join(t.TempDir(), "work")
	s.cfg.VideoProcessing.FFmpegPath = videoTestScript(t, "for last do :; done\nprintf output > \"$last\"\n")
	s.cfg.VideoProcessing.FFprobePath = videoTestScript(t, `for last do :; done
case "$last" in *output.mp4) duration=10;; *) duration=60;; esac
printf '{"format":{"duration":"%s"},"streams":[{"codec_type":"video","codec_name":"h264","profile":"Main","level":31,"pix_fmt":"yuv420p","width":320,"height":240,"avg_frame_rate":"30/1"}]}' "$duration"`)
	started := filepath.Join(t.TempDir(), "started")
	s.cfg.VideoProcessing.SystemdRunPath = videoTestScript(t, "printf '%s\\n' \"$@\" >> '"+started+"'\nwhile test \"$1\" != --; do shift; done\nshift\nexec \"$@\"\n")
	s.cfg.VideoProcessing.SystemctlPath = videoTestScript(t, "exit 0\n")
	if _, err = s.processVideo(context.Background(), job); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("accepted partial conversion: %v", err)
	}
	if _, err = os.Stat(started); err != nil {
		t.Fatal("unit never ran")
	}
	commands, err := os.ReadFile(started)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(commands), "--unit=wedding-video-") != 3 || strings.Count(string(commands), s.cfg.VideoProcessing.FFprobePath) != 2 || strings.Count(string(commands), "--property=RuntimeMaxSec=30\n") != 2 {
		t.Fatalf("production probes escaped tracked units: %s", commands)
	}
	if err = db.View(func(tx *bbolt.Tx) error {
		var stored videoJob
		if err := json.Unmarshal(tx.Bucket(videoJobsBucket).Get([]byte(job.ID)), &stored); err != nil {
			return err
		}
		if !validVideoUnit(stored.Unit) {
			t.Error("unit was not persisted", stored.Unit)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if uploads.Load() != 0 {
		t.Fatal("partial output uploaded")
	}
	if _, err = os.Stat(filepath.Join(s.cfg.VideoProcessing.WorkDir, job.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("temporary job files remain", err)
	}
	s.cfg.VideoProcessing.SystemdRunPath = videoTestScript(t, "exit 1\n")
	s.cfg.VideoProcessing.SystemctlPath = videoTestScript(t, "exit 1\n")
	if _, err = s.processVideo(context.Background(), job); !errors.Is(err, errVideoUnitStopFailed) {
		t.Fatalf("missing unit-stop failure: %v", err)
	}
	if _, err = os.Stat(filepath.Join(s.cfg.VideoProcessing.WorkDir, job.ID, "input")); err != nil {
		t.Fatal("removed work files while unit may still be active", err)
	}
}

func TestVideoStopAcceptsCollectedUnit(t *testing.T) {
	s := &Service{}
	s.cfg.VideoProcessing.SystemctlPath = videoTestScript(t, "if test \"$1\" = stop; then exit 5; fi\nprintf 'not-found\\n'\n")
	if err := s.stopVideoUnit(context.Background(), "wedding-video-0123456789abcdef0123456789abcdef.service"); err != nil {
		t.Fatal("collected unit prevented recovery", err)
	}
}

func videoTestScript(t *testing.T, source string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "command")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+source), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVideoUnitLimitsAndCancellation(t *testing.T) {
	dir := t.TempDir()
	started := filepath.Join(dir, "started")
	stopped := filepath.Join(dir, "stopped")
	s := &Service{}
	s.cfg.VideoProcessing.FFmpegPath = "/usr/bin/ffmpeg"
	s.cfg.VideoProcessing.SystemdRunPath = videoTestScript(t, "printf '%s\\n' \"$@\" > '"+started+".tmp'\nmv '"+started+".tmp' '"+started+"'\nexec sleep 30\n")
	s.cfg.VideoProcessing.SystemctlPath = videoTestScript(t, "printf '%s\\n' \"$@\" > '"+stopped+"'\n")
	unit := "wedding-video-0123456789abcdef0123456789abcdef.service"
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.runVideoUnit(ctx, unit, []string{"-i", "input", "output"}) }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("unit not started")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel error %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("unit not canceled")
	}
	args, err := os.ReadFile(started)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--wait", "--collect", "--service-type=exec", "CPUQuota=50%", "MemoryMax=192M", "MemorySwapMax=128M", "Nice=15", "TasksMax=32", "RuntimeMaxSec=14400", "KillMode=control-group", unit, "/usr/bin/ffmpeg"} {
		if !strings.Contains(string(args), want) {
			t.Error("missing unit limit", want)
		}
	}
	stop, err := os.ReadFile(stopped)
	if err != nil || string(stop) != "stop\n"+unit+"\n" {
		t.Fatalf("unit not stopped: %q %v", stop, err)
	}
	if err := s.stopVideoUnit(context.Background(), "unrelated.service"); err == nil {
		t.Fatal("accepted unowned unit")
	}
}

func TestVideoProbeBounds(t *testing.T) {
	s := &Service{}
	s.cfg.VideoProcessing.FFprobePath = videoTestScript(t, "printf 'not json'\n")
	if _, err := s.probeVideo(context.Background(), "input"); err == nil {
		t.Fatal("accepted malformed probe")
	}
	s.cfg.VideoProcessing.FFprobePath = videoTestScript(t, "exec head -c 70000 /dev/zero\n")
	if _, err := s.probeVideo(context.Background(), "input"); err == nil {
		t.Fatal("accepted excessive probe")
	}
}

func TestVideoHDRDownscalesBeforeFloat(t *testing.T) {
	for _, transfer := range []string{"smpte2084", "arib-std-b67"} {
		args := videoFFmpegArgs("input", "output", videoProbe{Streams: []videoStream{{CodecType: "video", ColorTransfer: transfer, Width: 3840, Height: 2160, PixFmt: "yuv420p10le"}}})
		filter := ""
		for i, arg := range args {
			if arg == "-vf" {
				filter = args[i+1]
			}
		}
		positions := []int{strings.Index(filter, "scale=w="), strings.Index(filter, "format=yuv420p10le"), strings.Index(filter, "format=gbrpf32le"), strings.Index(filter, "tonemap="), strings.LastIndex(filter, "format=yuv420p")}
		for i, pos := range positions {
			if pos < 0 || (i > 0 && pos <= positions[i-1]) {
				t.Fatalf("%s allocates float or reduces depth before bounded HDR resize: %s", transfer, filter)
			}
		}
	}
}

func TestVideoUnknownTransferDoesNotForceToneMapping(t *testing.T) {
	args := videoFFmpegArgs("input", "output", videoProbe{Streams: []videoStream{{CodecType: "video", Width: 3840, Height: 2160, PixFmt: "yuv420p10le"}}})
	if strings.Contains(strings.Join(args, " "), "tonemap=") {
		t.Fatal("10-bit content without transfer metadata was forced through an HDR colorspace conversion")
	}
}

func TestVideoTrackedProbeLimitsAndOutput(t *testing.T) {
	dir := t.TempDir()
	captured := filepath.Join(dir, "args")
	stopped := filepath.Join(dir, "stopped")
	s := &Service{}
	s.cfg.VideoProcessing.FFprobePath = "/usr/bin/ffprobe"
	s.cfg.VideoProcessing.SystemdRunPath = videoTestScript(t, "printf '%s\\n' \"$@\" > '"+captured+"'\nprintf '{\"format\":{\"duration\":\"1\"},\"streams\":[]}'\n")
	s.cfg.VideoProcessing.SystemctlPath = videoTestScript(t, "printf stopped > '"+stopped+"'\n")
	unit := "wedding-video-0123456789abcdef0123456789abcdef.service"
	probe, err := s.probeVideo(context.Background(), "input", unit)
	if err != nil || probe.Format.Duration != "1" {
		t.Fatalf("bounded probe: %+v %v", probe, err)
	}
	args, err := os.ReadFile(captured)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CPUQuota=50%", "MemoryMax=192M", "MemorySwapMax=128M", "Nice=15", "TasksMax=32", "RuntimeMaxSec=30", "KillMode=control-group", "--unit=" + unit, "/usr/bin/ffprobe\n-threads\n1\n"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("missing tracked probe constraint %s: %s", want, args)
		}
	}
	s.cfg.VideoProcessing.SystemdRunPath = videoTestScript(t, "exec head -c 70000 /dev/zero\n")
	if _, err = s.probeVideo(context.Background(), "input", unit); err == nil {
		t.Fatal("accepted unbounded tracked stdout")
	}
	if _, err = os.Stat(stopped); err != nil {
		t.Fatal("failed probe did not stop its unit", err)
	}
}

func TestVideoTrackedProbeCancellationAndOrphan(t *testing.T) {
	for _, stopFails := range []bool{false, true} {
		t.Run(fmt.Sprint(stopFails), func(t *testing.T) {
			dir := t.TempDir()
			started := filepath.Join(dir, "started")
			stopped := filepath.Join(dir, "stopped")
			s := &Service{}
			s.cfg.VideoProcessing.FFprobePath = "/usr/bin/ffprobe"
			s.cfg.VideoProcessing.SystemdRunPath = videoTestScript(t, "printf started > '"+started+".tmp'\nmv '"+started+".tmp' '"+started+"'\nexec sleep 30\n")
			exit := "0"
			if stopFails {
				exit = "1"
			}
			s.cfg.VideoProcessing.SystemctlPath = videoTestScript(t, "printf stopped > '"+stopped+"'\nexit "+exit+"\n")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := s.probeVideo(ctx, "input", "wedding-video-0123456789abcdef0123456789abcdef.service")
				done <- err
			}()
			deadline := time.Now().Add(2 * time.Second)
			for {
				if _, err := os.Stat(started); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("probe unit did not start")
				}
				time.Sleep(time.Millisecond)
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation lost: %v", err)
				}
				if errors.Is(err, errVideoUnitStopFailed) != stopFails {
					t.Fatalf("incorrect orphan marker: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("probe unit did not stop")
			}
			if _, err := os.Stat(stopped); err != nil {
				t.Fatal("probe canceled without stopping unit")
			}
		})
	}
}

func TestVideoFFmpegRealDimensions(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe unavailable")
	}
	s := &Service{}
	s.cfg.VideoProcessing.FFprobePath = ffprobe
	for _, size := range []string{"1080x1920", "320x240"} {
		t.Run(size, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "input.mp4")
			output := filepath.Join(dir, "output.mp4")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			fixture := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=size="+size+":rate=60", "-t", "0.5", "-c:v", "libx264", "-threads", "1", input)
			if data, err := fixture.CombinedOutput(); err != nil {
				t.Fatalf("fixture: %v %s", err, data)
			}
			in, err := s.probeVideo(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(ctx, ffmpeg, videoFFmpegArgs(input, output, in)...)
			if data, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("encode: %v %s", err, data)
			}
			out, err := s.probeVideo(ctx, output)
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(output)
			if err != nil {
				t.Fatal(err)
			}
			if err = validateVideoOutput(in, out, info.Size()); err != nil {
				t.Fatal(err)
			}
			w, h := out.Streams[0].Width, out.Streams[0].Height
			if size == "1080x1920" && (w != 720 || h != 1280) {
				t.Fatalf("portrait %dx%d", w, h)
			}
			if size == "320x240" && (w != 320 || h != 240) {
				t.Fatalf("upscaled %dx%d", w, h)
			}
		})
	}
}

func TestVideoDownloadRanges(t *testing.T) {
	data := make([]byte, 2*int(ChunkSize)+117)
	for i := range data {
		data[i] = byte(i % 251)
	}
	var active, peak atomic.Int32
	var mu sync.Mutex
	ranges := map[string]bool{}
	s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(15 * time.Millisecond)
		var start, end int64
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		ranges[r.Header.Get("Range")] = true
		mu.Unlock()
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
		w.WriteHeader(206)
		_, _ = w.Write(data[start : end+1])
	})
	f, err := os.CreateTemp(t.TempDir(), "input")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err = s.downloadVideo(context.Background(), "original", int64(len(data)), f); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(f.Name())
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("download bytes differ", err)
	}
	if peak.Load() != 2 {
		t.Fatalf("parallelism %d", peak.Load())
	}
	for _, r := range []string{"bytes=0-8388607", "bytes=8388608-16777215", "bytes=16777216-16777332"} {
		if !ranges[r] {
			t.Errorf("missing %s", r)
		}
	}
}

func TestVideoDownloadRejectsShortAndCancelsPeer(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") == "bytes=0-8388607" {
			<-started
			w.Header().Set("Content-Range", "bytes 0-8388607/16777216")
			w.WriteHeader(206)
			_, _ = w.Write([]byte("short"))
			return
		}
		close(started)
		<-r.Context().Done()
		close(canceled)
	})
	f, err := os.CreateTemp(t.TempDir(), "input")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = s.downloadVideo(ctx, "original", 2*ChunkSize, f); err == nil {
		t.Fatal("accepted short source")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("peer request not canceled")
	}
}

func TestVideoDiskDeferral(t *testing.T) {
	if err := checkVideoSpace(100, 100+videoMaxOutput+videoDiskReserve-1); !errors.Is(err, errVideoProcessingDeferred) {
		t.Fatalf("not deferred: %v", err)
	}
	if err := checkVideoSpace(100, 100+videoMaxOutput+videoDiskReserve); err != nil {
		t.Fatal(err)
	}
}

func TestVideoFFmpegArguments(t *testing.T) {
	for _, transfer := range []string{"bt709", "smpte2084", "arib-std-b67"} {
		args := strings.Join(videoFFmpegArgs("/private/input", "/private/output", videoProbe{Streams: []videoStream{{CodecType: "video", ColorTransfer: transfer}}}), " ")
		for _, want := range []string{"-preset veryfast", "-profile:v main", "-level:v 3.1", "-pix_fmt yuv420p", "-threads 1", "-filter_threads 1", "-maxrate 2500k", "-bufsize 5000k", "-movflags +faststart", "-fs 536870912", "-ac 2", "-ar 48000", "-b:a 128k", "min(1,", "1280/max(iw,ih)", "720/min(iw,ih)", "fps=fps='min(source_fps,30)'"} {
			if !strings.Contains(args, want) {
				t.Errorf("missing %s in %s", want, args)
			}
		}
		if strings.Contains(args, "tonemap=") != (transfer != "bt709") {
			t.Errorf("wrong HDR path: %s", args)
		}
	}
}

func TestVideoOutputValidation(t *testing.T) {
	input := videoProbe{Format: videoFormat{Duration: "60"}, Streams: []videoStream{{CodecType: "video"}, {CodecType: "audio"}}}
	output := videoProbe{Format: videoFormat{Duration: "60"}, Streams: []videoStream{{CodecType: "video", CodecName: "h264", Profile: "Main", Level: 31, PixFmt: "yuv420p", Width: 720, Height: 1280, AvgFrameRate: "30/1"}, {CodecType: "audio", CodecName: "aac", Profile: "LC", Channels: 2, SampleRate: "48000"}}}
	if err := validateVideoOutput(input, output, 100); err != nil {
		t.Fatal(err)
	}
	output.Format.Duration = "30"
	if validateVideoOutput(input, output, 100) == nil {
		t.Fatal("accepted partial output")
	}
	output.Format.Duration = "60"
	output.Streams[0].Width = 1280
	if validateVideoOutput(input, output, 100) == nil {
		t.Fatal("accepted oversized portrait")
	}
	output.Streams[0].Width = 720
	for _, size := range []int64{0, videoMaxOutput + 1} {
		if validateVideoOutput(input, output, size) == nil {
			t.Fatal("accepted invalid size")
		}
	}
	output.Format.Duration = "NaN"
	if validateVideoOutput(input, output, 100) == nil {
		t.Fatal("accepted NaN")
	}
}

func TestVideoOutputRejectsUpscaling(t *testing.T) {
	in := videoProbe{Format: videoFormat{Duration: "1"}, Streams: []videoStream{{CodecType: "video", Width: 320, Height: 240}}}
	out := videoProbe{Format: videoFormat{Duration: "1"}, Streams: []videoStream{{CodecType: "video", CodecName: "h264", Profile: "Main", Level: 31, PixFmt: "yuv420p", Width: 640, Height: 480, AvgFrameRate: "30/1"}}}
	if validateVideoOutput(in, out, 100) == nil {
		t.Fatal("accepted enlarged output")
	}
}

func TestVideoRealHDR(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe unavailable")
	}
	filters, err := exec.Command(ffmpeg, "-hide_banner", "-filters").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(filters), " zscale ") {
		t.Skip("FFmpeg build lacks zscale; production requires zscale and tonemap")
	}
	s := &Service{}
	s.cfg.VideoProcessing.FFprobePath = ffprobe
	for _, transfer := range []string{"smpte2084", "arib-std-b67"} {
		t.Run(transfer, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "input.mp4")
			output := filepath.Join(dir, "output.mp4")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			fixture := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=size=3840x2160:rate=30", "-t", "0.5", "-vf", "format=yuv420p10le", "-c:v", "libx265", "-preset", "ultrafast", "-threads", "1", "-x265-params", "pools=none:frame-threads=1:wpp=0:colorprim=bt2020:transfer="+transfer+":colormatrix=bt2020nc:log-level=error", "-pix_fmt", "yuv420p10le", input)
			if data, err := fixture.CombinedOutput(); err != nil {
				t.Fatalf("fixture: %v %s", err, data)
			}
			in, err := s.probeVideo(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			if in.Streams[0].ColorTransfer != transfer {
				t.Fatalf("HDR fixture lost transfer: %+v", in)
			}
			cmd := exec.CommandContext(ctx, ffmpeg, videoFFmpegArgs(input, output, in)...)
			if data, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("HDR encode: %v %s", err, data)
			}
			out, err := s.probeVideo(ctx, output)
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(output)
			if err != nil {
				t.Fatal(err)
			}
			if err = validateVideoOutput(in, out, info.Size()); err != nil {
				t.Fatal(err)
			}
			if out.Streams[0].ColorTransfer != "bt709" {
				t.Fatal("output not SDR", out.Streams[0])
			}
			if out.Streams[0].Width != 1280 || out.Streams[0].Height != 720 {
				t.Fatalf("4K HDR dimensions not bounded: %+v", out.Streams[0])
			}
		})
	}
}

func TestVideoCleanupPreservesUnknownFiles(t *testing.T) {
	s := &Service{}
	s.cfg.VideoProcessing.WorkDir = t.TempDir()
	if err := os.Chmod(s.cfg.VideoProcessing.WorkDir, 0700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(s.cfg.VideoProcessing.WorkDir, "original")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"input", "output.mp4", "unowned"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.cleanupVideoJob(videoJob{ID: "original"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "unowned")); err != nil {
		t.Fatal("removed unowned file")
	}
	if err := s.cleanupVideoJob(videoJob{ID: "../escape"}); err == nil {
		t.Fatal("accepted unsafe id")
	}
}
