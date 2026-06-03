package ratelimit

import (
	"sync"
	"time"
)

const (
	defaultMaxEntries = 50000
	defaultReapAfter  = 5 * time.Minute
)

type Limiter struct {
	mu         sync.Mutex
	limit      int
	window     time.Duration
	maxEntries int
	now        func() time.Time
	hits       map[string]*bucket
	lastReap   time.Time
}

type bucket struct {
	count   int
	resetAt time.Time
}

func New(limit int, window time.Duration) *Limiter {
	if limit <= 0 {
		limit = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	return &Limiter{
		limit:      limit,
		window:     window,
		maxEntries: defaultMaxEntries,
		now:        time.Now,
		hits:       make(map[string]*bucket),
	}
}

// Allow returns true if the given key has not exceeded the limit. It also records
// the hit. Stale buckets are reaped periodically.
func (l *Limiter) Allow(key string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	l.reapLocked(now)
	b, ok := l.hits[key]
	if !ok || !now.Before(b.resetAt) {
		if !ok && len(l.hits) >= l.maxEntries {
			return false
		}
		l.hits[key] = &bucket{count: 1, resetAt: now.Add(l.window)}
		return true
	}
	if b.count >= l.limit {
		return false
	}
	b.count++
	return true
}

func (l *Limiter) reapLocked(now time.Time) {
	if now.Sub(l.lastReap) < l.window*2 {
		return
	}
	l.lastReap = now
	for k, b := range l.hits {
		if !now.Before(b.resetAt) {
			delete(l.hits, k)
		}
	}
}

// SetNowFunc replaces the clock function. For tests only.
func (l *Limiter) SetNowFunc(fn func() time.Time) { l.mu.Lock(); defer l.mu.Unlock(); l.now = fn }

// NewWithMax creates a Limiter with an explicit max-entries ceiling. For tests.
func NewWithMax(limit, maxEntries int, window time.Duration) *Limiter {
	l := New(limit, window)
	l.maxEntries = maxEntries
	return l
}

// Reset forgets all buckets. Useful in tests.
func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits = make(map[string]*bucket)
}
