package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientAddsBearerTokenAndDecodesJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runtime/status" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer service-token" {
			t.Fatalf("unexpected authorization header %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"xray_active":true}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Token: "service-token"})
	response, err := client.Do(context.Background(), http.MethodGet, "/runtime/status", nil)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	if response.Status != http.StatusOK {
		t.Fatalf("unexpected status %d", response.Status)
	}
	payload := response.Data.(map[string]any)
	if payload["xray_active"] != true {
		t.Fatalf("unexpected payload %#v", payload)
	}
}

func TestClientRequiresToken(t *testing.T) {
	client := NewClient(Config{BaseURL: "http://127.0.0.1:1"})
	if _, err := client.Do(context.Background(), http.MethodGet, "/runtime/status", nil); err == nil {
		t.Fatal("expected missing token error")
	}
}
