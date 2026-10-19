package weddingmedia

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type photoUpstream struct {
	file                driveFile
	metadata, originals int
	images              map[string]int
	body, contentType   string
	length              int64
}

func photoService(t *testing.T) (*Service, *photoUpstream) {
	t.Helper()
	s, _ := testService(t, func(http.ResponseWriter, *http.Request) { t.Fatal("unexpected network call") })
	u := &photoUpstream{
		file:   driveFile{ID: "photo", Name: "photo.jpg", MIMEType: "image/jpeg", Parents: []string{"folder"}, ThumbnailLink: "https://lh3.googleusercontent.com/photo=s220?token=a%2Bb&sz=9"},
		images: map[string]int{}, body: "rendition", contentType: "image/jpeg", length: 9,
	}
	s.client.Transport = mediaRoundTripper(func(r *http.Request) (*http.Response, error) {
		body, contentType, length := u.body, u.contentType, u.length
		if r.URL.Host == "lh3.googleusercontent.com" {
			if r.Header.Get("Range") != "" {
				t.Fatal("rendition forwarded Range")
			}
			u.images[r.URL.RequestURI()]++
		} else if r.URL.Query().Get("alt") == "media" {
			u.originals++
			body, contentType = "original bytes", "image/jpeg"
			length = int64(len(body))
		} else {
			u.metadata++
			file := u.file
			file.ID = strings.TrimPrefix(r.URL.Path, "/drive/v3/files/")
			data, _ := json.Marshal(file)
			body, contentType, length = string(data), "application/json", int64(len(data))
		}
		h := http.Header{"Content-Type": {contentType}}
		if length >= 0 {
			h.Set("Content-Length", fmt.Sprint(length))
		}
		return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(body)), ContentLength: length}, nil
	})
	return s, u
}

func TestPhotoPreviewUsesLargeRenditionAndLeavesOriginalIntact(t *testing.T) {
	s, u := photoService(t)
	for _, kind := range []string{"thumbnail", "preview", "thumbnail", "preview"} {
		r := httptest.NewRequest("GET", "/wedding/media/photo/"+kind, nil)
		r.Header.Set("Range", "bytes=0-1")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 200 || w.Body.String() != "rendition" || w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("%s: %d %q %v", kind, w.Code, w.Body.String(), w.Header())
		}
	}
	for _, target := range []string{"/photo=s220?token=a%2Bb&sz=9", "/photo=s1600?token=a%2Bb&sz=9"} {
		if u.images[target] != 1 {
			t.Fatalf("rendition %s fetched %d times", target, u.images[target])
		}
	}
	if u.metadata != 4 {
		t.Fatalf("metadata checks = %d", u.metadata)
	}
	for _, suffix := range []string{"content", "content?download=1"} {
		w := request(s, "GET", "/wedding/media/photo/"+suffix, "", nil)
		if w.Code != 200 || w.Body.String() != "original bytes" {
			t.Fatalf("original %d %q", w.Code, w.Body.String())
		}
	}
	if u.originals != 2 {
		t.Fatalf("original requests = %d", u.originals)
	}
}

func TestPhotoPreviewRejectsUnsafeOrUnavailableSources(t *testing.T) {
	for _, tc := range []struct{ name, link, mediaType, parent string }{
		{"foreign host", "https://evil.test/photo=s220", "image/jpeg", "folder"},
		{"host suffix spoof", "https://lh3.googleusercontent.com.evil.test/photo=s220", "image/jpeg", "folder"},
		{"http", "http://lh3.googleusercontent.com/photo=s220", "image/jpeg", "folder"},
		{"port", "https://lh3.googleusercontent.com:443/photo=s220", "image/jpeg", "folder"},
		{"credentials", "https://x@lh3.googleusercontent.com/photo=s220", "image/jpeg", "folder"},
		{"unknown suffix", "https://lh3.googleusercontent.com/photo=w220", "image/jpeg", "folder"},
		{"query size only", "https://lh3.googleusercontent.com/photo?size=s220", "image/jpeg", "folder"},
		{"video", "https://lh3.googleusercontent.com/photo=s220", "video/mp4", "folder"},
		{"foreign folder", "https://lh3.googleusercontent.com/photo=s220", "image/jpeg", "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, u := photoService(t)
			u.file.ThumbnailLink, u.file.MIMEType, u.file.Parents = tc.link, tc.mediaType, []string{tc.parent}
			w := request(s, "GET", "/wedding/media/photo/preview", "", nil)
			if w.Code != 404 || len(u.images) != 0 || u.originals != 0 {
				t.Fatalf("status %d, upstream %v", w.Code, u.images)
			}
		})
	}
}

