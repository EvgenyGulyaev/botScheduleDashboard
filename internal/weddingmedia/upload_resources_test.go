package weddingmedia

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type uploadResourceTransport func(*http.Request) (*http.Response, error)

func (f uploadResourceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func uploadResourceService(t testing.TB, transport uploadResourceTransport) *Service {
	t.Helper()
	cfg := Config{ClientID: "client", ClientSecret: "secret", RefreshToken: "refresh", FolderID: "folder", CookieKey: strings.Repeat("a", 64), Origin: "https://wedding.test"}
	s, err := newService(cfg, "", &http.Client{Transport: transport}, "https://drive.test", "https://drive.test/token")
	if err != nil {
		t.Fatal(err)
	}
	s.accessToken, s.tokenExpiry = "token", time.Now().Add(time.Hour)
	t.Cleanup(func() { s.Close() })
	return s
}

func uploadResourceResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func uploadResourceRequest(s *Service, cookie *http.Cookie, id string, data []byte) *http.Request {
	r := httptest.NewRequest("PUT", "https://wedding.test/wedding/media/uploads/"+id, bytes.NewReader(data))
	r.Header.Set("Origin", "https://wedding.test")
	r.Header.Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(data)-1, len(data)))
	r.AddCookie(cookie)
	g, _ := s.claim(r)
	s.mu.Lock()
	s.sessions[id] = &upload{UploadState: UploadState{ID: id}, Owner: g.ID, URL: "https://drive.test/upload/" + id, MIMEType: "image/jpeg", Size: int64(len(data)), Expires: time.Now().Add(time.Hour)}
	s.mu.Unlock()
	return r
}

type uploadObservedBody struct {
	io.ReadCloser
	reads *atomic.Int32
	once  sync.Once
}

func (b *uploadObservedBody) Read(p []byte) (int, error) {
	b.once.Do(func() { b.reads.Add(1) })
	return b.ReadCloser.Read(p)
}

func TestUploadConcurrencyLeavesViewingCapacity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		var once sync.Once
		defer once.Do(func() { close(release) })
		var reads, puts atomic.Int32
		s := uploadResourceService(t, func(r *http.Request) (*http.Response, error) {
			if r.Method == "GET" {
				return uploadResourceResponse(`{"id":"photo","mimeType":"image/jpeg","parents":["folder"]}`), nil
			}
			puts.Add(1)
			select {
			case <-release:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
			return uploadResourceResponse(`{"id":"file","mimeType":"image/jpeg","size":"8"}`), nil
		})
		cookie := guest(t, s)
		data := []byte{255, 216, 255, 224, 0, 0, 0, 0}
		responses := make([]*httptest.ResponseRecorder, 8)
		for i := range responses {
			r := uploadResourceRequest(s, cookie, strconv.Itoa(i), data)
			r.Body = &uploadObservedBody{ReadCloser: r.Body, reads: &reads}
			responses[i] = httptest.NewRecorder()
			go s.ServeHTTP(responses[i], r)
		}
		synctest.Wait()
		if reads.Load() != 2 || puts.Load() != 2 {
			t.Fatalf("active bodies=%d upstream=%d, want two", reads.Load(), puts.Load())
		}
		view := request(s, "GET", "/wedding/media/photo", "", cookie)
		if view.Code != 200 {
			t.Fatalf("view blocked: %d %s", view.Code, view.Body.String())
		}
		extra := uploadResourceRequest(s, cookie, "extra", data)
		extra.Body = &uploadObservedBody{ReadCloser: extra.Body, reads: &reads}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, extra)
		if w.Code != 429 || reads.Load() != 2 {
			t.Fatalf("unbounded queue: status=%d reads=%d", w.Code, reads.Load())
		}
		once.Do(func() { close(release) })
		synctest.Wait()
		for _, w := range responses {
			if w.Code != 200 {
				t.Fatalf("queued upload: %d %s", w.Code, w.Body.String())
			}
		}
	})
}

