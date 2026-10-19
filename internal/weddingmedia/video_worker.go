package weddingmedia

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"go.etcd.io/bbolt"
)

func decodeVideoJob(tx *bbolt.Tx, id string, job *videoJob) error {
	b := tx.Bucket(videoJobsBucket)
	if b == nil || b.Get([]byte(id)) == nil {
		return errors.New("video job unavailable")
	}
	return json.Unmarshal(b.Get([]byte(id)), job)
}
func putVideoJob(tx *bbolt.Tx, job videoJob) error {
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return tx.Bucket(videoJobsBucket).Put([]byte(job.ID), data)
}

func (s *Service) claimVideoJob(now time.Time) (*videoJob, error) {
	var selected *videoJob
	err := s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(videoJobsBucket)
		if b == nil {
			return nil
		}
		if err := b.ForEach(func(_, data []byte) error {
			var job videoJob
			if err := json.Unmarshal(data, &job); err != nil {
				return err
			}
			if job.Status != "queued" || job.NextAttemptAt.After(now) {
				return nil
			}
			if selected == nil || job.CreatedAt.Before(selected.CreatedAt) {
				copy := job
				selected = &copy
			}
			return nil
		}); err != nil {
			return err
		}
		if selected == nil {
			return nil
		}
		if preview := s.cfg.VideoPreviews[selected.ID]; preview != "" {
			selected.Status = "ready"
			selected.PreviewID = preview
			selected.UpdatedAt = now
			if err := putVideoJob(tx, *selected); err != nil {
				return err
			}
			selected = nil
			return nil
		}
		selected.Status = "processing"
		selected.Attempts++
		selected.Unit = ""
		selected.UpdatedAt = now
		return putVideoJob(tx, *selected)
	})
	return selected, err
}

func (s *Service) finishVideoJob(job videoJob, preview string, processErr error, cancelled bool, now time.Time) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		var current videoJob
		if err := decodeVideoJob(tx, job.ID, &current); err != nil {
			return err
		}
		if current.Status != "processing" || current.Attempts != job.Attempts {
			return errors.New("obsolete video job result")
		}
		current.Unit = ""
		current.UpdatedAt = now
		current.NextAttemptAt = time.Time{}
		switch {
		case cancelled:
			current.Status = "queued"
			current.Attempts = max(0, current.Attempts-1)
		case errors.Is(processErr, errVideoProcessingDeferred):
			current.Status = "queued"
			current.Attempts = max(0, current.Attempts-1)
			current.NextAttemptAt = now.Add(time.Minute)
		case processErr == nil && validID.MatchString(preview) && preview != job.ID:
			current.Status = "ready"
			current.PreviewID = preview
		case current.Attempts >= 3:
			current.Status = "failed"
		default:
			current.Status = "queued"
			delay := time.Minute
			if current.Attempts > 1 {
				delay = 5 * time.Minute
			}
			current.NextAttemptAt = now.Add(delay)
		}
		return putVideoJob(tx, current)
	})
}

func (s *Service) recoverVideoJobs(ctx context.Context) error {
	var interrupted []videoJob
	if err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(videoJobsBucket)
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, data []byte) error {
			var job videoJob
			if err := json.Unmarshal(data, &job); err != nil {
				return err
			}
			if job.Status == "processing" {
				interrupted = append(interrupted, job)
			}
			return nil
		})
	}); err != nil {
		return err
	}
	for _, job := range interrupted {
		if err := ctx.Err(); err != nil {
			return err
		}
		if job.Unit != "" {
			if err := s.stopVideoUnit(ctx, job.Unit); err != nil {
				return err
			}
		}
		if err := s.cleanupVideoJob(job); err != nil {
			return err
		}
		if err := s.finishVideoJob(job, "", nil, true, time.Now().UTC()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) StartVideoWorker() { s.startVideoWorkerWithProcessor(s.processVideo) }

func (s *Service) startVideoWorkerWithProcessor(process videoProcessor) {
	s.videoMu.Lock()
	defer s.videoMu.Unlock()
	if s.closed || s.videoCancel != nil || !s.cfg.VideoProcessing.Enabled || s.db == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.videoCancel = cancel
	s.videoDone = make(chan struct{})
	go func() { defer close(s.videoDone); s.runVideoWorker(ctx, process) }()
}

func (s *Service) runVideoWorker(ctx context.Context, process videoProcessor) {
	wait := func() bool {
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return false
		case <-s.videoWake:
			return true
		case <-timer.C:
			return true
		}
	}
	recovering := true
	for ctx.Err() == nil {
		if recovering {
			if err := s.recoverVideoJobs(ctx); err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Print("wedding video queue: recovery deferred")
				if !wait() {
					return
				}
				continue
			}
			recovering = false
		}
		job, err := s.claimVideoJob(time.Now().UTC())
		if err != nil {
			log.Print("wedding video queue: job claim deferred")
			if !wait() {
				return
			}
			continue
		}
		if job == nil {
			if !wait() {
				return
			}
			continue
		}
		preview, err := process(ctx, *job)
		if errors.Is(err, errVideoUnitStopFailed) {
			// Keep the durable unit name until recovery confirms that FFmpeg stopped.
			recovering = true
			if ctx.Err() != nil || !wait() {
				return
			}
			continue
		}
		if finishErr := s.finishVideoJob(*job, preview, err, ctx.Err() != nil, time.Now().UTC()); finishErr != nil {
			log.Print("wedding video queue: result persistence deferred")
			recovering = true
		}
		if err != nil && ctx.Err() == nil && !errors.Is(err, errVideoProcessingDeferred) {
			log.Printf("wedding video queue: job %s attempt %d failed", job.ID, job.Attempts)
		}
	}
}
