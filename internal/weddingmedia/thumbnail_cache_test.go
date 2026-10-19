package weddingmedia

import (
	"fmt"
	"testing"
	"time"
)

func TestThumbnailCacheTTLAndExpiredInsertion(t *testing.T) {
	s, _ := photoService(t)
	now := time.Now()
	s.storeThumbnail("expired", []byte("old"), "image/jpeg", now)
	s.storeThumbnail("fresh", []byte("new"), "image/jpeg", now.Add(30*time.Second))
	if _, ok := s.loadThumbnail("expired", now.Add(time.Minute)); ok {
		t.Fatal("entry survives its one minute TTL")
	}
	if _, ok := s.loadThumbnail("fresh", now.Add(time.Minute)); !ok {
		t.Fatal("expiring one entry removed a fresh entry")
	}
	s.storeThumbnail("expired-on-write", []byte("old"), "image/jpeg", now)
	s.storeThumbnail("newest", []byte("newest"), "image/jpeg", now.Add(time.Minute))
	if _, ok := s.thumbnailCache["expired-on-write"]; ok {
		t.Fatal("insertion did not prune expired entries")
	}
	if len(s.thumbnailCache) != 2 {
		t.Fatalf("fresh entries = %d", len(s.thumbnailCache))
	}
}

func TestThumbnailCacheByteAccountingAfterReplacement(t *testing.T) {
	s, _ := photoService(t)
	now := time.Now()
	for i := 0; i < 4; i++ {
		s.storeThumbnail(fmt.Sprint(i), make([]byte, 2*1024*1024), "image/jpeg", now)
	}
	s.storeThumbnail("0", []byte("small"), "image/jpeg", now)
	s.storeThumbnail("4", make([]byte, 1024*1024), "image/jpeg", now)
	if len(s.thumbnailCache) != 5 {
		t.Fatalf("replacement evicted entries: %d", len(s.thumbnailCache))
	}
	bytes := 0
	for _, entry := range s.thumbnailCache {
		bytes += len(entry.Data)
	}
	if bytes > 8*1024*1024 {
		t.Fatalf("cache bytes = %d", bytes)
	}
}
