package weddingmedia

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testService(t *testing.T, upstream http.HandlerFunc) (*Service, *httptest.Server) {
	t.Helper()
	google := httptest.NewServer(upstream)
	t.Cleanup(google.Close)
	cfg := Config{ClientID: "client", ClientSecret: "secret", RefreshToken: "refresh", FolderID: "folder", CookieKey: strings.Repeat("a", 64), Origin: "https://wedding.test"}
	s, err := newService(cfg, "", google.Client(), google.URL, google.URL+"/token")
	if err != nil {
		t.Fatal(err)
	}
	s.accessToken = "token"
	s.tokenExpiry = time.Now().Add(time.Hour)
	t.Cleanup(func() { s.Close() })
	return s, google
}
func request(s *Service, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://wedding.test"+path, strings.NewReader(body))
	r.Header.Set("Origin", "https://wedding.test")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func guest(t *testing.T, s *Service) *http.Cookie {
	t.Helper()
	w := request(s, "GET", "/wedding/media/status", "", nil)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	return w.Result().Cookies()[0]
}

func TestGuestAuthorizationAndOrigin(t *testing.T) {
	s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("unexpected Google call") })
	s.Settings = func() (bool, string, error) { return true, "v1", nil }
	if w := request(s, "GET", "/wedding/media", "", nil); w.Code != 401 {
		t.Fatalf("status %d", w.Code)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "https://wedding.test/wedding/access/verify", nil)
	s.Authorize(w, r, "v1")
	c := w.Result().Cookies()[0]
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unsafe cookie %+v", c)
	}
	status := request(s, "GET", "/wedding/media/status", "", c)
	if !strings.Contains(status.Body.String(), `"guest_authorized":true`) {
		t.Fatal(status.Body.String())
	}
	s.Settings = func() (bool, string, error) { return true, "v2", nil }
	if w := request(s, "GET", "/wedding/media", "", c); w.Code != 401 {
		t.Fatalf("old cookie %d", w.Code)
	}
	s.Settings = nil
	c = guest(t, s)
	r = httptest.NewRequest("POST", "https://wedding.test/wedding/media/uploads", strings.NewReader(`{}`))
	r.AddCookie(c)
	r.Header.Set("Origin", "https://evil.test")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("origin %d", w.Code)
	}
}

func TestUploadValidation(t *testing.T) {
	s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("invalid upload reached Google") })
	c := guest(t, s)
	for _, tc := range []struct {
		body   string
		status int
	}{{`{"name":"x.svg","mime_type":"image/svg+xml","size":10}`, 415}, {`{"name":"x.jpg","mime_type":"image/jpeg","size":1073741825}`, 413}, {`{"name":"../x.jpg","mime_type":"image/jpeg","size":10}`, 400}, {`{"name":"x.jpg","mime_type":"image/jpeg","size":0}`, 400}} {
		if w := request(s, "POST", "/wedding/media/uploads", tc.body, c); w.Code != tc.status {
			t.Fatalf("%s got %d %s", tc.body, w.Code, w.Body.String())
		}
	}
}

