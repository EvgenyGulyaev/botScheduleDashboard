package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestLimiterAllowsUpToLimit(t *testing.T) {
	l := New(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.Allow("ip") {
			t.Fatalf("hit %d should be allowed", i)
		}
	}
	if l.Allow("ip") {
		t.Fatalf("4th hit should be blocked")
	}
}

func TestLimiterResetsAfterWindow(t *testing.T) {
	now := time.Unix(0, 0)
	l := New(2, time.Minute)
	l.now = func() time.Time { return now }
	if !l.Allow("ip") {
		t.Fatalf("hit 1 should be allowed")
	}
	if !l.Allow("ip") {
		t.Fatalf("hit 2 should be allowed")
	}
	if l.Allow("ip") {
		t.Fatalf("hit 3 should be blocked")
	}
	now = now.Add(2 * time.Minute)
	if !l.Allow("ip") {
		t.Fatalf("hit after window should be allowed")
	}
}

func TestLimiterKeysAreIndependent(t *testing.T) {
	l := New(1, time.Minute)
	if !l.Allow("a") {
		t.Fatalf("a should be allowed")
	}
	if l.Allow("a") {
		t.Fatalf("a second hit should be blocked")
	}
	if !l.Allow("b") {
		t.Fatalf("b should be allowed")
	}
}

func TestLimiterConcurrent(t *testing.T) {
	l := New(1000, time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				l.Allow("shared")
			}
		}()
	}
	wg.Wait()
}
