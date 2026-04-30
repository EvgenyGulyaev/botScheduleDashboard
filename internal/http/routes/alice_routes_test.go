package routes_test

import (
	"botDashboard/internal/model"
	"encoding/json"
	"net/http"
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"botDashboard/internal/store"
)

func TestPostAliceAnnounceTestRejectsUserWithoutAlicePermission(t *testing.T) {
	chatHTTPSetup(t)
	createTestUser(t, "alice", "alice@example.com")

	resp, data := doJSONRequest(t, nethttp.MethodPost, "/alice/announce/test", authToken(t, "alice@example.com", "alice"), map[string]any{
		"text":       "hello from admin endpoint",
		"account_id": "acc-1",
		"device_id":  "device-1",
	})
	if resp.StatusCode != nethttp.StatusForbidden {
		t.Fatalf("expected 403 for user without alice permission, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestPostAliceAnnounceTestSendsPayloadToAliceService(t *testing.T) {
	chatHTTPSetup(t)

	admin := createTestUser(t, "alice", "alice@example.com")
	admin.AppPermissions = []string{model.DefaultAppChat, model.DefaultAppAlice}
	if err := store.GetUserRepository().UpdateUser(admin, admin.Email); err != nil {
		t.Fatalf("update alice user: %v", err)
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

func TestPostAliceAnnounceResendsAudioNoticeToGroupRecipients(t *testing.T) {
	chatHTTPSetup(t)

	aliceUser := createTestUser(t, "alice", "alice@example.com")
	bob := createTestUser(t, "bob", "bob@example.com")
	carol := createTestUser(t, "carol", "carol@example.com")
	aliceUser.AppPermissions = []string{model.DefaultAppChat, model.DefaultAppAlice}
	if err := store.GetUserRepository().UpdateUser(aliceUser, aliceUser.Email); err != nil {
		t.Fatalf("update alice: %v", err)
	}

	bob.AppPermissions = []string{model.DefaultAppChat, model.DefaultAppAlice}
	bob.AliceSettings.AccountID = "acc-1"
	bob.AliceSettings.HouseholdID = "home-1"
	bob.AliceSettings.DeviceID = "shared-speaker"
	bob.AliceSettings.AnnounceSender = true
	if err := store.GetUserRepository().UpdateUser(bob, bob.Email); err != nil {
		t.Fatalf("update bob: %v", err)
	}

	carol.AppPermissions = []string{model.DefaultAppChat, model.DefaultAppAlice}
	carol.AliceSettings.AccountID = "acc-1"
	carol.AliceSettings.HouseholdID = "home-1"
	carol.AliceSettings.DeviceID = "shared-speaker"
	carol.AliceSettings.AnnounceSender = true
	if err := store.GetUserRepository().UpdateUser(carol, carol.Email); err != nil {
		t.Fatalf("update carol: %v", err)
	}

	repo := store.GetChatRepository()
	conversation, err := repo.CreateGroupConversation("Team", []model.ChatMember{
		{Email: aliceUser.Email, Login: aliceUser.Login},
		{Email: bob.Email, Login: bob.Login},
		{Email: carol.Email, Login: carol.Login},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	audioPath := filepath.Join(t.TempDir(), "voice.webm")
	if err := os.WriteFile(audioPath, []byte("voice"), 0600); err != nil {
		t.Fatalf("write audio file: %v", err)
	}

	audioResult, err := repo.AddAudioMessageWithResult(conversation.ID, aliceUser.Email, aliceUser.Login, store.ChatAudioUpload{
		FilePath:        audioPath,
		MimeType:        "audio/webm",
		SizeBytes:       5,
		DurationSeconds: 2,
	})
	if err != nil {
		t.Fatalf("add audio message: %v", err)
	}

	received := make([]struct {
		Text           string `json:"text"`
		RecipientEmail string `json:"recipient_email"`
		DeviceID       string `json:"device_id"`
	}, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Text           string `json:"text"`
			RecipientEmail string `json:"recipient_email"`
			DeviceID       string `json:"device_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		received = append(received, payload)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"sent","request_id":"req-announce","delivery_id":"delivery-announce"}`))
	}))
	defer server.Close()

	t.Setenv("ALICE_SERVICE_URL", server.URL)
	t.Setenv("ALICE_SERVICE_TOKEN", "token")

	resp, data := doJSONRequest(t, nethttp.MethodPost, "/alice/announce", authToken(t, "alice@example.com", "alice"), map[string]any{
		"conversation_id": conversation.ID,
		"message_id":      audioResult.Message.ID,
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}
	if len(received) != 1 {
		t.Fatalf("expected one deduplicated Alice delivery, got %#v", received)
	}
	if received[0].Text != "Передано от alice. Вам пришло голосовое сообщение" {
		t.Fatalf("expected voice notice on resend, got %#v", received)
	}
	if received[0].DeviceID != "shared-speaker" {
		t.Fatalf("unexpected target device: %#v", received)
	}
}

func TestPostAliceCleanupScenariosRejectsUserWithoutAlicePermission(t *testing.T) {
	chatHTTPSetup(t)
	createTestUser(t, "alice", "alice@example.com")

	resp, data := doJSONRequest(t, nethttp.MethodPost, "/alice/cleanup-scenarios", authToken(t, "alice@example.com", "alice"), map[string]any{
		"account_id": "acc-1",
	})
	if resp.StatusCode != nethttp.StatusForbidden {
		t.Fatalf("expected 403 for user without alice permission, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestPostAliceCleanupScenariosSendsAccountAndDeviceToAliceService(t *testing.T) {
	chatHTTPSetup(t)

	admin := createTestUser(t, "alice", "alice@example.com")
	admin.AppPermissions = []string{model.DefaultAppChat, model.DefaultAppAlice}
	if err := store.GetUserRepository().UpdateUser(admin, admin.Email); err != nil {
		t.Fatalf("update alice user: %v", err)
	}

	var received struct {
		DeviceID string `json:"device_id"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != nethttp.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/api/accounts/acc-1/cleanup-scenarios" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","account_id":"acc-1","device_id":"device-1","deleted":3}`))
	}))
	defer server.Close()

	t.Setenv("ALICE_SERVICE_URL", server.URL)
	t.Setenv("ALICE_SERVICE_TOKEN", "token")

	resp, data := doJSONRequest(t, nethttp.MethodPost, "/alice/cleanup-scenarios", authToken(t, "alice@example.com", "alice"), map[string]any{
		"account_id": "acc-1",
		"device_id":  "device-1",
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}
	if received.DeviceID != "device-1" {
		t.Fatalf("unexpected cleanup payload: %#v", received)
	}

	var response struct {
		Status   string `json:"status"`
		Account  string `json:"account_id"`
		DeviceID string `json:"device_id"`
		Deleted  int    `json:"deleted"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Status != "ok" || response.Account != "acc-1" || response.DeviceID != "device-1" || response.Deleted != 3 {
		t.Fatalf("unexpected cleanup response: %#v", response)
	}
}