func TestResumableUploadOwnershipSniffingAndCompletion(t *testing.T) {
	var googleURL string
	puts := 0
	accepted := false
	s, g := testService(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/upload/drive/v3/files":
			w.Header().Set("Location", googleURL+"/upload/session")
			w.WriteHeader(200)
		case "/upload/session":
			if strings.HasPrefix(r.Header.Get("Content-Range"), "bytes */") {
				if accepted {
					fmt.Fprint(w, `{"id":"file","name":"a.jpg","mimeType":"image/jpeg","size":"8","parents":["folder"]}`)
				} else {
					w.WriteHeader(308)
				}
				return
			}
			puts++
			accepted = true
			fmt.Fprint(w, `{"id":"file","name":"a.jpg","mimeType":"image/jpeg","size":"8","parents":["folder"]}`)
		default:
			t.Fatalf("unexpected %s", r.URL)
		}
	})
	googleURL = g.URL
	c := guest(t, s)
	w := request(s, "POST", "/wedding/media/uploads", `{"name":"a.jpg","mime_type":"image/jpeg","size":8}`, c)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var state UploadState
	json.Unmarshal(w.Body.Bytes(), &state)
	other := guest(t, s)
	if w = request(s, "GET", "/wedding/media/uploads/"+state.ID, "", other); w.Code != 404 {
		t.Fatal("ownership", w.Code)
	}
	put := func(data []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest("PUT", "https://wedding.test/wedding/media/uploads/"+state.ID, bytes.NewReader(data))
		r.AddCookie(c)
		r.Header.Set("Origin", "https://wedding.test")
		r.Header.Set("Content-Range", "bytes 0-7/8")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	if w = put([]byte("MZevil!!")); w.Code != 415 {
		t.Fatal("sniff", w.Code, w.Body.String())
	}
	data := []byte{0xff, 0xd8, 0xff, 0xe0, 0, 0, 0, 0}
	if w = put(data); w.Code != 200 || !strings.Contains(w.Body.String(), `"complete":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = put(data); w.Code != 200 || puts != 1 {
		t.Fatal("duplicate", puts, w.Code)
	}
}

func TestFolderPaginationAndRange(t *testing.T) {
	s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/drive/v3/files" {
			if !strings.Contains(r.URL.Query().Get("q"), "'folder' in parents") || r.URL.Query().Get("pageToken") != "next" {
				t.Fatal(r.URL)
			}
			fmt.Fprint(w, `{"files":[{"id":"good","name":"photo.jpg","mimeType":"image/jpeg","size":"100","parents":["folder"]}],"nextPageToken":"after"}`)
			return
		}
		if r.URL.Query().Get("alt") == "media" {
			if r.Header.Get("Range") != "bytes=10-19" {
				t.Fatal("range")
			}
			w.Header().Set("Content-Range", "bytes 10-19/100")
			w.Header().Set("Content-Length", "10")
			w.WriteHeader(206)
			fmt.Fprint(w, "0123456789")
			return
		}
		parent := "folder"
		if strings.Contains(r.URL.Path, "foreign") {
			parent = "other"
		}
		fmt.Fprintf(w, `{"id":"good","name":"photo.jpg","mimeType":"image/jpeg","size":"100","parents":[%q]}`, parent)
	})
	c := guest(t, s)
	if w := request(s, "GET", "/wedding/media?cursor=next", "", c); w.Code != 200 || !strings.Contains(w.Body.String(), `"next_cursor":"after"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request(s, "GET", "/wedding/media/foreign/content", "", c); w.Code != 404 {
		t.Fatal(w.Code)
	}
	r := httptest.NewRequest("GET", "https://wedding.test/wedding/media/good/content", nil)
	r.AddCookie(c)
	r.Header.Set("Range", "bytes=10-19")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 206 || w.Header().Get("Content-Range") != "bytes 10-19/100" || w.Body.String() != "0123456789" {
		t.Fatal(w.Code, w.Header(), w.Body.String())
	}
}

func TestAmbiguousUploadResumesAfterRestart(t *testing.T) {
	var googleURL string
	accepted := false
	puts := 0
	s, g := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/upload/drive/v3/files" {
			w.Header().Set("Location", googleURL+"/upload/resume")
			return
		}
		if strings.HasPrefix(r.Header.Get("Content-Range"), "bytes */") {
			if accepted {
				fmt.Fprint(w, `{"id":"done","mimeType":"image/jpeg","size":"8","parents":["folder"]}`)
			} else {
				w.WriteHeader(308)
			}
			return
		}
		puts++
		accepted = true
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Fatal(err)
		}
		conn.Close()
	})
	googleURL = g.URL
	path := filepath.Join(t.TempDir(), "drive.json")
	if err := SaveConfig(path, s.cfg); err != nil {
		t.Fatal(err)
	}
	persistent, err := newService(s.cfg, path, g.Client(), g.URL, g.URL+"/token")
	if err != nil {
		t.Fatal(err)
	}
	persistent.accessToken = "token"
	persistent.tokenExpiry = time.Now().Add(time.Hour)
	c := guest(t, persistent)
	w := request(persistent, "POST", "/wedding/media/uploads", `{"name":"a.jpg","mime_type":"image/jpeg","size":8}`, c)
	var state UploadState
	json.Unmarshal(w.Body.Bytes(), &state)
	r := httptest.NewRequest("PUT", "https://wedding.test/wedding/media/uploads/"+state.ID, bytes.NewReader([]byte{255, 216, 255, 224, 0, 0, 0, 0}))
	r.AddCookie(c)
	r.Header.Set("Origin", "https://wedding.test")
	r.Header.Set("Content-Range", "bytes 0-7/8")
	w = httptest.NewRecorder()
	persistent.ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatal(w.Code, w.Body.String())
	}
	persistent.Close()
	persistent, err = newService(s.cfg, path, g.Client(), g.URL, g.URL+"/token")
	if err != nil {
		t.Fatal(err)
	}
	defer persistent.Close()
	persistent.accessToken = "token"
	persistent.tokenExpiry = time.Now().Add(time.Hour)
	w = request(persistent, "GET", "/wedding/media/uploads/"+state.ID, "", c)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"complete":true`) || puts != 1 {
		t.Fatal("restart resume", w.Code, w.Body.String(), puts)
	}
}

func TestSessionBoundsAndProtectedConfig(t *testing.T) {
	s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("full session store reached Google") })
	c := guest(t, s)
	for i := 0; i < maxSessions; i++ {
		id := fmt.Sprint(i)
		s.sessions[id] = &upload{Owner: "other", Expires: time.Now().Add(time.Hour)}
	}
	if w := request(s, "POST", "/wedding/media/uploads", `{"name":"a.jpg","mime_type":"image/jpeg","size":8}`, c); w.Code != 429 {
		t.Fatal(w.Code, w.Body.String())
	}
	path := filepath.Join(t.TempDir(), "drive.json")
	if err := SaveConfig(path, s.cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("unsafe config", info, err)
	}
	if err = os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(path); err == nil {
		t.Fatal("world-readable credentials accepted")
	}
}

func TestPhoneMediaSignatures(t *testing.T) {
	for _, tc := range []struct{ mime, brand string }{{"image/heic", "heic"}, {"image/heif", "mif1"}, {"image/avif", "avif"}, {"video/quicktime", "qt  "}, {"video/mp4", "isom"}, {"video/3gpp", "3gp5"}} {
		data := append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p'}, []byte(tc.brand)...)
		if !matchesMedia(data, tc.mime) {
			t.Fatal(tc.mime)
		}
	}
	for _, m := range []string{"image/heic", "image/heif", "video/mp4", "image/jpeg"} {
		if matchesMedia([]byte("<svg onload=alert(1) />"), m) {
			t.Fatal("SVG accepted", m)
		}
	}
}

func TestUpstreamRangeErrorPreserved(t *testing.T) {
	s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("alt") == "media" {
			w.WriteHeader(416)
			return
		}
		fmt.Fprint(w, `{"id":"a","name":"a.mp4","mimeType":"video/mp4","size":"20","parents":["folder"]}`)
	})
	c := guest(t, s)
	r := httptest.NewRequest("GET", "https://wedding.test/wedding/media/a/content", nil)
	r.AddCookie(c)
	r.Header.Set("Range", "bytes=100-200")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 416 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestTokenRotationIsPersisted(t *testing.T) {
	calls := 0
	s, g := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			t.Fatal("unexpected token target", r.URL)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("refresh_token") != "refresh" || r.Form.Get("grant_type") != "refresh_token" {
			t.Fatal("invalid refresh request")
		}
		calls++
		fmt.Fprint(w, `{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`)
	})
	path := filepath.Join(t.TempDir(), "drive.json")
	if err := SaveConfig(path, s.cfg); err != nil {
		t.Fatal(err)
	}
	persistent, err := newService(s.cfg, path, g.Client(), g.URL, g.URL+"/token")
	if err != nil {
		t.Fatal(err)
	}
	defer persistent.Close()
	for i := 0; i < 2; i++ {
		token, err := persistent.token(t.Context())
		if err != nil || token != "new-access" {
			t.Fatal(token, err)
		}
	}
	if calls != 1 {
		t.Fatal("token not cached", calls)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg Config
	json.Unmarshal(data, &cfg)
	if cfg.RefreshToken != "new-refresh" {
		t.Fatal("rotation lost")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("rotated config permissions")
	}
}

func TestChunkOffsetAndRangeValidation(t *testing.T) {
	var googleURL string
	puts := 0
	s, g := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/upload/drive/v3/files" {
			w.Header().Set("Location", googleURL+"/upload/chunks")
			return
		}
		puts++
		w.Header().Set("Range", "bytes=0-262143")
		w.WriteHeader(308)
	})
	googleURL = g.URL
	c := guest(t, s)
	w := request(s, "POST", "/wedding/media/uploads", `{"name":"a.jpg","mime_type":"image/jpeg","size":262152}`, c)
	var state UploadState
	json.Unmarshal(w.Body.Bytes(), &state)
	put := func(contentRange string, data []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest("PUT", "https://wedding.test/wedding/media/uploads/"+state.ID, bytes.NewReader(data))
		r.Header.Set("Origin", "https://wedding.test")
		r.Header.Set("Content-Range", contentRange)
		r.AddCookie(c)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	if w = put("bytes 0-7/262152", make([]byte, 8)); w.Code != 400 {
		t.Fatal("unrounded chunk", w.Code)
	}
	data := make([]byte, 256*1024)
	copy(data, []byte{255, 216, 255, 224})
	w = put("bytes 0-262143/262152", data)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"offset":262144`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = put("bytes 0-262143/262152", data); w.Code != 409 || puts != 1 {
		t.Fatal("stale offset", w.Code, puts)
	}
	s.sessions[state.ID].Expires = time.Now().Add(-time.Second)
	if w = request(s, "GET", "/wedding/media/uploads/"+state.ID, "", c); w.Code != 404 {
		t.Fatal("expired session", w.Code)
	}
}

