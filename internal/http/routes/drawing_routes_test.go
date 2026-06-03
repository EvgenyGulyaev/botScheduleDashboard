package routes_test

import (
	"bytes"
	"io"
	"mime/multipart"
	nethttp "net/http"
	nethttptest "net/http/httptest"
	"strings"
	"sync"
	"testing"

	"botDashboard/internal/drawing"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"botDashboard/pkg/singleton"
)

type fakeDrawing struct {
	mu         sync.Mutex
	lastPath   string
	lastMethod string
	lastToken  string
	lastEmail  string
	handler    nethttp.HandlerFunc
}

func newFakeDrawing() *fakeDrawing {
	f := &fakeDrawing{}
	f.handler = func(w nethttp.ResponseWriter, r *nethttp.Request) {
		f.mu.Lock()
		f.lastPath = r.URL.Path
		f.lastMethod = r.Method
		f.lastToken = r.Header.Get("X-Service-Token")
		f.lastEmail = r.Header.Get("X-User-Email")
		f.mu.Unlock()
		w.WriteHeader(nethttp.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}
	return f
}

func (f *fakeDrawing) serve(w nethttp.ResponseWriter, r *nethttp.Request) {
	f.handler(w, r)
}

func installDrawingClient(baseURL, token string) {
	singleton.Set("drawing-client", drawing.NewClient(drawing.Config{BaseURL: baseURL, ServiceToken: token}))
}

func grantDrawingPermission(t *testing.T, user model.UserData) model.UserData {
	t.Helper()
	user.AppPermissions = []string{model.DefaultAppChat, model.DefaultAppDrawing}
	if err := store.GetUserRepository().UpdateUser(user, user.Email); err != nil {
		t.Fatalf("save user: %v", err)
	}
	return user
}

func startFake(t *testing.T, f *fakeDrawing) *nethttptest.Server {
	t.Helper()
	srv := nethttptest.NewServer(nethttp.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return srv
}

func doMultipart(t *testing.T, method, path, token string, body io.Reader, contentType string) (*nethttp.Response, []byte) {
	t.Helper()
	req, err := nethttp.NewRequest(method, chatHTTPURL+path, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

func TestDrawingRoutesUnauthenticated(t *testing.T) {
	chatHTTPSetup(t)
	installDrawingClient("http://127.0.0.1:1", "x")
	resp, _ := doJSONRequest(t, nethttp.MethodGet, "/drawing/images", "", nil)
	if resp.StatusCode != nethttp.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestDrawingRoutesListPassesUserContext(t *testing.T) {
	chatHTTPSetup(t)
	fake := newFakeDrawing()
	srv := startFake(t, fake)
	installDrawingClient(srv.URL, "service-token")

	user := grantDrawingPermission(t, createTestUser(t, "user", "user@example.com"))
	token := authToken(t, user.Email, user.Login)

	resp, _ := doJSONRequest(t, nethttp.MethodGet, "/drawing/images", token, nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.lastToken != "service-token" {
		t.Fatalf("expected service-token, got %q", fake.lastToken)
	}
	if fake.lastEmail != user.Email {
		t.Fatalf("expected email, got %q", fake.lastEmail)
	}
}

func TestDrawingRoutesRejectUserWithoutDrawingPermission(t *testing.T) {
	chatHTTPSetup(t)
	fake := newFakeDrawing()
	srv := startFake(t, fake)
	installDrawingClient(srv.URL, "service-token")

	user := createTestUser(t, "user", "user@example.com")
	user.AppPermissions = []string{model.DefaultAppChat}
	if err := store.GetUserRepository().UpdateUser(user, user.Email); err != nil {
		t.Fatalf("save user: %v", err)
	}
	token := authToken(t, user.Email, user.Login)

	resp, _ := doJSONRequest(t, nethttp.MethodGet, "/drawing/images", token, nil)
	if resp.StatusCode != nethttp.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestDrawingRoutesCreateMultipart(t *testing.T) {
	chatHTTPSetup(t)
	fake := newFakeDrawing()
	srv := startFake(t, fake)
	installDrawingClient(srv.URL, "service-token")

	user := grantDrawingPermission(t, createTestUser(t, "user", "user@example.com"))
	token := authToken(t, user.Email, user.Login)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("metadata", `{"title":"my","width":100,"height":50}`)
	fw, _ := mw.CreateFormFile("file", "test.png")
	fw.Write([]byte("PNGDATA"))
	mw.Close()

	resp, data := doMultipart(t, nethttp.MethodPost, "/drawing/images", token, &buf, mw.FormDataContentType())
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.lastMethod != nethttp.MethodPost {
		t.Fatalf("expected POST, got %q", fake.lastMethod)
	}
	if fake.lastPath != "/internal/drawing/images" {
		t.Fatalf("expected /internal/drawing/images, got %q", fake.lastPath)
	}
}

func TestDrawingRoutesUpdateMultipart(t *testing.T) {
	chatHTTPSetup(t)
	fake := newFakeDrawing()
	srv := startFake(t, fake)
	installDrawingClient(srv.URL, "service-token")

	user := grantDrawingPermission(t, createTestUser(t, "user", "user@example.com"))
	token := authToken(t, user.Email, user.Login)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("metadata", `{"title":"x","width":10,"height":10}`)
	fw, _ := mw.CreateFormFile("file", "x.png")
	fw.Write([]byte("X"))
	mw.Close()

	resp, data := doMultipart(t, nethttp.MethodPut, "/drawing/images/00000000000000000001", token, &buf, mw.FormDataContentType())
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestDrawingRoutesDeleteReturns204(t *testing.T) {
	chatHTTPSetup(t)
	fake := newFakeDrawing()
	srv := startFake(t, fake)
	installDrawingClient(srv.URL, "service-token")

	user := grantDrawingPermission(t, createTestUser(t, "user", "user@example.com"))
	token := authToken(t, user.Email, user.Login)

	resp, _ := doJSONRequest(t, nethttp.MethodDelete, "/drawing/images/00000000000000000001", token, nil)
	if resp.StatusCode != nethttp.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
}

func TestDrawingRoutesUpstreamErrorPropagates(t *testing.T) {
	chatHTTPSetup(t)
	fake := newFakeDrawing()
	fake.handler = func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.WriteHeader(nethttp.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"missing"}`))
	}
	srv := startFake(t, fake)
	installDrawingClient(srv.URL, "service-token")

	user := grantDrawingPermission(t, createTestUser(t, "user", "user@example.com"))
	token := authToken(t, user.Email, user.Login)

	resp, data := doJSONRequest(t, nethttp.MethodGet, "/drawing/images/00000000000000000099", token, nil)
	if resp.StatusCode != nethttp.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(data))
	}
	if !strings.Contains(string(data), "missing") {
		t.Fatalf("expected missing in body, got %s", string(data))
	}
}

func TestDrawingRoutesTokenMismatch(t *testing.T) {
	chatHTTPSetup(t)
	fake := newFakeDrawing()
	fake.handler = func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Header.Get("X-Service-Token") != "expected-token" {
			w.WriteHeader(nethttp.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"bad token"}`))
			return
		}
		w.WriteHeader(nethttp.StatusOK)
	}
	srv := startFake(t, fake)
	// Install client with token that fake rejects
	installDrawingClient(srv.URL, "wrong-token")

	user := grantDrawingPermission(t, createTestUser(t, "user", "user@example.com"))
	token := authToken(t, user.Email, user.Login)

	resp, data := doJSONRequest(t, nethttp.MethodGet, "/drawing/images", token, nil)
	if resp.StatusCode != nethttp.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestDrawingStampRoutesListPassesUserContext(t *testing.T) {
	chatHTTPSetup(t)
	fake := newFakeDrawing()
	srv := startFake(t, fake)
	installDrawingClient(srv.URL, "service-token")

	user := grantDrawingPermission(t, createTestUser(t, "user", "user@example.com"))
	token := authToken(t, user.Email, user.Login)

	resp, _ := doJSONRequest(t, nethttp.MethodGet, "/drawing/stamps", token, nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.lastPath != "/internal/drawing/stamps" {
		t.Fatalf("expected stamp list path, got %q", fake.lastPath)
	}
	if fake.lastEmail != user.Email {
		t.Fatalf("expected email, got %q", fake.lastEmail)
	}
}

func TestDrawingStampRoutesRejectUserWithoutDrawingPermission(t *testing.T) {
	chatHTTPSetup(t)
	fake := newFakeDrawing()
	srv := startFake(t, fake)
	installDrawingClient(srv.URL, "service-token")

	user := createTestUser(t, "user", "user@example.com")
	user.AppPermissions = []string{model.DefaultAppChat}
	if err := store.GetUserRepository().UpdateUser(user, user.Email); err != nil {
		t.Fatalf("save user: %v", err)
	}
	token := authToken(t, user.Email, user.Login)

	resp, _ := doJSONRequest(t, nethttp.MethodGet, "/drawing/stamps", token, nil)
	if resp.StatusCode != nethttp.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestDrawingStampRoutesCreateMultipart(t *testing.T) {
	chatHTTPSetup(t)
	fake := newFakeDrawing()
	srv := startFake(t, fake)
	installDrawingClient(srv.URL, "service-token")

	user := grantDrawingPermission(t, createTestUser(t, "user", "user@example.com"))
	token := authToken(t, user.Email, user.Login)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("metadata", `{"name":"Seal","textValue":"S","priority":"text"}`)
	fw, _ := mw.CreateFormFile("file", "seal.png")
	fw.Write([]byte("PNGDATA"))
	mw.Close()

	resp, data := doMultipart(t, nethttp.MethodPost, "/drawing/stamps", token, &buf, mw.FormDataContentType())
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.lastMethod != nethttp.MethodPost {
		t.Fatalf("expected POST, got %q", fake.lastMethod)
	}
	if fake.lastPath != "/internal/drawing/stamps" {
		t.Fatalf("expected /internal/drawing/stamps, got %q", fake.lastPath)
	}
}
