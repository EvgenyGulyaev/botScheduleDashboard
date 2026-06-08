package routes_test

import (
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	nethttp "net/http"
	"strings"
	"testing"
)

func TestProxyRoutesRequireProxyAppPermission(t *testing.T) {
	chatHTTPSetup(t)
	t.Setenv("VPN_GATEWAY_URL", "")
	t.Setenv("VPN_GATEWAY_API_TOKEN", "")

	withoutProxy := createTestUser(t, "viewer", "viewer@example.com")
	withoutProxy.AppPermissions = []string{model.DefaultAppChat}
	if err := store.GetUserRepository().UpdateUser(withoutProxy, ""); err != nil {
		t.Fatalf("update user without proxy: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodGet, "/proxy/runtime/status", authToken(t, withoutProxy.Email, withoutProxy.Login), nil)
	if resp.StatusCode != nethttp.StatusUnauthorized {
		t.Fatalf("expected 401 without proxy permission, got %d: %s", resp.StatusCode, string(data))
	}

	withProxy := createTestUser(t, "proxy-editor", "proxy-editor@example.com")
	withProxy.AppPermissions = []string{model.DefaultAppChat, model.DefaultAppProxy}
	if err := store.GetUserRepository().UpdateUser(withProxy, ""); err != nil {
		t.Fatalf("update user with proxy: %v", err)
	}

	resp, data = doJSONRequest(t, nethttp.MethodGet, "/proxy/runtime/status", authToken(t, withProxy.Email, withProxy.Login), nil)
	if resp.StatusCode == nethttp.StatusUnauthorized {
		t.Fatalf("expected proxy permission to pass auth gate, got %d: %s", resp.StatusCode, string(data))
	}
	if resp.StatusCode != nethttp.StatusBadGateway || !strings.Contains(string(data), "proxy service is not configured") {
		t.Fatalf("expected request to reach proxy client, got %d: %s", resp.StatusCode, string(data))
	}
}