func TestMaliciousUploadEndpointAndPrivateMetadata(t *testing.T) {
	s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://attacker.test/upload/stolen")
	})
	c := guest(t, s)
	if w := request(s, "POST", "/wedding/media/uploads", `{"name":"a.jpg","mime_type":"image/jpeg","size":8}`, c); w.Code != 503 {
		t.Fatal("untrusted session URL", w.Code)
	}
	w := request(s, "GET", "/wedding/media/status", "", c)
	for _, secret := range []string{"secret", "refresh", "folder", "token", "cookie_key"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("secret in status", secret)
		}
	}
	s.Settings = func() (bool, string, error) { return true, "changed", nil }
	if w = request(s, "GET", "/wedding/media/a", "", c); w.Code != 401 {
		t.Fatal("metadata auth", w.Code)
	}
}

func TestUploadCreationRateLimit(t *testing.T) {
	var target string
	calls := 0
	s, g := testService(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Location", target+"/upload/new")
	})
	target = g.URL
	c := guest(t, s)
	for i := 0; i < 61; i++ {
		w := request(s, "POST", "/wedding/media/uploads", `{"name":"a.jpg","mime_type":"image/jpeg","size":8}`, c)
		if i < 60 && w.Code != 200 {
			t.Fatal(i, w.Code, w.Body.String())
		}
		if i == 60 && w.Code != 429 {
			t.Fatal("creation rate unbounded", w.Code, calls)
		}
	}
	if calls != 60 {
		t.Fatal("limited requests reached Google", calls)
	}
}

