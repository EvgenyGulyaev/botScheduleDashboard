package weddingmedia

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompatibleVideoPreviewKeepsOriginalDownload(t *testing.T) {
	for _, tc := range []struct{ name, query, target, mime, disposition string }{
		{"watch", "", "preview", "video/mp4", "inline"},
		{"download", "?download=1", "original", "video/quicktime", "attachment"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("alt") != "media" {
					fmt.Fprint(w, `{"id":"original","name":"clip.mov","mimeType":"video/quicktime","parents":["folder"],"size":"100"}`)
					return
				}
				if r.URL.Path != "/drive/v3/files/"+tc.target || r.Header.Get("Range") != "bytes=10-19" {
					t.Errorf("wrong content target or range: %s %s", r.URL.Path, r.Header.Get("Range"))
				}
				w.Header().Set("Content-Range", "bytes 10-19/100")
				w.Header().Set("Content-Length", "10")
				w.WriteHeader(206)
				fmt.Fprint(w, "0123456789")
			})
			if err := json.Unmarshal([]byte(`{"video_previews":{"original":"preview"}}`), &s.cfg); err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("GET", "https://wedding.test/wedding/media/original/content"+tc.query, nil)
			r.AddCookie(guest(t, s))
			r.Header.Set("Range", "bytes=10-19")
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != 206 || w.Body.String() != "0123456789" || w.Header().Get("Content-Range") != "bytes 10-19/100" || w.Header().Get("Content-Type") != tc.mime || !strings.HasPrefix(w.Header().Get("Content-Disposition"), tc.disposition+";") {
				t.Fatalf("unexpected video response: %d %v %s", w.Code, w.Header(), w.Body.String())
			}
		})
	}
}

func TestVideoPreviewStillRequiresOriginalInAlbum(t *testing.T) {
	s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("alt") == "media" {
			t.Fatal("foreign original exposed a preview")
		}
		fmt.Fprint(w, `{"id":"foreign","mimeType":"video/mp4","parents":["other"]}`)
	})
	if err := json.Unmarshal([]byte(`{"video_previews":{"foreign":"preview"}}`), &s.cfg); err != nil {
		t.Fatal(err)
	}
	if w := request(s, "GET", "/wedding/media/foreign/content", "", guest(t, s)); w.Code != 404 {
		t.Fatalf("foreign media status %d", w.Code)
	}
}

func TestVideoPreviewDoesNotReplacePhotos(t *testing.T) {
	s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("alt") != "media" {
			fmt.Fprint(w, `{"id":"photo","name":"photo.jpg","mimeType":"image/jpeg","parents":["folder"]}`)
			return
		}
		if r.URL.Path != "/drive/v3/files/photo" {
			t.Errorf("photo used video preview: %s", r.URL.Path)
		}
		fmt.Fprint(w, "photo")
	})
	if err := json.Unmarshal([]byte(`{"video_previews":{"photo":"preview"}}`), &s.cfg); err != nil {
		t.Fatal(err)
	}
	if w := request(s, "GET", "/wedding/media/photo/content", "", guest(t, s)); w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("photo response %d %v", w.Code, w.Header())
	}
}

func TestVideoPreviewRejectsInvalidConfiguredIDs(t *testing.T) {
	for _, value := range []string{`{"../original":"preview"}`, `{"original":"../preview"}`, `{"original":""}`} {
		var cfg Config
		if err := json.Unmarshal([]byte(`{"video_previews":`+value+`}`), &cfg); err != nil {
			t.Fatal(err)
		}
		s, err := newService(cfg, "", nil, "https://www.googleapis.com", "https://oauth2.googleapis.com/token")
		if err == nil {
			s.Close()
			t.Errorf("accepted invalid preview mapping %s", value)
		}
	}
}
