package weddingmedia

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.etcd.io/bbolt"
)

func persistentVideoService(t *testing.T) *Service {
	t.Helper()
	base, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("alt") == "media" {
			w.Write([]byte("content"))
			return
		}
		w.Write([]byte(`{"id":"video","name":"video.mov","mimeType":"video/quicktime","size":"100","parents":["folder"]}`))
	})
	cfg := base.cfg
	workDir := t.TempDir()
	if err := os.Chmod(workDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"video_processing":{"enabled":true,"folder_id":"views","work_dir":"`+workDir+`"}}`), &cfg); err != nil {
		t.Fatal(err)
	}
	s, err := newService(cfg, filepath.Join(t.TempDir(), "config.json"), base.client, base.baseURL, base.tokenURL)
	if err != nil {
		t.Fatal(err)
	}
	s.accessToken = base.accessToken
	s.tokenExpiry = base.tokenExpiry
	t.Cleanup(func() { s.Close() })
	return s
}

func TestVideoJobCompletionIsDurableAndDeduplicated(t *testing.T) {
	s := persistentVideoService(t)
	u := &upload{UploadState: UploadState{ID: "upload", Complete: true, File: &Media{ID: "video", Name: "video.mov", MIMEType: "video/quicktime", Size: 100}}, FolderID: "folder"}
	for range 2 {
		if err := s.persist(u); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("video_jobs"))
		if b == nil {
			t.Fatal("completed video did not create a durable job")
		}
		if b.Stats().KeyN != 1 {
			t.Fatalf("duplicate jobs: %d", b.Stats().KeyN)
		}
		var job map[string]any
		if err := json.Unmarshal(b.Get([]byte("video")), &job); err != nil {
			return err
		}
		if job["status"] != "queued" {
			t.Fatalf("job not queued: %v", job)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	response := request(s, "GET", "/wedding/media/video", "", guest(t, s))
	if !strings.Contains(response.Body.String(), `"preview_status":"queued"`) {
		t.Fatalf("missing preparation status: %s", response.Body.String())
	}
}

func TestVideoJobSkipsPhotosAndIncompleteUploads(t *testing.T) {
	s := persistentVideoService(t)
	for _, u := range []*upload{
		{UploadState: UploadState{ID: "photo-upload", Complete: true, File: &Media{ID: "photo", MIMEType: "image/jpeg"}}},
		{UploadState: UploadState{ID: "unfinished", File: &Media{ID: "video", MIMEType: "video/mp4"}}},
	} {
		if err := s.persist(u); err != nil {
			t.Fatal(err)
		}
	}
	s.db.View(func(tx *bbolt.Tx) error {
		if b := tx.Bucket([]byte("video_jobs")); b != nil && b.Stats().KeyN != 0 {
			t.Fatal("queued a photo or incomplete upload")
		}
		return nil
	})
}

func TestVideoJobRejectsInvalidProcessingConfiguration(t *testing.T) {
	for _, data := range []string{
		`{"video_processing":{"enabled":true,"folder_id":"../views","work_dir":"/tmp/views"}}`,
		`{"video_processing":{"enabled":true,"folder_id":"views","work_dir":"/"}}`,
		`{"video_processing":{"enabled":true,"folder_id":"views","work_dir":"relative"}}`,
	} {
		var cfg Config
		if err := json.Unmarshal([]byte(data), &cfg); err != nil {
			t.Fatal(err)
		}
		s, err := newService(cfg, filepath.Join(t.TempDir(), "config"), nil, "https://www.googleapis.com", "https://oauth2.googleapis.com/token")
		if err == nil {
			s.Close()
			t.Errorf("accepted invalid video processing configuration: %s", data)
		}
	}
}

func TestVideoJobCachedGalleryReflectsStatusChanges(t *testing.T) {
	s := persistentVideoService(t)
	queueTestVideo(t, s, "video")
	s.listCache[""] = cachedGallery{Page: galleryPage{Items: []Media{{ID: "video", MIMEType: "video/mp4"}}}, Expires: time.Now().Add(time.Hour)}
	c := guest(t, s)
	if w := request(s, "GET", "/wedding/media", "", c); !strings.Contains(w.Body.String(), `"preview_status":"queued"`) {
		t.Fatal("cached list hides queued status")
	}
	job, _ := s.claimVideoJob(time.Now())
	if err := s.finishVideoJob(*job, "ready-video", nil, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	if w := request(s, "GET", "/wedding/media", "", c); !strings.Contains(w.Body.String(), `"preview_status":"ready"`) {
		t.Fatal("cached list hides ready status")
	}
	if s.listCache[""].Page.Items[0].PreviewStatus != "" {
		t.Fatal("mutated shared cached metadata")
	}
}

func TestVideoPreviewStatusesBatch(t *testing.T) {
	s := persistentVideoService(t)
	queueTestVideo(t, s, "video")
	c := guest(t, s)
	// A full media semaphore must not block DB-only status polling.
	for range cap(s.inFlight) {
		s.inFlight <- struct{}{}
	}
	w := request(s, "GET", "/wedding/media/previews?ids=video,missing,video", "", c)
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"statuses":{"video":"queued"}}` {
		t.Fatalf("batch response: %d %s", w.Code, w.Body.String())
	}
	for range cap(s.inFlight) {
		<-s.inFlight
	}
	s.Settings = func() (bool, string, error) { return true, "protected", nil }
	if w := request(s, "GET", "/wedding/media/previews?ids=video", "", nil); w.Code != 401 {
		t.Fatalf("unauthorized status: %d", w.Code)
	}
	s.Settings = nil
	for _, query := range []string{"", "ids=bad!", "ids=" + strings.Repeat("video,", 40) + "video"} {
		if w := request(s, "GET", "/wedding/media/previews?"+query, "", c); w.Code != 400 {
			t.Errorf("invalid query accepted: %q status=%d", query, w.Code)
		}
	}
	s.cfg.FolderID = "other-album"
	w = request(s, "GET", "/wedding/media/previews?ids=video", "", c)
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"statuses":{}}` {
		t.Fatalf("leaked previous album status: %d %s", w.Code, w.Body.String())
	}
}

func TestVideoJobReadyCopyKeepsOriginalDownload(t *testing.T) {
	s := persistentVideoService(t)
	queueTestVideo(t, s, "video")
	job, err := s.claimVideoJob(time.Now())
	if err != nil || job == nil {
		t.Fatalf("claim: %v", err)
	}
	if err = s.finishVideoJob(*job, "ready-video", nil, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("alt") == "media" {
			w.Write([]byte(r.URL.Path))
			return
		}
		w.Write([]byte(`{"id":"video","name":"clip.mov","mimeType":"video/quicktime","parents":["folder"],"size":"100"}`))
	}))
	defer upstream.Close()
	s.baseURL = upstream.URL
	c := guest(t, s)
	for _, tc := range []struct{ query, target string }{{"", "ready-video"}, {"?download=1", "video"}} {
		w := request(s, "GET", "/wedding/media/video/content"+tc.query, "", c)
		if w.Code != 200 || w.Body.String() != "/drive/v3/files/"+tc.target {
			t.Fatalf("content %s: %d %s", tc.query, w.Code, w.Body.String())
		}
	}
}