func TestSessionDatabaseMustBePrivate(t *testing.T) {
	s, g := testService(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("unexpected network") })
	path := filepath.Join(t.TempDir(), "drive.json")
	if err := os.WriteFile(path+".sessions.db", nil, 0644); err != nil {
		t.Fatal(err)
	}
	reopened, err := newService(s.cfg, path, g.Client(), g.URL, g.URL+"/token")
	if err == nil {
		reopened.Close()
		t.Fatal("world-readable upload bearer URLs accepted")
	}
}

func TestFirstChunkIsSniffedOnEveryRetry(t *testing.T) {
	var target string
	puts := 0
	s, g := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/upload/drive/v3/files" {
			w.Header().Set("Location", target+"/upload/retry")
			return
		}
		puts++
		w.WriteHeader(503)
	})
	target = g.URL
	c := guest(t, s)
	w := request(s, "POST", "/wedding/media/uploads", `{"name":"a.jpg","mime_type":"image/jpeg","size":8}`, c)
	var state UploadState
	json.Unmarshal(w.Body.Bytes(), &state)
	put := func(data []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest("PUT", "https://wedding.test/wedding/media/uploads/"+state.ID, bytes.NewReader(data))
		r.Header.Set("Origin", "https://wedding.test")
		r.Header.Set("Content-Range", "bytes 0-7/8")
		r.AddCookie(c)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	if w = put([]byte{255, 216, 255, 224, 0, 0, 0, 0}); w.Code != 503 {
		t.Fatal(w.Code)
	}
	if w = put([]byte("MZevil!!")); w.Code != 415 || puts != 1 {
		t.Fatal("retry bypassed sniffing", w.Code, puts)
	}
}

