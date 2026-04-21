package routes_test

import (
	"botDashboard/internal/http/routes"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"encoding/json"
	nethttp "net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestForgotPasswordAlwaysReturnsNeutralResponse(t *testing.T) {
	chatHTTPSetup(t)
	createTestUser(t, "alice", "alice@example.com")

	restoreMail := routes.SetSendMailForTest(func(string, string, string) error { return nil })
	defer restoreMail()

	resp, data := doJSONRequest(t, nethttp.MethodPost, "/auth/forgot-password", "", map[string]any{
		"email": "alice@example.com",
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	resp, data = doJSONRequest(t, nethttp.MethodPost, "/auth/forgot-password", "", map[string]any{
		"email": "missing@example.com",
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 for missing user, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestResetPasswordConsumesTokenAndReturnsSession(t *testing.T) {
	chatHTTPSetup(t)
	createTestUser(t, "alice", "alice@example.com")

	repo := store.GetUserRepository()
	token, err := repo.CreatePasswordResetToken("alice@example.com", 30*time.Minute)
	if err != nil {
		t.Fatalf("create reset token: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodPost, "/auth/reset-password", "", map[string]any{
		"token":    token,
		"password": "brand-new-secret",
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode reset response: %v", err)
	}
	if strings.TrimSpace(payload["token"].(string)) == "" {
		t.Fatalf("expected session token in response")
	}

	resp, _ = doJSONRequest(t, nethttp.MethodPost, "/login", "", map[string]any{
		"email":    "alice@example.com",
		"password": "brand-new-secret",
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected updated password to work, got %d", resp.StatusCode)
	}
}

func TestGoogleAuthCreatesSession(t *testing.T) {
	chatHTTPSetup(t)

	restoreVerify := routes.SetVerifyGoogleIDTokenForTest(func(token string) (routes.GoogleIdentityShim, error) {
		return routes.GoogleIdentityShim{
			Email:         "google@example.com",
			EmailVerified: true,
			Name:          "Google User",
		}, nil
	})
	defer restoreVerify()

	resp, data := doJSONRequest(t, nethttp.MethodPost, "/auth/google", "", map[string]any{
		"id_token": "demo-google-token",
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	user, err := store.GetUserRepository().FindUserByEmail("google@example.com")
	if err != nil {
		t.Fatalf("expected google user to be created: %v", err)
	}
	if user.DefaultApp != model.DefaultAppChat {
		t.Fatalf("expected google user default app to be chat, got %#v", user)
	}
}

func TestGetGoogleConfigReturnsClientID(t *testing.T) {
	chatHTTPSetup(t)
	if err := os.Setenv("GOOGLE_CLIENT_ID", "google-client-id"); err != nil {
		t.Fatalf("set google client id: %v", err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("GOOGLE_CLIENT_ID") })

	resp, data := doJSONRequest(t, nethttp.MethodGet, "/auth/google/config", "", nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode google config: %v", err)
	}
	if payload["client_id"] != "google-client-id" {
		t.Fatalf("unexpected google config payload: %#v", payload)
	}
}