func TestUploadWaitingCancellationDoesNotReadBody(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		var once sync.Once
		defer once.Do(func() { close(release) })
		s := uploadResourceService(t, func(r *http.Request) (*http.Response, error) {
			select {
			case <-release:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
			return uploadResourceResponse(`{"id":"file","mimeType":"image/jpeg","size":"8"}`), nil
		})
		cookie := guest(t, s)
		data := []byte{255, 216, 255, 224, 0, 0, 0, 0}
		for i := 0; i < 2; i++ {
			go s.ServeHTTP(httptest.NewRecorder(), uploadResourceRequest(s, cookie, strconv.Itoa(i), data))
		}
		synctest.Wait()
		ctx, cancel := context.WithCancel(context.Background())
		r := uploadResourceRequest(s, cookie, "cancelled", data).WithContext(ctx)
		var reads atomic.Int32
		r.Body = &uploadObservedBody{ReadCloser: r.Body, reads: &reads}
		w := httptest.NewRecorder()
		go s.ServeHTTP(w, r)
		synctest.Wait()
		cancel()
		synctest.Wait()
		if reads.Load() != 0 || w.Code != 503 {
			t.Fatalf("cancelled waiter: status=%d reads=%d", w.Code, reads.Load())
		}
		once.Do(func() { close(release) })
		synctest.Wait()
	})
}

func BenchmarkResumableChunkMemory(b *testing.B) {
	data := make([]byte, ChunkSize)
	copy(data, []byte{255, 216, 255, 224})
	s := uploadResourceService(b, func(r *http.Request) (*http.Response, error) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			return nil, err
		}
		return uploadResourceResponse(`{"id":"file","mimeType":"image/jpeg","size":"8388608"}`), nil
	})
	u := &upload{UploadState: UploadState{ID: "memory"}, Owner: "guest", URL: "https://drive.test/upload", MIMEType: "image/jpeg", Size: ChunkSize, Expires: time.Now().Add(time.Hour)}
	s.sessions[u.ID] = u
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		u.Offset, u.Complete, u.File = 0, false, nil
		r := httptest.NewRequest("PUT", "https://wedding.test/wedding/media/uploads/memory", bytes.NewReader(data))
		r.Header.Set("Content-Range", "bytes 0-8388607/8388608")
		if err := s.resume(httptest.NewRecorder(), r, u.ID, u.Owner); err != nil {
			b.Fatal(err)
		}
	}
}

func TestUploadChunkAllocationBudget(t *testing.T) {
	r := testing.Benchmark(BenchmarkResumableChunkMemory)
	if n := r.AllocedBytesPerOp(); n > ChunkSize+(256<<10) {
		t.Fatalf("chunk allocated %d bytes, budget %d", n, ChunkSize+(256<<10))
	}
}

type uploadReadFailure struct{}

func (uploadReadFailure) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestUploadChunkRejectsMalformedBodyBeforeDrive(t *testing.T) {
	data := []byte{255, 216, 255, 224, 0, 0, 0, 0}
	for name, body := range map[string]io.Reader{
		"short":             bytes.NewReader(data[:7]),
		"extra":             bytes.NewReader(append(append([]byte(nil), data...), 0)),
		"read error":        io.MultiReader(bytes.NewReader(data[:4]), uploadReadFailure{}),
		"error after chunk": io.MultiReader(bytes.NewReader(data), uploadReadFailure{}),
		"wrong MIME":        strings.NewReader("notajpeg"),
	} {
		t.Run(name, func(t *testing.T) {
			var puts int
			s := uploadResourceService(t, func(r *http.Request) (*http.Response, error) { puts++; return uploadResourceResponse(`{}`), nil })
			r := uploadResourceRequest(s, guest(t, s), "body", data)
			r.Body = io.NopCloser(body)
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			want := 400
			if name == "wrong MIME" {
				want = 415
			}
			if w.Code != want || puts != 0 {
				t.Fatalf("status=%d upstream=%d", w.Code, puts)
			}
		})
	}
}
