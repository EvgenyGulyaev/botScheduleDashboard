package weddingmedia

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const testHLSPlaylist = "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:4\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-INDEPENDENT-SEGMENTS\n#EXT-X-MAP:URI=\"hls.mp4\",BYTERANGE=\"100@0\"\n#EXTINF:4.000000,\n#EXT-X-BYTERANGE:200@100\nhls.mp4\n#EXTINF:1.500000,\n#EXT-X-BYTERANGE:150@300\nhls.mp4\n#EXT-X-ENDLIST\n"

func TestVideoHLSValidatesRangesAndRendersOnlyAuthenticatedRelativeURLs(t *testing.T) {
	p, err := parseVideoHLS(testHLSPlaylist)
	if err != nil {
		t.Fatal(err)
	}
	if p.Init != 100 || len(p.Segments) != 2 || p.Segments[1].Offset != 300 || p.Segments[1].Length != 150 || p.Segments[1].Duration != 1.5 {
		t.Fatalf("wrong index: %+v", p)
	}
	rendered := p.render()
	if strings.Contains(rendered, "hls.mp4") || strings.Contains(rendered, "https:") || strings.Count(rendered, "?format=hls-data") != 3 || !strings.Contains(rendered, "#EXT-X-ENDLIST") {
		t.Fatalf("unsafe or incomplete playlist: %s", rendered)
	}
	if !strings.Contains(rendered, "#EXT-X-BYTERANGE:150@300") || !strings.Contains(rendered, "#EXT-X-TARGETDURATION:4") {
		t.Fatal("range or duration lost")
	}
}

func TestVideoHLSContentUsesPrivateCopyAndKeepsOriginalDownloads(t *testing.T) {
	for _, tc := range []struct{ query, contentID, contentType string }{
		{"?format=hls", "", "application/vnd.apple.mpegurl"},
		{"?format=hls-data", "hls-copy", "video/mp4"},
		{"", "classic", "video/mp4"},
		{"?download=1&format=hls", "movie", "video/mp4"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			var got string
			s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("alt") == "media" {
					got = strings.TrimPrefix(r.URL.Path, "/drive/v3/files/")
					w.Header().Set("Content-Range", "bytes 0-1/450")
					w.Header().Set("Content-Length", "2")
					w.WriteHeader(206)
					_, _ = w.Write([]byte("ok"))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "movie", "name": "movie.mp4", "mimeType": "video/mp4", "parents": []string{"folder"}})
			})
			s.cfg.VideoPreviews = map[string]string{"movie": "classic"}
			s.cfg.VideoHLSPreviews = map[string]VideoHLSPreview{"movie": {ID: "hls-copy", Playlist: testHLSPlaylist}}
			w := request(s, "GET", "/wedding/media/movie/content"+tc.query, "", nil)
			if got != tc.contentID || !strings.HasPrefix(w.Header().Get("Content-Type"), tc.contentType) {
				t.Fatalf("copy=%q type=%q", got, w.Header().Get("Content-Type"))
			}
			if tc.contentID == "" && (w.Code != 200 || !strings.Contains(w.Body.String(), "?format=hls-data")) {
				t.Fatalf("bad manifest: %d %s", w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("private response became cacheable")
			}
		})
	}
}

func TestVideoHLSStillChecksAlbumMembershipAndLegacyFallback(t *testing.T) {
	s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("alt") == "media" {
			w.WriteHeader(206)
			_, _ = w.Write([]byte("classic"))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "movie", "mimeType": "video/mp4", "parents": []string{"folder"}})
	})
	s.cfg.VideoPreviews = map[string]string{"movie": "classic"}
	w := request(s, "GET", "/wedding/media/movie/content?format=hls", "", nil)
	if w.Code != 206 || w.Body.String() != "classic" {
		t.Fatal("legacy MP4 fallback lost")
	}
	w = request(s, "GET", "/wedding/media/movie/content?format=hls-data", "", nil)
	if w.Code != 404 {
		t.Fatalf("missing HLS data status %d", w.Code)
	}
	s.cfg.FolderID = "other"
	s.cfg.VideoHLSPreviews = map[string]VideoHLSPreview{"movie": {ID: "hls-copy", Playlist: testHLSPlaylist}}
	w = request(s, "GET", "/wedding/media/movie/content?format=hls", "", nil)
	if w.Code != 404 {
		t.Fatalf("out-of-album HLS status %d", w.Code)
	}
	s.Settings = func() (bool, string, error) { return true, "locked", nil }
	for _, format := range []string{"hls", "hls-data"} {
		w = request(s, "GET", "/wedding/media/movie/content?format="+format, "", nil)
		if w.Code != 401 {
			t.Fatalf("unauthorized %s status %d", format, w.Code)
		}
	}
}

func TestVideoHLSRejectsInvalidOrUnboundedPlaylists(t *testing.T) {
	for name, raw := range map[string]string{
		"external URI":      strings.ReplaceAll(testHLSPlaylist, "hls.mp4", "https://evil.invalid/video.mp4"),
		"nonzero init":      strings.Replace(testHLSPlaylist, "100@0", "100@1", 1),
		"large init":        strings.Replace(testHLSPlaylist, "100@0", "999999@0", 1),
		"overlap":           strings.Replace(testHLSPlaylist, "150@300", "150@299", 1),
		"gap":               strings.Replace(testHLSPlaylist, "150@300", "150@301", 1),
		"large segment":     strings.Replace(testHLSPlaylist, "150@300", "2097153@300", 1),
		"infinite duration": strings.Replace(testHLSPlaylist, "1.500000", "Inf", 1),
		"negative duration": strings.Replace(testHLSPlaylist, "1.500000", "-1", 1),
		"missing end":       strings.Replace(testHLSPlaylist, "#EXT-X-ENDLIST\n", "", 1),
		"encryption":        strings.Replace(testHLSPlaylist, "#EXT-X-ENDLIST", "#EXT-X-KEY:METHOD=AES-128,URI=\"https://evil.invalid\"\n#EXT-X-ENDLIST", 1),
		"trailing segment":  testHLSPlaylist + "#EXTINF:1,\n",
		"too large":         strings.Repeat("x", 128*1024+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseVideoHLS(raw); err == nil {
				t.Fatal("invalid playlist accepted")
			}
		})
	}
}
