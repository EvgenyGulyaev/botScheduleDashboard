package routes_test

import (
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"encoding/json"
	nethttp "net/http"
	"reflect"
	"testing"
)

func TestWeddingPublicSettingsAndCreateRSVP(t *testing.T) {
	chatHTTPSetup(t)
	if err := store.GetWeddingRepository().ClearAll(); err != nil {
		t.Fatalf("clear wedding data: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodGet, "/wedding/public-settings", "", nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected public settings 200, got %d: %s", resp.StatusCode, string(data))
	}
	var settings model.WeddingSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("decode public settings: %v", err)
	}
	if !reflect.DeepEqual(settings.DrinkOptions, model.DefaultWeddingDrinkOptions()) {
		t.Fatalf("expected default drinks, got %#v", settings.DrinkOptions)
	}

	resp, data = doJSONRequest(t, nethttp.MethodPost, "/wedding/rsvps", "", map[string]any{
		"full_name":   "  Анна Иванова  ",
		"attendance":  model.WeddingAttendanceAttending,
		"drinks":      []string{"Белое сухое", "Другое"},
		"other_drink": "Сидр",
		"song":        "ABBA - Dancing Queen",
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected create rsvp 200, got %d: %s", resp.StatusCode, string(data))
	}
	var created model.WeddingRSVP
	if err := json.Unmarshal(data, &created); err != nil {
		t.Fatalf("decode created rsvp: %v", err)
	}
	if created.ID == "" || created.FullName != "Анна Иванова" || created.CreatedAt.IsZero() {
		t.Fatalf("unexpected created rsvp: %#v", created)
	}
}

func TestWeddingRSVPValidation(t *testing.T) {
	chatHTTPSetup(t)
	if err := store.GetWeddingRepository().ClearAll(); err != nil {
		t.Fatalf("clear wedding data: %v", err)
	}

	for _, body := range []map[string]any{
		{"attendance": model.WeddingAttendanceAttending},
		{"full_name": "Анна Иванова", "attendance": "maybe"},
	} {
		resp, data := doJSONRequest(t, nethttp.MethodPost, "/wedding/rsvps", "", body)
		if resp.StatusCode != nethttp.StatusBadRequest {
			t.Fatalf("expected validation 400, got %d: %s", resp.StatusCode, string(data))
		}
	}
}

func TestWeddingAdminRoutesRequireWeddingPermission(t *testing.T) {
	chatHTTPSetup(t)
	if err := store.GetWeddingRepository().ClearAll(); err != nil {
		t.Fatalf("clear wedding data: %v", err)
	}

	resp, _ := doJSONRequest(t, nethttp.MethodGet, "/wedding/rsvps", "", nil)
	if resp.StatusCode != nethttp.StatusUnauthorized {
		t.Fatalf("expected no token to be unauthorized, got %d", resp.StatusCode)
	}

	user := createTestUser(t, "guest-admin", "guest-admin@example.com")
	user.AppPermissions = []string{model.DefaultAppChat}
	if err := store.GetUserRepository().UpdateUser(user, ""); err != nil {
		t.Fatalf("update user permissions: %v", err)
	}
	resp, data := doJSONRequest(t, nethttp.MethodGet, "/wedding/rsvps", authToken(t, user.Email, user.Login), nil)
	if resp.StatusCode != nethttp.StatusForbidden {
		t.Fatalf("expected missing wedding permission 403, got %d: %s", resp.StatusCode, string(data))
	}

	user.AppPermissions = []string{model.DefaultAppChat, model.DefaultAppWedding}
	if err := store.GetUserRepository().UpdateUser(user, ""); err != nil {
		t.Fatalf("update user permissions: %v", err)
	}
	resp, data = doJSONRequest(t, nethttp.MethodGet, "/wedding/rsvps", authToken(t, user.Email, user.Login), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected wedding permission 200, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestWeddingAdminSettingsUpdateAffectsPublicSettings(t *testing.T) {
	chatHTTPSetup(t)
	if err := store.GetWeddingRepository().ClearAll(); err != nil {
		t.Fatalf("clear wedding data: %v", err)
	}
	user := createTestUser(t, "wedding-admin", "wedding-admin@example.com")
	token := authToken(t, user.Email, user.Login)

	resp, data := doJSONRequest(t, nethttp.MethodPatch, "/wedding/settings", token, map[string]any{
		"drink_options": []string{"  Вино  ", "Сидр", "Вино"},
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected settings update 200, got %d: %s", resp.StatusCode, string(data))
	}
	var settings model.WeddingSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if !reflect.DeepEqual(settings.DrinkOptions, []string{"Вино", "Сидр"}) {
		t.Fatalf("expected normalized settings, got %#v", settings.DrinkOptions)
	}

	resp, data = doJSONRequest(t, nethttp.MethodGet, "/wedding/public-settings", "", nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected public settings 200, got %d: %s", resp.StatusCode, string(data))
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("decode public settings: %v", err)
	}
	if !reflect.DeepEqual(settings.DrinkOptions, []string{"Вино", "Сидр"}) {
		t.Fatalf("expected public settings to use saved drinks, got %#v", settings.DrinkOptions)
	}

	resp, data = doJSONRequest(t, nethttp.MethodPatch, "/wedding/settings", token, map[string]any{
		"drink_options": []string{},
	})
	if resp.StatusCode != nethttp.StatusBadRequest {
		t.Fatalf("expected empty settings 400, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestWeddingAdminCanDeleteRSVP(t *testing.T) {
	chatHTTPSetup(t)
	if err := store.GetWeddingRepository().ClearAll(); err != nil {
		t.Fatalf("clear wedding data: %v", err)
	}
	user := createTestUser(t, "wedding-delete-admin", "wedding-delete-admin@example.com")
	token := authToken(t, user.Email, user.Login)

	created, err := store.GetWeddingRepository().CreateRSVP(model.WeddingRSVP{
		FullName:   "Тестовая заявка",
		Attendance: model.WeddingAttendanceAttending,
	})
	if err != nil {
		t.Fatalf("create rsvp: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodDelete, "/wedding/rsvps/"+created.ID, token, nil)
	if resp.StatusCode != nethttp.StatusNoContent {
		t.Fatalf("expected delete rsvp 204, got %d: %s", resp.StatusCode, string(data))
	}

	items, err := store.GetWeddingRepository().ListRSVPs()
	if err != nil {
		t.Fatalf("list rsvps: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected rsvp to be deleted, got %#v", items)
	}

	resp, data = doJSONRequest(t, nethttp.MethodDelete, "/wedding/rsvps/"+created.ID, token, nil)
	if resp.StatusCode != nethttp.StatusNotFound {
		t.Fatalf("expected duplicate delete 404, got %d: %s", resp.StatusCode, string(data))
	}
}
