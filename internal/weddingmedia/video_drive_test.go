package weddingmedia

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func previewMetadata(id string, size int64) map[string]any {
	return map[string]any{"id": id, "mimeType": "video/mp4", "size": fmt.Sprint(size), "parents": []string{"private"}, "appProperties": map[string]string{"wedding_original_id": "original", "wedding_conversion_version": videoConversionVersion}}
}

func TestVideoReadyCopyReuse(t *testing.T) {
	s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/drive/v3/files/original":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "original", "mimeType": "video/quicktime", "size": "123", "parents": []string{"folder"}})
		case "/drive/v3/files":
			q := r.URL.Query().Get("q")
			for _, want := range []string{"'private' in parents", "trashed = false", "wedding_original_id", videoConversionVersion} {
				if !strings.Contains(q, want) {
					t.Error("missing copy query constraint", want)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"files": []any{previewMetadata("ready", 456)}})
		case "/drive/v3/files/ready":
			_ = json.NewEncoder(w).Encode(previewMetadata("ready", 456))
		default:
			t.Error("unexpected request", r.URL.Path)
			w.WriteHeader(500)
		}
	})
	s.cfg.VideoProcessing.FolderID = "private"
	id, err := s.processVideo(context.Background(), videoJob{ID: "original"})
	if err != nil || id != "ready" {
		t.Fatalf("%s %v", id, err)
	}
}

func TestVideoUploadChunksRecoverAcceptedBytes(t *testing.T) {
	size := ChunkSize + 321
	data := make([]byte, int(size))
	for i := range data {
		data[i] = byte(i % 251)
	}
	path := filepath.Join(t.TempDir(), "output.mp4")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	offset := int64(0)
	seen := []string{}
	lost := false
	var uploadURL string
	s, server := testService(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/upload/drive/v3/files":
			var meta map[string]any
			_ = json.NewDecoder(r.Body).Decode(&meta)
			if fmt.Sprint(meta["parents"]) != "[private]" {
				t.Error("wrong parents", meta)
			}
			if r.Header.Get("X-Upload-Content-Length") != fmt.Sprint(size) {
				t.Error("unknown total")
			}
			w.Header().Set("Location", uploadURL)
			w.WriteHeader(200)
		case "/upload/session":
			mu.Lock()
			defer mu.Unlock()
			cr := r.Header.Get("Content-Range")
			seen = append(seen, cr)
			if strings.HasPrefix(cr, "bytes */") {
				w.Header().Set("Range", fmt.Sprintf("bytes=0-%d", offset-1))
				w.WriteHeader(308)
				return
			}
			var start, end, total int64
			if _, err := fmt.Sscanf(cr, "bytes %d-%d/%d", &start, &end, &total); err != nil || start != offset || total != size {
				t.Error("bad range", cr, offset)
			}
			got, _ := io.ReadAll(r.Body)
			if string(got) != string(data[start:end+1]) {
				t.Error("upload bytes changed")
			}
			offset = end + 1
			if !lost {
				lost = true
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				_ = conn.Close()
				return
			}
			_ = json.NewEncoder(w).Encode(previewMetadata("ready", size))
		case "/drive/v3/files/ready":
			_ = json.NewEncoder(w).Encode(previewMetadata("ready", size))
		default:
			t.Error("unexpected request", r.URL.Path)
			w.WriteHeader(500)
		}
	})
	uploadURL = server.URL + "/upload/session"
	s.cfg.VideoProcessing.FolderID = "private"
	id, err := s.uploadVideoCopy(context.Background(), "original", path, size, videoConversionVersion)
	if err != nil || id != "ready" {
		t.Fatalf("%s %v", id, err)
	}
	want := []string{"bytes 0-8388607/8388929", "bytes */8388929", "bytes 8388608-8388928/8388929"}
	if fmt.Sprint(seen) != fmt.Sprint(want) {
		t.Fatalf("ranges %v", seen)
	}
}

func TestVideoCopyValidationRejectsAlbumAndBadCompletion(t *testing.T) {
	for _, mode := range []string{"album", "size", "id", "tags", "mime", "trashed"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
				m := previewMetadata("ready", 100)
				switch mode {
				case "album":
					m["parents"] = []string{"private", "folder"}
				case "size":
					m["size"] = "0"
				case "id":
					m["id"] = "../bad"
				case "tags":
					m["appProperties"] = map[string]string{}
				case "mime":
					m["mimeType"] = "video/quicktime"
				case "trashed":
					m["trashed"] = true
				}
				_ = json.NewEncoder(w).Encode(m)
			})
			s.cfg.VideoProcessing.FolderID = "private"
			if _, err := s.confirmVideoCopy(context.Background(), "original", "ready", 100, videoConversionVersion); err == nil {
				t.Fatal("accepted invalid copy")
			}
		})
	}
}

func TestVideoUploadRejectsInvalidProgressAndEarlyCompletion(t *testing.T) {
	for _, mode := range []string{"ahead", "early", "stalled", "location"} {
		t.Run(mode, func(t *testing.T) {
			size := ChunkSize + 1
			path := filepath.Join(t.TempDir(), "output.mp4")
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = f.Truncate(size); err != nil {
				t.Fatal(err)
			}
			_ = f.Close()
			var session string
			requests := 0
			s, server := testService(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/upload/drive/v3/files":
					if mode == "location" {
						w.Header().Set("Location", "https://untrusted.invalid/upload/session")
					} else {
						w.Header().Set("Location", session)
					}
				case "/upload/session":
					requests++
					_, _ = io.Copy(io.Discard, r.Body)
					switch mode {
					case "ahead":
						w.Header().Set("Range", fmt.Sprintf("bytes=0-%d", size-1))
						w.WriteHeader(308)
					case "early":
						_ = json.NewEncoder(w).Encode(previewMetadata("ready", size))
					case "stalled":
						w.WriteHeader(308)
					default:
						t.Error("invalid upload endpoint was used")
					}
				case "/drive/v3/files/ready":
					_ = json.NewEncoder(w).Encode(previewMetadata("ready", size))
				default:
					t.Error("unexpected request", r.URL.Path)
					w.WriteHeader(500)
				}
			})
			session = server.URL + "/upload/session"
			s.cfg.VideoProcessing.FolderID = "private"
			if _, err = s.uploadVideoCopy(context.Background(), "original", path, size, videoConversionVersion); err == nil {
				t.Fatal("accepted invalid upload", mode)
			}
			if requests > 4 {
				t.Fatal("unbounded retries", requests)
			}
		})
	}
}
