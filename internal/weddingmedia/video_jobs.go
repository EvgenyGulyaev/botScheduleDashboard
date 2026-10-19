package weddingmedia

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"go.etcd.io/bbolt"
)

type VideoProcessingConfig struct {
	Enabled        bool   `json:"enabled,omitempty"`
	FolderID       string `json:"folder_id,omitempty"`
	WorkDir        string `json:"work_dir,omitempty"`
	FFmpegPath     string `json:"ffmpeg_path,omitempty"`
	FFprobePath    string `json:"ffprobe_path,omitempty"`
	SystemdRunPath string `json:"systemd_run_path,omitempty"`
	SystemctlPath  string `json:"systemctl_path,omitempty"`
}

type videoJob struct {
	ID            string           `json:"id"`
	FolderID      string           `json:"album_folder_id"`
	Original      Media            `json:"original"`
	Status        string           `json:"status"`
	Attempts      int              `json:"attempts"`
	NextAttemptAt time.Time        `json:"next_attempt_at"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
	PreviewID     string           `json:"preview_id,omitempty"`
	HLS           *VideoHLSPreview `json:"hls,omitempty"`
	Unit          string           `json:"unit,omitempty"`
}

type videoProcessor func(context.Context, videoJob) (string, error)

var errVideoProcessingDeferred = errors.New("video processing deferred")
var videoJobsBucket = []byte("video_jobs")

func configureVideoProcessing(cfg *Config, path string) error {
	p := &cfg.VideoProcessing
	if !p.Enabled {
		return nil
	}
	if path == "" || !validID.MatchString(p.FolderID) || p.FolderID == cfg.FolderID {
		return errors.New("video processing requires persistent storage and a separate private Drive folder")
	}
	if p.WorkDir == "" {
		p.WorkDir = filepath.Join(filepath.Dir(path), "video-work")
	}
	if !filepath.IsAbs(p.WorkDir) || filepath.Clean(p.WorkDir) == string(filepath.Separator) {
		return errors.New("invalid video processing work directory")
	}
	defaults := []struct {
		value    *string
		fallback string
	}{{&p.FFmpegPath, "/usr/bin/ffmpeg"}, {&p.FFprobePath, "/usr/bin/ffprobe"}, {&p.SystemdRunPath, "/usr/bin/systemd-run"}, {&p.SystemctlPath, "/usr/bin/systemctl"}}
	for _, entry := range defaults {
		if *entry.value == "" {
			*entry.value = entry.fallback
		}
		if !filepath.IsAbs(*entry.value) {
			return errors.New("video processing tools require absolute paths")
		}
	}
	return nil
}

func (s *Service) enqueueVideoJob(tx *bbolt.Tx, u *upload) error {
	if !s.cfg.VideoProcessing.Enabled || !u.Complete || u.File == nil || !strings.HasPrefix(u.File.MIMEType, "video/") || s.cfg.VideoPreviews[u.File.ID] != "" {
		return nil
	}
	if !validID.MatchString(u.File.ID) {
		return errors.New("invalid completed video ID")
	}
	b, err := tx.CreateBucketIfNotExists(videoJobsBucket)
	if err != nil {
		return err
	}
	if b.Get([]byte(u.File.ID)) != nil {
		return nil
	}
	now := time.Now().UTC()
	job := videoJob{ID: u.File.ID, FolderID: s.cfg.FolderID, Original: *u.File, Status: "queued", CreatedAt: now, UpdatedAt: now}
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return b.Put([]byte(job.ID), data)
}

// previewStatuses reads a page of preparation states in one local transaction.
// It deliberately avoids Drive metadata requests and active media slots.
func (s *Service) previewStatuses(w http.ResponseWriter, r *http.Request) error {
	query := r.URL.Query()
	values := query["ids"]
	if len(values) != 1 || len(values[0]) > 4096 {
		return fail(400, "invalid video status IDs")
	}
	ids := strings.Split(values[0], ",")
	if len(ids) > 40 {
		return fail(400, "too many video status IDs")
	}
	for _, id := range ids {
		if !validID.MatchString(id) {
			return fail(400, "invalid video status ID")
		}
	}
	statuses := make(map[string]string)
	if s.db != nil {
		if err := s.db.View(func(tx *bbolt.Tx) error {
			b := tx.Bucket(videoJobsBucket)
			if b == nil {
				return nil
			}
			for _, id := range ids {
				data := b.Get([]byte(id))
				if data == nil {
					continue
				}
				var job videoJob
				if err := json.Unmarshal(data, &job); err != nil {
					return err
				}
				if job.FolderID != s.cfg.FolderID {
					continue
				}
				switch job.Status {
				case "queued", "processing", "ready", "failed":
					statuses[id] = job.Status
				}
			}
			return nil
		}); err != nil {
			return fail(503, "video status unavailable")
		}
	}
	jsonResponse(w, 200, map[string]any{"statuses": statuses})
	return nil
}

func (s *Service) previewState(id string) (string, string) {
	if preview := s.cfg.VideoPreviews[id]; preview != "" {
		return preview, "ready"
	}
	if s.db == nil {
		return "", ""
	}
	var job videoJob
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(videoJobsBucket)
		if b == nil || b.Get([]byte(id)) == nil {
			return nil
		}
		return json.Unmarshal(b.Get([]byte(id)), &job)
	})
	if err != nil {
		return "", ""
	}
	switch job.Status {
	case "ready":
		if validID.MatchString(job.PreviewID) {
			return job.PreviewID, "ready"
		}
	case "queued", "processing", "failed":
		return "", job.Status
	}
	return "", ""
}

func (s *Service) decorateMedia(m Media) Media {
	if strings.HasPrefix(m.MIMEType, "video/") {
		_, m.PreviewStatus = s.previewState(m.ID)
	}
	return m
}

func (s *Service) decorateGallery(page galleryPage) galleryPage {
	items := make([]Media, len(page.Items))
	for i, m := range page.Items {
		items[i] = s.decorateMedia(m)
	}
	page.Items = items
	return page
}

func (s *Service) uploadState(u *upload) UploadState {
	state := u.UploadState
	if state.File != nil {
		m := s.decorateMedia(*state.File)
		state.File = &m
	}
	return state
}

func (s *Service) setVideoJobUnit(id, unit string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(videoJobsBucket)
		if b == nil {
			return errors.New("video job unavailable")
		}
		var job videoJob
		if err := json.Unmarshal(b.Get([]byte(id)), &job); err != nil {
			return err
		}
		if job.Status != "processing" {
			return fmt.Errorf("video job is not processing")
		}
		job.Unit = unit
		job.UpdatedAt = time.Now().UTC()
		data, err := json.Marshal(job)
		if err != nil {
			return err
		}
		return b.Put([]byte(id), data)
	})
}
