package ratelimit

import (
	"fmt"
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

func TestLimiterReapsStaleBuckets(t *testing.T) {
	now := time.Unix(0, 0)
	l := New(2, time.Minute)
	l.SetNowFunc(func() time.Time { return now })
	l.Allow("ip1")
	l.Allow("ip2")
	// Advance past window + some grace
	now = now.Add(3 * time.Minute)
	if !l.Allow("ip1") {
		t.Fatal("ip1 should be allowed after reaping stale bucket")
	}
}

func TestLimiterBlocksNewKeysWhenMapFull(t *testing.T) {
	l := NewWithMax(100, 100, time.Minute)
	// Fill map with 100 keys
	for i := 0; i < 100; i++ {
		l.Allow(fmt.Sprintf("ip%d", i))
	}
	// Existing key still works (limit 100 = not hit)
	if !l.Allow("ip0") {
		t.Fatal("existing ip0 should be allowed")
	}
	// New key must be blocked
	if l.Allow("ip-new") {
		t.Fatal("new key should be blocked")
	}
}

func TestLimiterExistingKeyStillWorksWhenMapFull(t *testing.T) {
	l := NewWithMax(5, 5, time.Minute)
	// Fill the map with 5 distinct keys
	for i := 0; i < 5; i++ {
		l.Allow(fmt.Sprintf("ip%d", i))
	}
	// Now the map is full — new key must be blocked
	if l.Allow("ip-new") {
		t.Fatal("new key should be blocked because map is full")
	}
}
