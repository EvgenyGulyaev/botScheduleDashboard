package routes_test

import (
	"botDashboard/internal/http/middleware"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"encoding/json"
	nethttp "net/http"
	"testing"
)

type profileResponse struct {
	Login         string `json:"login"`
	Email         string `json:"email"`
	IsAdmin       bool   `json:"is_admin"`
	DefaultApp    string `json:"default_app"`
	AliceSettings struct {
		AccountID   string `json:"account_id"`
		HouseholdID string `json:"household_id"`
		RoomID      string `json:"room_id"`
		DeviceID    string `json:"device_id"`
		ScenarioID  string `json:"scenario_id"`
	} `json:"alice_settings"`
	NotificationSettings struct {
		PushEnabled  bool `json:"push_enabled"`
		SoundEnabled bool `json:"sound_enabled"`
		ToastEnabled bool `json:"toast_enabled"`
	} `json:"notification_settings"`
	Push struct {
		Supported bool   `json:"supported"`
		PublicKey string `json:"public_key"`
	} `json:"push"`
}

func TestGetProfileReturnsCurrentUserAndDefaults(t *testing.T) {
	chatHTTPSetup(t)
	createTestUser(t, "alice", "alice@example.com")

	resp, data := doJSONRequest(t, nethttp.MethodGet, "/profile", authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var profile profileResponse
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("unmarshal profile: %v", err)
	}

	if profile.Login != "alice" || profile.Email != "alice@example.com" {
		t.Fatalf("unexpected profile payload: %#v", profile)
	}
	if profile.DefaultApp != "chat" {
		t.Fatalf("expected chat as default app, got %#v", profile)
	}
	if !profile.NotificationSettings.SoundEnabled || !profile.NotificationSettings.ToastEnabled {
		t.Fatalf("expected default local notification settings enabled, got %#v", profile.NotificationSettings)
	}
	if profile.NotificationSettings.PushEnabled {
		t.Fatalf("expected push disabled by default")
	}
}

func TestPatchProfileUpdatesSessionAndNotificationSettings(t *testing.T) {
	chatHTTPSetup(t)
	createTestUser(t, "alice", "alice@example.com")
	if err := store.GetUserRepository().SavePushSubscription("alice@example.com", storeSubscription("https://push.example.com/alice")); err != nil {
		t.Fatalf("save push subscription: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodPatch, "/profile", authToken(t, "alice@example.com", "alice"), map[string]any{
		"login":              "alice-new",
		"email":              "alice-new@example.com",
		"password":           "new-password",
		"default_app":        "dashboard",
		"alice_account_id":   "acc-1",
		"alice_household_id": "home-1",
		"alice_room_id":      "room-1",
		"alice_device_id":    "device-1",
		"alice_scenario_id":  "scenario-1",
		"push_enabled":       true,
		"sound_enabled":      false,
		"toast_enabled":      false,
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	refreshedToken := resp.Header.Get(middleware.RefreshedTokenHeader)
	if refreshedToken == "" {
		t.Fatalf("expected refreshed auth token header")
	}

	var profile profileResponse
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("unmarshal profile: %v", err)
	}
	if profile.Login != "alice-new" || profile.Email != "alice-new@example.com" {
		t.Fatalf("unexpected updated profile: %#v", profile)
	}
	if profile.DefaultApp != "dashboard" {
		t.Fatalf("expected updated default_app, got %#v", profile)
	}
	if profile.AliceSettings.AccountID != "acc-1" || profile.AliceSettings.HouseholdID != "home-1" || profile.AliceSettings.RoomID != "room-1" || profile.AliceSettings.DeviceID != "device-1" || profile.AliceSettings.ScenarioID != "scenario-1" {
		t.Fatalf("expected alice settings in profile, got %#v", profile.AliceSettings)
	}
	if !profile.NotificationSettings.PushEnabled || profile.NotificationSettings.SoundEnabled || profile.NotificationSettings.ToastEnabled {
		t.Fatalf("unexpected notification settings: %#v", profile.NotificationSettings)
	}

	user, err := store.GetUserRepository().FindUserByEmail("alice-new@example.com")
	if err != nil {
		t.Fatalf("find updated user: %v", err)
	}
	if user.Login != "alice-new" {
		t.Fatalf("expected updated login, got %#v", user)
	}

	subscriptions, err := store.GetUserRepository().ListPushSubscriptions("alice-new@example.com")
	if err != nil {
		t.Fatalf("list migrated subscriptions: %v", err)
	}
	if len(subscriptions) != 1 || subscriptions[0].Endpoint != "https://push.example.com/alice" {
		t.Fatalf("expected migrated push subscriptions, got %#v", subscriptions)
	}

	oldSubscriptions, err := store.GetUserRepository().ListPushSubscriptions("alice@example.com")
	if err != nil {
		t.Fatalf("list old subscriptions: %v", err)
	}
	if len(oldSubscriptions) != 0 {
		t.Fatalf("expected old subscriptions to be removed, got %#v", oldSubscriptions)
	}
}

func TestPushSubscriptionEndpointsStoreAndDeleteSubscription(t *testing.T) {
	chatHTTPSetup(t)
	createTestUser(t, "alice", "alice@example.com")

	payload := map[string]any{
		"endpoint":   "https://push.example.com/subscription-1",
		"user_agent": "Safari",
		"keys": map[string]any{
			"p256dh": "demo-p256dh",
			"auth":   "demo-auth",
		},
	}

	resp, data := doJSONRequest(t, nethttp.MethodPost, "/profile/push-subscriptions", authToken(t, "alice@example.com", "alice"), payload)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on create, got %d: %s", resp.StatusCode, string(data))
	}

	subscriptions, err := store.GetUserRepository().ListPushSubscriptions("alice@example.com")
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	if len(subscriptions) != 1 {
		t.Fatalf("expected one subscription, got %#v", subscriptions)
	}
	if subscriptions[0].Endpoint != "https://push.example.com/subscription-1" {
		t.Fatalf("unexpected subscription payload: %#v", subscriptions[0])
	}

	resp, data = doJSONRequest(t, nethttp.MethodDelete, "/profile/push-subscriptions", authToken(t, "alice@example.com", "alice"), map[string]any{
		"endpoint": "https://push.example.com/subscription-1",
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on delete, got %d: %s", resp.StatusCode, string(data))
	}

	subscriptions, err = store.GetUserRepository().ListPushSubscriptions("alice@example.com")
	if err != nil {
		t.Fatalf("list subscriptions after delete: %v", err)
	}
	if len(subscriptions) != 0 {
		t.Fatalf("expected subscriptions to be cleared, got %#v", subscriptions)
	}
}

func storeSubscription(endpoint string) model.PushSubscription {
	return model.PushSubscription{
		Endpoint:  endpoint,
		UserAgent: "test-agent",
		Keys: model.PushSubscriptionKeys{
			P256DH: "demo-p256dh",
			Auth:   "demo-auth",
		},
	}
}
