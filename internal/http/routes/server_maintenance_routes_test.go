package routes_test

import (
	"botDashboard/internal/store"
	"encoding/json"
	nethttp "net/http"
	"testing"
)

type maintenancePlanResponse struct {
	Items                 []maintenanceItemResponse `json:"items"`
	TotalReclaimableBytes uint64                    `json:"total_reclaimable_bytes"`
}

type maintenanceItemResponse struct {
	Key              string `json:"key"`
	ReclaimableBytes uint64 `json:"reclaimable_bytes"`
}

func TestServerMaintenancePreviewRequiresAdmin(t *testing.T) {
	chatHTTPSetup(t)

	createTestUser(t, "alice", "alice@example.com")
	resp, data := doJSONRequest(t, nethttp.MethodGet, "/server/maintenance/preview", authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusUnauthorized && resp.StatusCode != nethttp.StatusForbidden {
		t.Fatalf("expected regular user to be rejected, got %d: %s", resp.StatusCode, string(data))
	}

	admin := createTestUser(t, "panel-admin", "panel-admin@example.com")
	admin.IsAdmin = true
	if err := store.GetUserRepository().UpdateUser(admin, admin.Email); err != nil {
		t.Fatalf("update admin user: %v", err)
	}
	resp, data = doJSONRequest(t, nethttp.MethodGet, "/server/maintenance/preview", authToken(t, admin.Email, admin.Login), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected admin preview access, got %d: %s", resp.StatusCode, string(data))
	}

	resp, data = doJSONRequest(t, nethttp.MethodPost, "/server/maintenance/cleanup", authToken(t, admin.Email, admin.Login), map[string]any{
		"items": []string{},
	})
	if resp.StatusCode != nethttp.StatusUnauthorized && resp.StatusCode != nethttp.StatusForbidden {
		t.Fatalf("expected regular admin cleanup to be rejected, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestServerMaintenancePreviewAndNoopCleanup(t *testing.T) {
	chatHTTPSetup(t)

	super := createTestUser(t, "root", "root@example.com")
	super.IsAdmin = true
	super.IsSuperAdmin = true
	if err := store.GetUserRepository().UpdateUser(super, super.Email); err != nil {
		t.Fatalf("update super user: %v", err)
	}
	token := authToken(t, super.Email, super.Login)

	resp, data := doJSONRequest(t, nethttp.MethodGet, "/server/maintenance/preview", token, nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 preview, got %d: %s", resp.StatusCode, string(data))
	}
	var preview maintenancePlanResponse
	if err := json.Unmarshal(data, &preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if len(preview.Items) == 0 {
		t.Fatalf("expected maintenance items, got %#v", preview)
	}

	resp, data = doJSONRequest(t, nethttp.MethodPost, "/server/maintenance/cleanup", token, map[string]any{
		"items": []string{},
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 noop cleanup, got %d: %s", resp.StatusCode, string(data))
	}
}
