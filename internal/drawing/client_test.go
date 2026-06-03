package drawing

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	nethttp "net/http"
	nethttptest "net/http/httptest"
	"strings"
	"testing"

	"botDashboard/internal/model"
)

const testToken = "secret-token"

func newTestClient(t *testing.T, handler nethttp.Handler) (*Client, *nethttptest.Server) {
	t.Helper()
	srv := nethttptest.NewServer(handler)
	cfg := Config{BaseURL: srv.URL, ServiceToken: testToken}
	return NewClient(cfg), srv
}

func TestListImages(t *testing.T) {
	c, srv := newTestClient(t, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Header.Get("X-Service-Token") != testToken {
			t.Fatalf("missing service token")
		}
		if r.Header.Get("X-User-Email") != "u@e.com" {
			t.Fatalf("missing user email")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"id":"1","title":"x","mime_type":"image/png","size":10,"width":1,"height":1,"created_by":"u","updated_by":"u","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}]}`))
	}))
	defer srv.Close()
	items, err := c.ListImages(context.Background(), model.UserData{Email: "u@e.com", Login: "u"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 || items[0].ID != "1" {
		t.Fatalf("unexpected: %#v", items)
	}
}

func TestCreateMultipart(t *testing.T) {
	var gotContentType string
	var gotBody []byte
	var gotToken string
	c, srv := newTestClient(t, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotToken = r.Header.Get("X-Service-Token")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ImageItem{ID: "42", Title: "x"})
	}))
	defer srv.Close()
	item, err := c.CreateImage(context.Background(), model.UserData{Email: "u@e.com", Login: "u"}, CreatePayload{
		Title:  "img",
		Width:  100,
		Height: 50,
		Body:   strings.NewReader("PNGDATA"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if item.ID != "42" {
		t.Fatalf("expected 42, got %q", item.ID)
	}
	if gotToken != testToken {
		t.Fatalf("missing service token")
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data") {
		t.Fatalf("expected multipart, got %q", gotContentType)
	}
	boundary := strings.Split(gotContentType, "boundary=")[1]
	mr := multipart.NewReader(bytes.NewReader(gotBody), boundary)
	part, err := mr.NextPart()
	if err != nil {
		t.Fatalf("next part: %v", err)
	}
	if part.FormName() != "metadata" {
		t.Fatalf("expected metadata first, got %q", part.FormName())
	}
}

func TestErrorFromResponse(t *testing.T) {
	c, srv := newTestClient(t, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"message":"bad"}`))
	}))
	defer srv.Close()
	_, err := c.GetImage(context.Background(), model.UserData{Email: "u"}, "x")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Fatalf("expected message, got %v", err)
	}
}
