package weddingmedia

import "time"

const thumbnailCacheEntries = 64
const thumbnailCacheBytes = 8 * 1024 * 1024

func (s *Service) loadThumbnail(key string, now time.Time) (cachedThumbnail, bool) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	entry, ok := s.thumbnailCache[key]
	if !ok || !now.Before(entry.Expires) {
		delete(s.thumbnailCache, key)
		return cachedThumbnail{}, false
	}
	s.thumbnailCacheClock++
	entry.Used = s.thumbnailCacheClock
	s.thumbnailCache[key] = entry
	return entry, true
}

func (s *Service) storeThumbnail(key string, data []byte, contentType string, now time.Time) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	delete(s.thumbnailCache, key)
	bytes := len(data)
	for id, entry := range s.thumbnailCache {
		if !now.Before(entry.Expires) {
			delete(s.thumbnailCache, id)
		} else {
			bytes += len(entry.Data)
		}
	}
	// Scanning at most 64 entries keeps eviction and byte accounting together.
	for len(s.thumbnailCache) >= thumbnailCacheEntries || bytes > thumbnailCacheBytes {
		var oldest string
		var used uint64
		for id, entry := range s.thumbnailCache {
			if oldest == "" || entry.Used < used {
				oldest, used = id, entry.Used
			}
		}
		if oldest == "" {
			return
		}
		bytes -= len(s.thumbnailCache[oldest].Data)
		delete(s.thumbnailCache, oldest)
	}
	s.thumbnailCacheClock++
	s.thumbnailCache[key] = cachedThumbnail{Data: data, MIMEType: contentType, Expires: now.Add(time.Minute), Used: s.thumbnailCacheClock}
}
