package weddingmedia

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVideoViewingBoundsRangesWithoutChangingOriginalDownloads(t *testing.T) {
	for _, tc := range []struct{ name, input, query, want string }{
		{"initial", "", "", "bytes=0-524287"},
		{"open", "bytes=0-", "", "bytes=0-524287"},
		{"continuation", "bytes=524288-", "", "bytes=524288-2621439"},
		{"seek", "bytes=3000000-", "", "bytes=3000000-5097151"},
		{"large", "bytes=0-99000000", "", "bytes=0-524287"},
		{"small", "bytes=0-1", "", "bytes=0-1"},
		{"tail", "bytes=-10000000", "", "bytes=-2097152"},
		{"download", "bytes=0-", "?download=1", "bytes=0-"},
		{"full download", "", "?download=1", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var seen string
			s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("alt") == "media" {
					seen = r.Header.Get("Range")
					w.WriteHeader(206)
					_, _ = w.Write([]byte("video"))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "movie", "name": "movie.mp4", "mimeType": "video/mp4", "parents": []string{"folder"}})
			})
			r := httptest.NewRequest("GET", "/wedding/media/movie/content"+tc.query, nil)
			if tc.input != "" {
				r.Header.Set("Range", tc.input)
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != 206 || seen != tc.want {
				t.Fatalf("status %d range %q want %q", w.Code, seen, tc.want)
			}
		})
	}
}

func TestVideoRangeRejectsOverflowAndInvalidBounds(t *testing.T) {
	for _, value := range []string{"bytes=-0", "bytes=20-10", "bytes=9223372036854775808-", "bytes=0-9223372036854775808", "bytes=0-1,4-5"} {
		if _, err := boundedVideoRange(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
	if got, err := boundedVideoRange("bytes=9223372036854775806-"); err != nil || got != "bytes=9223372036854775806-9223372036854775807" {
		t.Fatalf("overflow near maximum: %q %v", got, err)
	}
}
