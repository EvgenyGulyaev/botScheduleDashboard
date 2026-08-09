package routes

import (
	"botDashboard/internal/model"
	"testing"
	"time"
)

func TestScheduleWeddingRSVPNotificationReturnsBeforeNotificationCompletes(t *testing.T) {
	originalNotifier := weddingRSVPNotifier
	defer func() {
		weddingRSVPNotifier = originalNotifier
	}()

	started := make(chan struct{})
	release := make(chan struct{})
	weddingRSVPNotifier = func(model.WeddingRSVP) {
		close(started)
		<-release
	}

	done := make(chan struct{})
	go func() {
		scheduleWeddingRSVPNotification(model.WeddingRSVP{})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduling notification waited for it to complete")
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("notification was not started")
	}
	close(release)
}
