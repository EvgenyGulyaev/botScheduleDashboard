package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestAuthorizationURLUsesOfflineDriveFileAndPKCE(t *testing.T) {
	u, err := url.Parse(authorizationURL("client", "http://127.0.0.1:1234/callback", "state", "verifier"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("scope") != "https://www.googleapis.com/auth/drive.file" || q.Get("access_type") != "offline" || q.Get("state") != "state" || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "verifier" {
		t.Fatal(q)
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
