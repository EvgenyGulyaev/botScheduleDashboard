package weddingmedia

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"go.etcd.io/bbolt"
)

func queueTestVideo(t *testing.T, s *Service, id string) {
	t.Helper()
	if err := s.persist(&upload{UploadState: UploadState{ID: "upload-" + id, Complete: true, File: &Media{ID: id, Name: id + ".mp4", MIMEType: "video/mp4", Size: 100}}, FolderID: "folder"}); err != nil {
		t.Fatal(err)
	}
}
func readTestJob(t *testing.T, s *Service, id string) videoJob {
	t.Helper()
	var job videoJob
	if err := s.db.View(func(tx *bbolt.Tx) error { return decodeVideoJob(tx, id, &job) }); err != nil {
		t.Fatal(err)
	}
	return job
}

func TestVideoWorkerClaimsOldestOnceAndPublishesReady(t *testing.T) {
	s := persistentVideoService(t)
	queueTestVideo(t, s, "one")
	queueTestVideo(t, s, "two")
	now := time.Now().UTC()
	first, err := s.claimVideoJob(now)
	if err != nil || first == nil || first.ID != "one" || first.Attempts != 1 {
		t.Fatalf("first claim: %+v %v", first, err)
	}
	if err := s.setVideoJobUnit(first.ID, "wedding-video-test"); err != nil {
		t.Fatal(err)
	}
	if err := s.finishVideoJob(*first, "preview-one", nil, false, now); err != nil {
		t.Fatal(err)
	}
	if id, status := s.previewState("one"); id != "preview-one" || status != "ready" {
		t.Fatalf("ready result: %s %s", id, status)
	}
	second, err := s.claimVideoJob(now)
	if err != nil || second == nil || second.ID != "two" {
		t.Fatalf("second claim: %+v %v", second, err)
	}
	if next, err := s.claimVideoJob(now); err != nil || next != nil {
		t.Fatalf("processing job claimed twice: %+v %v", next, err)
	}
}

func TestVideoWorkerRetriesBackoffThenStops(t *testing.T) {
	s := persistentVideoService(t)
	queueTestVideo(t, s, "retry")
	now := time.Now().UTC()
	for attempt := 1; attempt <= 3; attempt++ {
		job, err := s.claimVideoJob(now)
		if err != nil || job == nil || job.Attempts != attempt {
			t.Fatalf("attempt %d: %+v %v", attempt, job, err)
		}
		if err := s.finishVideoJob(*job, "", errors.New("failed"), false, now); err != nil {
			t.Fatal(err)
		}
		saved := readTestJob(t, s, "retry")
		if attempt == 3 {
			if saved.Status != "failed" {
				t.Fatalf("did not stop retries: %+v", saved)
			}
			break
		}
		if saved.Status != "queued" || !saved.NextAttemptAt.After(now) {
			t.Fatalf("missing backoff: %+v", saved)
		}
		if early, err := s.claimVideoJob(now); err != nil || early != nil {
			t.Fatal("retried before backoff elapsed")
		}
		now = saved.NextAttemptAt
	}
	if job, err := s.claimVideoJob(now.Add(24 * time.Hour)); err != nil || job != nil {
		t.Fatal("exhausted job was retried")
	}
}

func TestVideoWorkerDeferralAndShutdownDoNotConsumeAttempts(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "storage", true: "shutdown"}[cancelled], func(t *testing.T) {
			s := persistentVideoService(t)
			queueTestVideo(t, s, "defer")
			now := time.Now().UTC()
			job, _ := s.claimVideoJob(now)
			if err := s.finishVideoJob(*job, "", errVideoProcessingDeferred, cancelled, now); err != nil {
				t.Fatal(err)
			}
			saved := readTestJob(t, s, "defer")
			if saved.Status != "queued" || saved.Attempts != 0 {
				t.Fatalf("consumed attempt: %+v", saved)
			}
		})
	}
}

func TestVideoWorkerDoesNotBlockRequestsAndCloseJoins(t *testing.T) {
	s := persistentVideoService(t)
	queueTestVideo(t, s, "one")
	queueTestVideo(t, s, "two")
	entered := make(chan struct{}, 2)
	stopped := make(chan struct{})
	var active, maximum atomic.Int32
	s.startVideoWorkerWithProcessor(func(ctx context.Context, job videoJob) (string, error) {
		value := active.Add(1)
		maximum.Store(value)
		entered <- struct{}{}
		<-ctx.Done()
		active.Add(-1)
		close(stopped)
		return "", ctx.Err()
	})
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	start := time.Now()
	if w := request(s, "GET", "/wedding/media/video", "", guest(t, s)); w.Code != 200 {
		t.Fatalf("API blocked: %d", w.Code)
	}
	if time.Since(start) > time.Second {
		t.Fatal("API waited for conversion")
	}
	if maximum.Load() != 1 {
		t.Fatal("started concurrent conversion")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("Close returned before processor stopped")
	}
}

func TestVideoWorkerRecoveryRequeuesInterruptedClaim(t *testing.T) {
	s := persistentVideoService(t)
	queueTestVideo(t, s, "interrupted")
	job, _ := s.claimVideoJob(time.Now())
	if err := s.recoverVideoJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	recovered := readTestJob(t, s, job.ID)
	if recovered.Status != "queued" || recovered.Attempts != 0 || recovered.Unit != "" {
		t.Fatalf("recovery lost job: %+v", recovered)
	}
}

func TestVideoWorkerRetainsOrphanUnitAndBlocksNextJob(t *testing.T) {
	s := persistentVideoService(t)
	queueTestVideo(t, s, "one")
	queueTestVideo(t, s, "two")
	s.cfg.VideoProcessing.SystemctlPath = "/usr/bin/false"
	entered := make(chan string, 2)
	s.startVideoWorkerWithProcessor(func(ctx context.Context, job videoJob) (string, error) {
		if err := s.setVideoJobUnit(job.ID, "wedding-video-00000000000000000000000000000000.service"); err != nil {
			return "", err
		}
		entered <- job.ID
		return "", errVideoUnitStopFailed
	})
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	select {
	case id := <-entered:
		t.Fatalf("started %s with orphaned encoder", id)
	case <-time.After(80 * time.Millisecond):
	}
	saved := readTestJob(t, s, "one")
	if saved.Status != "processing" || saved.Unit == "" {
		t.Fatalf("lost orphan recovery state: %+v", saved)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
