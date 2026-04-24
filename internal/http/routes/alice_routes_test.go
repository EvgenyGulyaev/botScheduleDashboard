package routes_test

import (
	"encoding/json"
	"net/http"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"botDashboard/internal/store"
)

func TestPostAliceAnnounceTestRejectsNonAdmin(t *testing.T) {
	chatHTTPSetup(t)
	createTestUser(t, "alice", "alice@example.com")

	resp, data := doJSONRequest(t, nethttp.MethodPost, "/alice/announce/test", authToken(t, "alice@example.com", "alice"), map[string]any{
		"text":       "hello from admin endpoint",
		"account_id": "acc-1",
		"device_id":  "device-1",
	})
	if resp.StatusCode != nethttp.StatusUnauthorized {
		t.Fatalf("expected 401 for non-admin, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestPostAliceAnnounceTestSendsPayloadToAliceService(t *testing.T) {
	chatHTTPSetup(t)

	admin := createTestUser(t, "alice", "alice@example.com")
	admin.IsAdmin = true
	if err := store.GetUserRepository().UpdateUser(admin, admin.Email); err != nil {
		t.Fatalf("update admin user: %v", err)
	}

	var received struct {
		Text        string `json:"text"`
		AccountID   string `json:"account_id"`
		HouseholdID string `json:"household_id"`
		RoomID      string `json:"room_id"`
		DeviceID    string `json:"device_id"`
		Voice       string `json:"voice"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != nethttp.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/api/announce/scenario" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"sent","request_id":"req-1","delivery_id":"delivery-1"}`))
	}))
	defer server.Close()

	t.Setenv("ALICE_SERVICE_URL", server.URL)
	t.Setenv("ALICE_SERVICE_TOKEN", "token")

	resp, data := doJSONRequest(t, nethttp.MethodPost, "/alice/announce/test", authToken(t, "alice@example.com", "alice"), map[string]any{
		"text":         "hello from admin endpoint",
		"account_id":   "acc-1",
		"household_id": "home-1",
		"room_id":      "room-1",
		"device_id":    "device-1",
		"voice":        "oksana",
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}
	if received.Text != "hello from admin endpoint" || received.AccountID != "acc-1" || received.HouseholdID != "home-1" || received.RoomID != "room-1" || received.DeviceID != "device-1" || received.Voice != "oksana" {
		t.Fatalf("unexpected announce payload: %#v", received)
	}
}