func TestGalleryCacheInvalidatedByCompletion(t *testing.T) {
	var target string
	lists := 0
	s, g := testService(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/drive/v3/files":
			lists++
			fmt.Fprint(w, `{"files":[]}`)
		case "/upload/drive/v3/files":
			w.Header().Set("Location", target+"/upload/cache")
		default:
			fmt.Fprint(w, `{"id":"done","mimeType":"image/jpeg","size":"8","parents":["folder"]}`)
		}
	})
	target = g.URL
	c := guest(t, s)
	request(s, "GET", "/wedding/media", "", c)
	request(s, "GET", "/wedding/media", "", c)
	if lists != 1 {
		t.Fatal("list not cached", lists)
	}
	w := request(s, "POST", "/wedding/media/uploads", `{"name":"a.jpg","mime_type":"image/jpeg","size":8}`, c)
	var state UploadState
	json.Unmarshal(w.Body.Bytes(), &state)
	r := httptest.NewRequest("PUT", "https://wedding.test/wedding/media/uploads/"+state.ID, bytes.NewReader([]byte{255, 216, 255, 224, 0, 0, 0, 0}))
	r.Header.Set("Origin", "https://wedding.test")
	r.Header.Set("Content-Range", "bytes 0-7/8")
	r.AddCookie(c)
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	request(s, "GET", "/wedding/media", "", c)
	if lists != 2 {
		t.Fatal("stale list after completion", lists)
	}
}

type mediaRoundTripper func(*http.Request) (*http.Response, error)

func (f mediaRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestThumbnailCacheStillChecksFolderEachTime(t *testing.T) {
	thumbnails, metadata := 0, 0
	foreign := false
	s, g := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/thumbnail" {
			thumbnails++
			w.Header().Set("Content-Type", "image/jpeg")
			fmt.Fprint(w, "jpg")
			return
		}
		metadata++
		parent := "folder"
		if foreign {
			parent = "other"
		}
		fmt.Fprintf(w, `{"id":"photo","mimeType":"image/jpeg","size":"8","parents":[%q],"thumbnailLink":"https://lh3.googleusercontent.com/thumbnail"}`, parent)
	})
	transport := g.Client().Transport
	s.client.Transport = mediaRoundTripper(func(r *http.Request) (*http.Response, error) {
		copyRequest := r.Clone(r.Context())
		copyURL := *r.URL
		copyRequest.URL = &copyURL
		if copyURL.Host == "lh3.googleusercontent.com" {
			target, _ := url.Parse(g.URL)
			copyRequest.URL.Scheme = target.Scheme
			copyRequest.URL.Host = target.Host
		}
		return transport.RoundTrip(copyRequest)
	})
	c := guest(t, s)
	for i := 0; i < 2; i++ {
		if w := request(s, "GET", "/wedding/media/photo/thumbnail", "", c); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if thumbnails != 1 || metadata != 2 {
		t.Fatal("thumbnail cache", thumbnails, metadata)
	}
	foreign = true
	if w := request(s, "GET", "/wedding/media/photo/thumbnail", "", c); w.Code != 404 {
		t.Fatal("cached foreign file leaked", w.Code)
	}
}
