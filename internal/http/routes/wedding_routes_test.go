package routes_test

import (
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"bytes"
	"encoding/json"
	"io"
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
		"drink_options":       []string{"  Вино  ", "Сидр", "Вино"},
		"access_code_enabled": true,
		"access_code":         "171026",
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
	if !settings.AccessCodeEnabled || settings.AccessCode != "171026" || settings.AccessCodeVersion == "" {
		t.Fatalf("expected access code settings to be saved, got %#v", settings)
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
	var publicPayload map[string]any
	if err := json.Unmarshal(data, &publicPayload); err != nil {
		t.Fatalf("decode public payload: %v", err)
	}
	if _, ok := publicPayload["access_code"]; ok {
		t.Fatalf("public settings must not expose access code: %s", string(data))
	}
	if publicPayload["access_code_enabled"] != true || publicPayload["access_code_version"] == "" {
		t.Fatalf("expected public access gate metadata, got %#v", publicPayload)
	}

	resp, data = doJSONRequest(t, nethttp.MethodPatch, "/wedding/settings", token, map[string]any{
		"drink_options": []string{},
	})
	if resp.StatusCode != nethttp.StatusBadRequest {
		t.Fatalf("expected empty settings 400, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestWeddingAccessVerifyRateLimitsByIP(t *testing.T) {
	chatHTTPSetup(t)
	if err := store.GetWeddingRepository().ClearAll(); err != nil {
		t.Fatalf("clear wedding data: %v", err)
	}
	user := createTestUser(t, "wedding-access-admin", "wedding-access-admin@example.com")
	token := authToken(t, user.Email, user.Login)

	resp, data := doJSONRequest(t, nethttp.MethodPatch, "/wedding/settings", token, map[string]any{
		"drink_options":       model.DefaultWeddingDrinkOptions(),
		"access_code_enabled": true,
		"access_code":         "171026",
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected settings update 200, got %d: %s", resp.StatusCode, string(data))
	}

	resp, data = doJSONRequest(t, nethttp.MethodPost, "/wedding/rsvps", "", map[string]any{
		"full_name":  "Гость без кода",
		"attendance": model.WeddingAttendanceAttending,
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected rsvp submit without access code to stay public, got %d: %s", resp.StatusCode, string(data))
	}

	for attempt := 1; attempt <= 2; attempt++ {
		resp, data = doJSONRequestWithHeaders(t, nethttp.MethodPost, "/wedding/access/verify", "", map[string]any{
			"code": "000000",
		}, map[string]string{"X-Forwarded-For": "203.0.113.10, 10.0.0.2"})
		if resp.StatusCode != nethttp.StatusUnauthorized {
			t.Fatalf("expected wrong code attempt %d to be 401, got %d: %s", attempt, resp.StatusCode, string(data))
		}
	}

	resp, data = doJSONRequestWithHeaders(t, nethttp.MethodPost, "/wedding/access/verify", "", map[string]any{
		"code": "000000",
	}, map[string]string{"X-Forwarded-For": "203.0.113.10"})
	if resp.StatusCode != nethttp.StatusTooManyRequests {
		t.Fatalf("expected third wrong attempt to be 429, got %d: %s", resp.StatusCode, string(data))
	}
	var limited model.WeddingAccessVerifyResult
	if err := json.Unmarshal(data, &limited); err != nil {
		t.Fatalf("decode rate limit payload: %v", err)
	}
	if limited.RetryAfterSeconds <= 0 {
		t.Fatalf("expected retry_after_seconds, got %#v", limited)
	}

	resp, data = doJSONRequestWithHeaders(t, nethttp.MethodPost, "/wedding/access/verify", "", map[string]any{
		"code": "171026",
	}, map[string]string{"X-Forwarded-For": "203.0.113.10"})
	if resp.StatusCode != nethttp.StatusTooManyRequests {
		t.Fatalf("expected locked IP to stay 429 even with correct code, got %d: %s", resp.StatusCode, string(data))
	}

	resp, data = doJSONRequestWithHeaders(t, nethttp.MethodPost, "/wedding/access/verify", "", map[string]any{
		"code": "171026",
	}, map[string]string{"X-Forwarded-For": "203.0.113.11"})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected correct code from another IP to pass, got %d: %s", resp.StatusCode, string(data))
	}
	var verified model.WeddingAccessVerifyResult
	if err := json.Unmarshal(data, &verified); err != nil {
		t.Fatalf("decode verify payload: %v", err)
	}
	if !verified.OK || verified.AccessCodeVersion == "" {
		t.Fatalf("expected successful verify payload, got %#v", verified)
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

func TestWeddingAdminCanUpdateRSVP(t *testing.T) {
	chatHTTPSetup(t)
	if err := store.GetWeddingRepository().ClearAll(); err != nil {
		t.Fatalf("clear wedding data: %v", err)
	}
	user := createTestUser(t, "wedding-update-admin", "wedding-update-admin@example.com")
	token := authToken(t, user.Email, user.Login)

	created, err := store.GetWeddingRepository().CreateRSVP(model.WeddingRSVP{
		FullName:   "До правки",
		Attendance: model.WeddingAttendanceAttending,
		Drinks:     []string{"Белое сухое"},
	})
	if err != nil {
		t.Fatalf("create rsvp: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodPatch, "/wedding/rsvps/"+created.ID, token, map[string]any{
		"full_name":   "  После правки  ",
		"attendance":  model.WeddingAttendanceNotAttending,
		"drinks":      []string{"Водка", "Другое"},
		"other_drink": "Сидр",
		"song":        "Kino - Спокойная ночь",
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected update rsvp 200, got %d: %s", resp.StatusCode, string(data))
	}
	var updated model.WeddingRSVP
	if err := json.Unmarshal(data, &updated); err != nil {
		t.Fatalf("decode updated rsvp: %v", err)
	}
	if updated.ID != created.ID ||
		updated.FullName != "После правки" ||
		updated.Attendance != model.WeddingAttendanceNotAttending ||
		!reflect.DeepEqual(updated.Drinks, []string{"Водка", "Другое"}) ||
		updated.OtherDrink != "Сидр" ||
		updated.Song != "Kino - Спокойная ночь" ||
		!updated.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("unexpected updated rsvp: %#v", updated)
	}

	resp, data = doJSONRequest(t, nethttp.MethodPatch, "/wedding/rsvps/"+created.ID, token, map[string]any{
		"attendance": model.WeddingAttendanceAttending,
	})
	if resp.StatusCode != nethttp.StatusBadRequest {
		t.Fatalf("expected invalid update 400, got %d: %s", resp.StatusCode, string(data))
	}

	resp, data = doJSONRequest(t, nethttp.MethodPatch, "/wedding/rsvps/missing", token, map[string]any{
		"full_name":  "Нет",
		"attendance": model.WeddingAttendanceAttending,
	})
	if resp.StatusCode != nethttp.StatusNotFound {
		t.Fatalf("expected missing update 404, got %d: %s", resp.StatusCode, string(data))
	}
}

func doJSONRequestWithHeaders(t *testing.T, method, path, token string, body any, headers map[string]string) (*nethttp.Response, []byte) {
	t.Helper()

	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
	}

	req, err := nethttp.NewRequest(method, chatHTTPURL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := nethttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return resp, data
}

func TestPostWeddingRSVPCreatesSystemNotificationsForWeddingAdmins(t *testing.T) {
	chatHTTPSetup(t)
	if err := store.GetWeddingRepository().ClearAll(); err != nil {
		t.Fatalf("clear wedding data: %v", err)
	}
	if err := store.GetChatRepository().ClearAll(); err != nil {
		t.Fatalf("clear chat data: %v", err)
	}
	if err := store.GetUserRepository().ClearAll(); err != nil {
		t.Fatalf("clear user data: %v", err)
	}

	weddingAdmin := createTestUser(t, "wedding-admin", "wedding-admin@example.com")
	weddingAdmin.AppPermissions = []string{model.DefaultAppChat, model.DefaultAppWedding}
	if err := store.GetUserRepository().UpdateUser(weddingAdmin, ""); err != nil {
		t.Fatalf("update wedding admin: %v", err)
	}

	regularUser := createTestUser(t, "regular-user", "regular-user@example.com")
	regularUser.AppPermissions = []string{model.DefaultAppChat}
	if err := store.GetUserRepository().UpdateUser(regularUser, ""); err != nil {
		t.Fatalf("update regular user: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodPost, "/wedding/rsvps", "", map[string]any{
		"full_name":  "Анна Иванова",
		"attendance": model.WeddingAttendanceAttending,
		"drinks":     []string{"Белое сухое"},
		"song":       "ABBA - Dancing Queen",
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected create rsvp 200, got %d: %s", resp.StatusCode, string(data))
	}

	conversations, err := store.GetChatRepository().ListUserConversations(weddingAdmin.Email)
	if err != nil {
		t.Fatalf("list wedding admin conversations: %v", err)
	}
	if len(conversations) == 0 {
		t.Fatal("expected wedding admin to have at least one conversation")
	}
	found := false
	for _, convID := range conversations {
		conv, err := store.GetChatRepository().FindConversationByID(convID)
		if err != nil {
			t.Fatalf("get conversation %s: %v", convID, err)
		}
		if conv.LastMessageText == "Анна Иванова - Буду" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected system notification 'Анна Иванова - Буду' for wedding admin")
	}

	userConversations, err := store.GetChatRepository().ListUserConversations(regularUser.Email)
	if err != nil {
		t.Fatalf("list regular user conversations: %v", err)
	}
	for _, convID := range userConversations {
		conv, err := store.GetChatRepository().FindConversationByID(convID)
		if err != nil {
			t.Fatalf("get conversation %s: %v", convID, err)
		}
		if conv.LastMessageText == "Анна Иванова - Буду" {
			t.Fatal("regular user should not get wedding notification")
		}
	}
}

func TestPostWeddingRSVPCreatesNotificationWithNotAttending(t *testing.T) {
	chatHTTPSetup(t)
	if err := store.GetWeddingRepository().ClearAll(); err != nil {
		t.Fatalf("clear wedding data: %v", err)
	}
	if err := store.GetChatRepository().ClearAll(); err != nil {
		t.Fatalf("clear chat data: %v", err)
	}
	if err := store.GetUserRepository().ClearAll(); err != nil {
		t.Fatalf("clear user data: %v", err)
	}

	weddingAdmin := createTestUser(t, "wedding-admin-2", "wedding-admin-2@example.com")
	weddingAdmin.AppPermissions = []string{model.DefaultAppChat, model.DefaultAppWedding}
	if err := store.GetUserRepository().UpdateUser(weddingAdmin, ""); err != nil {
		t.Fatalf("update wedding admin: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodPost, "/wedding/rsvps", "", map[string]any{
		"full_name":  "Петр Петров",
		"attendance": model.WeddingAttendanceNotAttending,
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected create rsvp 200, got %d: %s", resp.StatusCode, string(data))
	}

	conversations, err := store.GetChatRepository().ListUserConversations(weddingAdmin.Email)
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	found := false
	for _, convID := range conversations {
		conv, err := store.GetChatRepository().FindConversationByID(convID)
		if err != nil {
			t.Fatalf("get conversation %s: %v", convID, err)
		}
		if conv.LastMessageText == "Петр Петров - Не Буду" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected system notification 'Петр Петров - Не Буду'")
	}
}

func TestPostWeddingRSVPCreatesNoSystemNotificationsWithoutWeddingAdmins(t *testing.T) {
	chatHTTPSetup(t)
	if err := store.GetWeddingRepository().ClearAll(); err != nil {
		t.Fatalf("clear wedding data: %v", err)
	}
	if err := store.GetChatRepository().ClearAll(); err != nil {
		t.Fatalf("clear chat data: %v", err)
	}
	if err := store.GetUserRepository().ClearAll(); err != nil {
		t.Fatalf("clear user data: %v", err)
	}

	regularUser := createTestUser(t, "regular-only", "regular-only@example.com")
	regularUser.AppPermissions = []string{model.DefaultAppChat}
	if err := store.GetUserRepository().UpdateUser(regularUser, ""); err != nil {
		t.Fatalf("update regular user: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodPost, "/wedding/rsvps", "", map[string]any{
		"full_name":  "Ненужный Гость",
		"attendance": model.WeddingAttendanceAttending,
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected create rsvp 200, got %d: %s", resp.StatusCode, string(data))
	}

	conversations, err := store.GetChatRepository().ListUserConversations(regularUser.Email)
	if err != nil {
		t.Fatalf("list regular user conversations: %v", err)
	}
	for _, convID := range conversations {
		conv, err := store.GetChatRepository().FindConversationByID(convID)
		if err != nil {
			t.Fatalf("get conversation %s: %v", convID, err)
		}
		if conv.LastMessageText != "" {
			t.Fatalf("expected no system notifications, found conversation with text: %s", conv.LastMessageText)
		}
	}
}
