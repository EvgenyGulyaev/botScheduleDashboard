package weddingmedia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGalleryPageSizeAndMediaQuery(t *testing.T) {
	s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/drive/v3/files" || q.Get("pageSize") != "20" || q.Get("orderBy") != "createdTime desc" || q.Get("pageToken") != "next" {
			t.Errorf("unexpected list request: %s", r.URL)
		}
		filter := q.Get("q")
		if !strings.Contains(filter, "'folder' in parents and trashed = false and (") || !strings.Contains(filter, "mimeType = 'image/jpeg'") || !strings.Contains(filter, "mimeType = 'video/x-matroska'") || strings.Contains(filter, "image/svg+xml") || strings.Contains(filter, "application/vnd.google-apps.folder") {
			t.Errorf("unexpected media filter: %s", filter)
		}
		fmt.Fprint(w, `{"files":[],"nextPageToken":"after"}`)
	})
	w := request(s, "GET", "/wedding/media?cursor=next", "", guest(t, s))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"items":[],"next_cursor":"after"}` {
		t.Fatalf("list contract: %d %s", w.Code, w.Body.String())
	}
}

func summaryTotal(t *testing.T, s *Service, cookie *http.Cookie) int {
	t.Helper()
	w := request(s, "GET", "/wedding/media/summary", "", cookie)
	var result struct {
		Total int `json:"total"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || !strings.Contains(w.Body.String(), `"total":`) {
		t.Fatalf("summary: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("summary cache header: %v", w.Header())
	}
	return result.Total
}

func TestSummaryCountsAllMetadataPagesAndFiltersFiles(t *testing.T) {
	var cursors []string
	s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/drive/v3/files" || q.Get("alt") != "" || q.Get("pageSize") != "1000" || q.Get("fields") != "nextPageToken,incompleteSearch,files(id,mimeType,parents,trashed)" {
			t.Errorf("summary must request metadata only: %s", r.URL)
		}
		if !strings.Contains(q.Get("q"), "'folder' in parents and trashed = false and (") {
			t.Errorf("missing folder/media query: %s", r.URL)
		}
		cursor := q.Get("pageToken")
		cursors = append(cursors, cursor)
		switch cursor {
		case "":
			fmt.Fprint(w, `{"files":[{"id":"photo","mimeType":"image/jpeg","parents":["folder"]},{"id":"foreign","mimeType":"image/png","parents":["other"]},{"id":"removed","mimeType":"image/jpeg","parents":["folder"],"trashed":true},{"id":"svg","mimeType":"image/svg+xml","parents":["folder"]},{"id":"directory","mimeType":"application/vnd.google-apps.folder","parents":["folder"]}],"nextPageToken":"empty"}`)
		case "empty":
			fmt.Fprint(w, `{"files":[],"nextPageToken":"last"}`)
		case "last":
			fmt.Fprint(w, `{"files":[{"id":"video","mimeType":"video/mp4","parents":["folder"]},{"id":"heic","mimeType":"image/heic","parents":["folder","other"]}]}`)
		default:
			t.Errorf("unexpected cursor: %s", cursor)
		}
	})
	c := guest(t, s)
	if total := summaryTotal(t, s, c); total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if strings.Join(cursors, ",") != ",empty,last" {
		t.Fatalf("pages: %v", cursors)
	}
	if total := summaryTotal(t, s, c); total != 3 || len(cursors) != 3 {
		t.Fatalf("summary not cached: total=%d pages=%v", total, cursors)
	}
	s.cacheMu.Lock()
	remaining := time.Until(s.summaryCache.Expires)
	s.summaryCache.Expires = time.Now().Add(-time.Second)
	s.cacheMu.Unlock()
	if remaining < 29*time.Second || remaining > 30*time.Second {
		t.Fatalf("summary TTL = %s, want 30s", remaining)
	}
	if total := summaryTotal(t, s, c); total != 3 || len(cursors) != 6 {
		t.Fatalf("expired summary not refreshed: total=%d pages=%v", total, cursors)
	}
}

