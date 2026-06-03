package ratelimit

import (
	"sync"
	"time"
)

// Limiter is an in-memory token-bucket-ish rate limiter keyed by an arbitrary string
// (typically a client IP). It is safe for concurrent use. Buckets expire after the
// configured window of inactivity.
type Limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	now    func() time.Time
	hits   map[string]*bucket
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
		limit:  limit,
		window: window,
		now:    time.Now,
		hits:   make(map[string]*bucket),
	}
}

// Allow returns true if the given key has not exceeded the limit. It also records
// the hit. Stale buckets are reaped lazily on each call.
func (l *Limiter) Allow(key string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.hits[key]
	if !ok || !now.Before(b.resetAt) {
		l.hits[key] = &bucket{count: 1, resetAt: now.Add(l.window)}
		return true
	}
	if b.count >= l.limit {
		return false
	}
	b.count++
	return true
}

// Reset forgets all buckets. Useful in tests.
func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits = make(map[string]*bucket)
}
