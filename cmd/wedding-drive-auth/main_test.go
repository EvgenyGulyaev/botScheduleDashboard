package main

import (
	"botDashboard/internal/weddingmedia"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAuthorizationURLUsesOfflineDriveFileAndPKCE(t *testing.T) {
	u, err := url.Parse(authorizationURL("client", "http://127.0.0.1:1234/callback", "state", "verifier", false))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("scope") != "https://www.googleapis.com/auth/drive.file" || q.Get("access_type") != "offline" || q.Get("state") != "state" || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "verifier" {
		t.Fatal(q)
	}
}
func TestAuthorizationURLCanReadDirectDriveUploads(t *testing.T) {
	u, err := url.Parse(authorizationURL("client", "http://127.0.0.1:1234/callback", "state", "verifier", true))
	if err != nil {
		t.Fatal(err)
	}
	if got := u.Query().Get("scope"); got != "https://www.googleapis.com/auth/drive.file https://www.googleapis.com/auth/drive.readonly" {
		t.Fatal(got)
	}
}

func TestReconnectPreservesAlbumAndVideoConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drive.json")
	want := weddingmedia.Config{
		ClientID: "client", ClientSecret: "secret", RefreshToken: "refresh",
		FolderID: "album", CookieKey: "cookie", Origin: "https://wedding.example",
		VideoProcessing:  weddingmedia.VideoProcessingConfig{Enabled: true, FolderID: "previews", WorkDir: "/private/work"},
		VideoPreviews:    map[string]string{"original": "preview"},
		VideoHLSPreviews: map[string]weddingmedia.VideoHLSPreview{"original": {ID: "hls", Playlist: "playlist"}},
	}
	if err := weddingmedia.SaveConfig(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := readReconnectConfig(path)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("configuration not preserved: %v", err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readReconnectConfig(path); err == nil {
		t.Fatal("accepted unprotected credentials")
	}
}

func TestReconnectRejectsIncompleteConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drive.json")
	if err := weddingmedia.SaveConfig(path, weddingmedia.Config{ClientID: "client"}); err != nil {
		t.Fatal(err)
	}
	if _, err := readReconnectConfig(path); err == nil {
		t.Fatal("accepted incomplete configuration")
	}
}

func TestOAuthCallbackRejectsWrongState(t *testing.T) {
	results := make(chan callbackResult, 1)
	handler := callbackHandler("expected", results)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1/callback?state=bad&code=stolen", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatal(w.Code)
	}
	select {
	case <-results:
		t.Fatal("bad state accepted")
	default:
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1/callback?state=expected&code=ok", nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if result := <-results; result.code != "ok" {
		t.Fatal(result)
	}
}