func TestPhotoPreviewCacheRechecksAuthorizationAndFolder(t *testing.T) {
	s, u := photoService(t)
	for i := 0; i < 2; i++ {
		if w := request(s, "GET", "/wedding/media/photo/preview", "", nil); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	u.file.Parents = []string{"other"}
	if w := request(s, "GET", "/wedding/media/photo/preview", "", nil); w.Code != 404 {
		t.Fatalf("foreign cached file: %d", w.Code)
	}
	s.Settings = func() (bool, string, error) { return true, "v1", nil }
	if w := request(s, "GET", "/wedding/media/photo/preview", "", nil); w.Code != 401 {
		t.Fatalf("unauthorized cached file: %d", w.Code)
	}
	if u.metadata != 3 || u.images["/photo=s1600?token=a%2Bb&sz=9"] != 1 {
		t.Fatalf("metadata %d images %v", u.metadata, u.images)
	}
}

func TestPhotoPreviewValidatesImageResponseAndBoundedReads(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body string
		length                  int64
		status                  int
		cached                  bool
	}{
		{"SVG", "image/svg+xml", "svg", 3, 415, false},
		{"mixed case SVG", "image/SVG+xml", "svg", 3, 415, false},
		{"nonimage", "text/html", "html", 4, 415, false},
		{"large cacheable", "image/jpeg", strings.Repeat("x", 1024*1024), 1024 * 1024, 200, true},
		{"oversize streams", "image/jpeg", strings.Repeat("x", 2*1024*1024+1), 2*1024*1024 + 1, 200, false},
		{"unknown length streams", "image/jpeg", "unknown", -1, 200, false},
		{"dishonest length bounded", "image/jpeg", strings.Repeat("x", 2*1024*1024+1), 1, 503, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, u := photoService(t)
			u.body, u.contentType, u.length = tc.body, tc.contentType, tc.length
			for i := 0; i < 2; i++ {
				w := request(s, "GET", "/wedding/media/photo/preview", "", nil)
				if w.Code != tc.status {
					t.Fatalf("status %d: %s", w.Code, w.Body.String())
				}
				if tc.status == 200 && w.Body.String() != tc.body {
					t.Fatal("response bytes changed")
				}
			}
			want := 2
			if tc.cached {
				want = 1
			}
			if u.images["/photo=s1600?token=a%2Bb&sz=9"] != want {
				t.Fatalf("upstream requests %v", u.images)
			}
		})
	}
}

func TestThumbnailCacheRetainsTwentyEntriesAndEvictsLeastRecentlyUsed(t *testing.T) {
	s, u := photoService(t)
	get := func(id int) {
		t.Helper()
		if w := request(s, "GET", fmt.Sprintf("/wedding/media/photo%d/thumbnail", id), "", nil); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for i := 0; i < 20; i++ {
		get(i)
	}
	for i := 0; i < 20; i++ {
		get(i)
	}
	if u.images["/photo=s220?token=a%2Bb&sz=9"] != 20 {
		t.Fatalf("20 thumbnails not retained: %v", u.images)
	}
	for i := 20; i < 64; i++ {
		get(i)
	}
	get(0)
	get(64)
	get(0)
	if u.images["/photo=s220?token=a%2Bb&sz=9"] != 65 {
		t.Fatalf("hot entry evicted: %v", u.images)
	}
	get(1)
	if u.images["/photo=s220?token=a%2Bb&sz=9"] != 66 {
		t.Fatal("cold entry retained beyond 64 entry cap")
	}
}

func TestThumbnailCacheSharesByteBudgetAndExpiresIndividually(t *testing.T) {
	s, u := photoService(t)
	u.body, u.length = strings.Repeat("x", 2*1024*1024), 2*1024*1024
	for i := 0; i < 4; i++ {
		if w := request(s, "GET", fmt.Sprintf("/wedding/media/photo%d/preview", i), "", nil); w.Code != 200 {
			t.Fatal(w.Code)
		}
	}
	u.body, u.length = "thumb", 5
	request(s, "GET", "/wedding/media/card/thumbnail", "", nil)
	request(s, "GET", "/wedding/media/photo1/preview", "", nil)
	if u.images["/photo=s1600?token=a%2Bb&sz=9"] != 4 {
		t.Fatal("byte eviction reset whole cache")
	}
	request(s, "GET", "/wedding/media/photo0/preview", "", nil)
	if u.images["/photo=s1600?token=a%2Bb&sz=9"] != 5 {
		t.Fatal("shared cache exceeded 8 MiB budget")
	}
	s.cacheMu.Lock()
	for key, entry := range s.thumbnailCache {
		if strings.Contains(key, "card") {
			entry.Expires = time.Now().Add(-time.Second)
			s.thumbnailCache[key] = entry
		}
	}
	s.cacheMu.Unlock()
	request(s, "GET", "/wedding/media/card/thumbnail", "", nil)
	request(s, "GET", "/wedding/media/photo1/preview", "", nil)
	if u.images["/photo=s220?token=a%2Bb&sz=9"] != 2 || u.images["/photo=s1600?token=a%2Bb&sz=9"] != 5 {
		t.Fatalf("individual expiry: %v", u.images)
	}
}

func TestThumbnailOversizeStreamsWithoutCaching(t *testing.T) {
	s, u := photoService(t)
	u.body, u.length = strings.Repeat("x", 512*1024+1), 512*1024+1
	for i := 0; i < 2; i++ {
		w := request(s, "GET", "/wedding/media/photo/thumbnail", "", nil)
		if w.Code != 200 || w.Body.String() != u.body {
			t.Fatalf("thumbnail response %d, bytes %d", w.Code, w.Body.Len())
		}
	}
	if len(s.thumbnailCache) != 0 || u.images["/photo=s220?token=a%2Bb&sz=9"] != 2 {
		t.Fatal("thumbnail larger than 512 KiB was cached")
	}
}