func TestSummaryChecksAuthorizationBeforeCache(t *testing.T) {
	var calls atomic.Int32
	s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"files":[]}`)
	})
	c := guest(t, s)
	if total := summaryTotal(t, s, c); total != 0 {
		t.Fatal(total)
	}
	if total := summaryTotal(t, s, c); total != 0 || calls.Load() != 1 {
		t.Fatalf("zero count not cached: total=%d calls=%d", total, calls.Load())
	}
	s.Settings = func() (bool, string, error) { return true, "new-code", nil }
	for _, cookie := range []*http.Cookie{nil, c} {
		w := request(s, "GET", "/wedding/media/summary", "", cookie)
		if w.Code != 401 || calls.Load() != 1 || strings.Contains(w.Body.String(), `"total"`) {
			t.Fatalf("unauthorized cached count: %d %s calls=%d", w.Code, w.Body.String(), calls.Load())
		}
	}
}

func TestSummaryRejectsBrokenPaginationWithoutCaching(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		wantCalls  int32
	}{
		{"repeated token", `{"files":[],"nextPageToken":"repeat"}`, 2},
		{"oversized token", `{"files":[],"nextPageToken":"` + strings.Repeat("a", 2049) + `"}`, 1},
		{"incomplete search", `{"files":[],"incompleteSearch":true}`, 1},
		{"invalid upstream token", "", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if tc.body == "" {
					w.WriteHeader(400)
					fmt.Fprint(w, `{"error":{"message":"Invalid page token"}}`)
					return
				}
				fmt.Fprint(w, tc.body)
			})
			c := guest(t, s)
			for i := int32(1); i <= 2; i++ {
				w := request(s, "GET", "/wedding/media/summary", "", c)
				if w.Code != 503 || strings.Contains(w.Body.String(), `"total"`) || calls.Load() != tc.wantCalls*i {
					t.Fatalf("invalid pagination: %d %s calls=%d", w.Code, w.Body.String(), calls.Load())
				}
			}
		})
	}
}

func TestSummaryUploadInvalidatesCacheAndInflightScan(t *testing.T) {
	for _, inflight := range []bool{false, true} {
		t.Run(fmt.Sprintf("inflight=%v", inflight), func(t *testing.T) {
			var calls atomic.Int32
			var target string
			started, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			s, g := testService(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/drive/v3/files":
					if calls.Add(1) == 1 {
						close(started)
						if inflight {
							<-release
						}
						fmt.Fprint(w, `{"files":[]}`)
					} else {
						fmt.Fprint(w, `{"files":[{"id":"done","mimeType":"image/jpeg","parents":["folder"]}]}`)
					}
				case "/upload/drive/v3/files":
					w.Header().Set("Location", target+"/upload/summary")
				default:
					fmt.Fprint(w, `{"id":"done","mimeType":"image/jpeg","size":"8","parents":["folder"]}`)
				}
			})
			target = g.URL
			c := guest(t, s)
			result := make(chan *httptest.ResponseRecorder, 1)
			go func() { result <- request(s, "GET", "/wedding/media/summary", "", c) }()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("summary did not start")
			}
			if !inflight {
				if w := <-result; w.Code != 200 {
					t.Fatalf("initial count: %d %s", w.Code, w.Body.String())
				}
			}
			w := request(s, "POST", "/wedding/media/uploads", `{"name":"a.jpg","mime_type":"image/jpeg","size":8}`, c)
			var state UploadState
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &state) != nil {
				t.Fatalf("create upload: %d %s", w.Code, w.Body.String())
			}
			r := httptest.NewRequest("PUT", "https://wedding.test/wedding/media/uploads/"+state.ID, bytes.NewReader([]byte{255, 216, 255, 224, 0, 0, 0, 0}))
			r.Header.Set("Origin", "https://wedding.test")
			r.Header.Set("Content-Range", "bytes 0-7/8")
			r.AddCookie(c)
			w = httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != 200 {
				t.Fatalf("complete upload: %d %s", w.Code, w.Body.String())
			}
			unblock()
			if inflight {
				if w := <-result; w.Code != 200 {
					t.Fatalf("inflight count: %d %s", w.Code, w.Body.String())
				}
			}
			if total := summaryTotal(t, s, c); total != 1 || calls.Load() != 2 {
				t.Fatalf("stale count after completion: total=%d calls=%d", total, calls.Load())
			}
		})
	}
}

func TestSummaryWaitHonorsCanceledContext(t *testing.T) {
	s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected Google call") })
	s.summaryLoading = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := httptest.NewRequest("GET", "https://wedding.test/wedding/media/summary", nil).WithContext(ctx)
	if err := s.summary(httptest.NewRecorder(), r); err != context.Canceled {
		t.Fatalf("waiting summary error: %v", err)
	}
}

func TestSummaryScanRespectsMediaLimitButCacheNeedsNoSlot(t *testing.T) {
	var calls atomic.Int32
	s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"files":[]}`)
	})
	c := guest(t, s)
	for i := 0; i < cap(s.inFlight); i++ {
		s.inFlight <- struct{}{}
	}
	if w := request(s, "GET", "/wedding/media/summary", "", c); w.Code != 429 || calls.Load() != 0 {
		t.Fatalf("scan bypasses media limit: %d calls=%d", w.Code, calls.Load())
	}
	for i := 0; i < cap(s.inFlight); i++ {
		<-s.inFlight
	}
	if total := summaryTotal(t, s, c); total != 0 || calls.Load() != 1 {
		t.Fatalf("failed scan poisoned cache: total=%d calls=%d", total, calls.Load())
	}
	for i := 0; i < cap(s.inFlight); i++ {
		s.inFlight <- struct{}{}
	}
	if total := summaryTotal(t, s, c); total != 0 || calls.Load() != 1 {
		t.Fatalf("cache used a media slot: total=%d calls=%d", total, calls.Load())
	}
}

func TestGalleryRejectsInvalidNextToken(t *testing.T) {
	for _, next := range []string{"current", strings.Repeat("a", 2049)} {
		s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{"files": []driveFile{}, "nextPageToken": next})
		})
		if w := request(s, "GET", "/wedding/media?cursor=current", "", guest(t, s)); w.Code != 503 || strings.Contains(w.Body.String(), `"items"`) {
			t.Fatalf("invalid list token: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestSummaryCoalescesConcurrentScansAndDoesNotBlockContent(t *testing.T) {
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/drive/v3/files" {
			if calls.Add(1) == 1 {
				close(started)
			}
			<-release
			fmt.Fprint(w, `{"files":[]}`)
			return
		}
		fmt.Fprint(w, `{"id":"photo","mimeType":"image/jpeg","parents":["folder"]}`)
	})
	c := guest(t, s)
	var wg sync.WaitGroup
	results := make(chan *httptest.ResponseRecorder, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- request(s, "GET", "/wedding/media/summary", "", c)
		}()
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("summary did not start")
	}
	// Let the remaining requests reach the shared scan while Google stays blocked.
	deadline := time.Now().Add(50 * time.Millisecond)
	for len(s.inFlight) < 8 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if w := request(s, "GET", "/wedding/media/photo", "", c); w.Code != 200 {
		t.Fatalf("metadata blocked by summary: %d %s", w.Code, w.Body.String())
	}
	unblock()
	wg.Wait()
	close(results)
	for w := range results {
		if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"total":0}` {
			t.Fatalf("summary result: %d %s", w.Code, w.Body.String())
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("concurrent scans = %d, want 1", calls.Load())
	}
}
