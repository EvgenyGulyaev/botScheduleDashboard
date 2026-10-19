package weddingmedia

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

// Implement WriterTo as well so the test catches io.Copy bypassing Read's bound.
type videoDownloadTestBody struct {
	offset, remaining int64
	maxRead           int
	closed, copied    bool
}

func (b *videoDownloadTestBody) Read(p []byte) (int, error) {
	b.maxRead = max(b.maxRead, len(p))
	if b.remaining == 0 {
		return 0, io.EOF
	}
	n := int(min(int64(len(p)), b.remaining))
	for i := 0; i < n; i++ {
		p[i] = byte((b.offset + int64(i)) % 251)
	}
	b.offset += int64(n)
	b.remaining -= int64(n)
	return n, nil
}

func (b *videoDownloadTestBody) Close() error { b.closed = true; return nil }

func (b *videoDownloadTestBody) WriteTo(io.Writer) (int64, error) {
	b.copied = true
	return 0, errors.New("unexpected WriterTo fast path")
}

func TestVideoDownloadUsesBoundedBuffers(t *testing.T) {
	const size = 2*ChunkSize + 117
	var mu sync.Mutex
	bodies := map[int64]*videoDownloadTestBody{}
	s := &Service{baseURL: "https://drive.test", accessToken: "token", tokenExpiry: time.Now().Add(time.Hour)}
	s.client = &http.Client{Transport: mediaRoundTripper(func(r *http.Request) (*http.Response, error) {
		var start, end int64
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			return nil, err
		}
		body := &videoDownloadTestBody{offset: start, remaining: end - start + 1}
		mu.Lock()
		bodies[start] = body
		mu.Unlock()
		return &http.Response{StatusCode: 206, Header: http.Header{"Content-Range": {fmt.Sprintf("bytes %d-%d/%d", start, end, size)}}, ContentLength: body.remaining, Body: body}, nil
	})}
	file, err := os.CreateTemp(t.TempDir(), "input")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := s.downloadVideo(context.Background(), "original", size, file); err != nil {
		t.Fatal(err)
	}
	for _, start := range []int64{0, ChunkSize, 2 * ChunkSize} {
		body := bodies[start]
		if body == nil {
			t.Fatalf("missing range at %d", start)
		}
		if body.maxRead > 256<<10 || body.copied || !body.closed {
			t.Errorf("range %d: max read %d, WriterTo %t, closed %t", start, body.maxRead, body.copied, body.closed)
		}
		var sample [117]byte
		if _, err := file.ReadAt(sample[:], start); err != nil {
			t.Fatal(err)
		}
		for i, got := range sample {
			if want := byte((start + int64(i)) % 251); got != want {
				t.Fatalf("offset %d: got %d, want %d", start+int64(i), got, want)
			}
		}
	}
}

func TestVideoDownloadValidatesStream(t *testing.T) {
	const size = ChunkSize
	for _, tc := range []struct {
		name, contentRange, want string
		status                   int
		contentLength, bodySize  int64
	}{
		{"truncated", "bytes 0-8388607/8388608", "incomplete video download", 206, -1, size - 1},
		{"oversized", "bytes 0-8388607/8388608", "invalid video download body", 206, -1, size + 1},
		{"wrong range", "bytes 1-8388608/8388608", "invalid video download range", 206, size, size},
		{"wrong length", "bytes 0-8388607/8388608", "invalid video download range", 206, size - 1, size},
		{"wrong status", "bytes 0-8388607/8388608", "invalid video download range", 200, size, size},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &videoDownloadTestBody{remaining: tc.bodySize}
			s := &Service{baseURL: "https://drive.test", accessToken: "token", tokenExpiry: time.Now().Add(time.Hour)}
			s.client = &http.Client{Transport: mediaRoundTripper(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Range": {tc.contentRange}}, ContentLength: tc.contentLength, Body: body}, nil
			})}
			file, err := os.CreateTemp(t.TempDir(), "input")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if err := s.downloadVideo(context.Background(), "original", size, file); err == nil || err.Error() != tc.want {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
			if !body.closed {
				t.Fatal("response body not closed")
			}
		})
	}
}

func BenchmarkVideoDownloadBuffers(b *testing.B) {
	const size = 2 * ChunkSize
	s := &Service{baseURL: "https://drive.test", accessToken: "token", tokenExpiry: time.Now().Add(time.Hour)}
	s.client = &http.Client{Transport: mediaRoundTripper(func(r *http.Request) (*http.Response, error) {
		var start, end int64
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: 206, Header: http.Header{"Content-Range": {fmt.Sprintf("bytes %d-%d/%d", start, end, size)}}, ContentLength: end - start + 1, Body: &videoDownloadTestBody{offset: start, remaining: end - start + 1}}, nil
	})}
	file, err := os.CreateTemp(b.TempDir(), "input")
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()
	b.SetBytes(size)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.downloadVideo(context.Background(), "original", size, file); err != nil {
			b.Fatal(err)
		}
	}
}
