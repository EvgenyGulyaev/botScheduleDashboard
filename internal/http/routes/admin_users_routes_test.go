package routes_test

import (
	"botDashboard/internal/command"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"encoding/json"
	nethttp "net/http"
	"testing"
)

type adminUsersResponse struct {
	Items []adminUserResponse `json:"items"`
}

type adminUserResponse struct {
	Login            string   `json:"login"`
	Email            string   `json:"email"`
	IsAdmin          bool     `json:"is_admin"`
	IsSuperAdmin     bool     `json:"is_super_admin"`
	DefaultApp       string   `json:"default_app"`
	AppPermissions   []string `json:"app_permissions"`
	VisibilityGroups []string `json:"visibility_groups"`
}

func TestAdminUsersRequiresSuperAdmin(t *testing.T) {
	chatHTTPSetup(t)
	admin := createTestUser(t, "admin", "admin@example.com")
	admin.IsAdmin = true
	if err := store.GetUserRepository().UpdateUser(admin, ""); err != nil {
		t.Fatalf("promote admin: %v", err)
	}

	resp, _ := doJSONRequest(t, nethttp.MethodGet, "/admin/users", authToken(t, admin.Email, admin.Login), nil)
	if resp.StatusCode != nethttp.StatusUnauthorized {
		t.Fatalf("expected 401 for regular admin, got %d", resp.StatusCode)
	}

	admin.IsSuperAdmin = true
	if err := store.GetUserRepository().UpdateUser(admin, ""); err != nil {
		t.Fatalf("promote super admin: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodGet, "/admin/users", authToken(t, admin.Email, admin.Login), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 for super admin, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestAdminUsersCRUDUpdatesRolesPermissionsAndDefaultApp(t *testing.T) {
	chatHTTPSetup(t)
	super := createTestUser(t, "evgeny", "evgeny@example.com")
	super.IsAdmin = true
	super.IsSuperAdmin = true
	if err := store.GetUserRepository().UpdateUser(super, ""); err != nil {
		t.Fatalf("promote super admin: %v", err)
	}

	token := authToken(t, super.Email, super.Login)
	resp, data := doJSONRequest(t, nethttp.MethodPost, "/admin/users", token, map[string]any{
		"login":             "nina",
		"email":             "nina@example.com",
		"password":          "secret-password",
		"is_admin":          true,
		"default_app":       model.DefaultAppGeo3D,
		"app_permissions":   []string{model.DefaultAppChat, model.DefaultAppGeo3D, model.DefaultAppAlice},
		"visibility_groups": []string{"general", "family"},
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on create, got %d: %s", resp.StatusCode, string(data))
	}

	user, err := store.GetUserRepository().FindUserByEmail("nina@example.com")
	if err != nil {
		t.Fatalf("find created user: %v", err)
	}
	if !user.IsAdmin || user.IsSuperAdmin {
		t.Fatalf("unexpected created roles: %#v", user)
	}
	if user.DefaultApp != model.DefaultAppGeo3D {
		t.Fatalf("expected geo3d default app, got %#v", user.DefaultApp)
	}
	if len(user.AppPermissions) != 3 || user.AppPermissions[0] != model.DefaultAppChat || user.AppPermissions[1] != model.DefaultAppGeo3D || user.AppPermissions[2] != model.DefaultAppAlice {
		t.Fatalf("unexpected app permissions: %#v", user.AppPermissions)
	}
	if len(user.VisibilityGroups) != 2 || user.VisibilityGroups[0] != model.DefaultVisibilityGroup || user.VisibilityGroups[1] != "family" {
		t.Fatalf("unexpected visibility groups: %#v", user.VisibilityGroups)
	}

	resp, data = doJSONRequest(t, nethttp.MethodPatch, "/admin/users/nina%40example.com", token, map[string]any{
		"login":             "nina-new",
		"email":             "nina-new@example.com",
		"is_super_admin":    true,
		"default_app":       model.DefaultAppChat,
		"app_permissions":   []string{model.DefaultAppChat},
		"visibility_groups": []string{"team", "team"},
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on update, got %d: %s", resp.StatusCode, string(data))
	}

	user, err = store.GetUserRepository().FindUserByEmail("nina-new@example.com")
	if err != nil {
		t.Fatalf("find updated user: %v", err)
	}
	if user.Login != "nina-new" || !user.IsSuperAdmin || !user.IsAdmin {
		t.Fatalf("unexpected updated user: %#v", user)
	}
	if len(user.AppPermissions) != 1 || user.AppPermissions[0] != model.DefaultAppChat {
		t.Fatalf("unexpected updated app permissions: %#v", user.AppPermissions)
	}
	if len(user.VisibilityGroups) != 1 || user.VisibilityGroups[0] != "team" {
		t.Fatalf("unexpected updated visibility groups: %#v", user.VisibilityGroups)
	}

	resp, data = doJSONRequest(t, nethttp.MethodDelete, "/admin/users/nina-new%40example.com", token, nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on delete, got %d: %s", resp.StatusCode, string(data))
	}
	if _, err := store.GetUserRepository().FindUserByEmail("nina-new@example.com"); err == nil {
		t.Fatalf("expected user to be deleted")
	}
}

func TestProfileIncludesSuperAdminAndAppPermissions(t *testing.T) {
	chatHTTPSetup(t)
	user := createTestUser(t, "alice", "alice@example.com")
	user.IsAdmin = true
	user.IsSuperAdmin = true
	user.AppPermissions = []string{model.DefaultAppChat, model.DefaultAppShortLinks}
	if err := store.GetUserRepository().UpdateUser(user, ""); err != nil {
		t.Fatalf("update user: %v", err)
	}

	resp, data := doJSONRequest(t, nethttp.MethodGet, "/profile", authToken(t, user.Email, user.Login), nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var profile adminUserResponse
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("unmarshal profile: %v", err)
	}
	if !profile.IsSuperAdmin {
		t.Fatalf("expected is_super_admin in profile")
	}
	if len(profile.AppPermissions) != 2 || profile.AppPermissions[1] != model.DefaultAppShortLinks {
		t.Fatalf("unexpected profile app permissions: %#v", profile.AppPermissions)
	}
}

func TestUserSetSuperAdminCommandAcceptsLogin(t *testing.T) {
	chatHTTPSetup(t)
	createTestUser(t, "evgeny", "evgeny@example.com")

	result := (&command.UserSetSuperAdmin{Identity: "evgeny"}).Execute()
	if result != "Success" {
		t.Fatalf("expected success, got %q", result)
	}

	user, err := store.GetUserRepository().FindUserByEmail("evgeny@example.com")
	if err != nil {
		t.Fatalf("find user: %v", err)
	}
	if !user.IsAdmin || !user.IsSuperAdmin {
		t.Fatalf("expected evgeny to be super admin, got %#v", user)
	}
}

func TestAdminAuditLogsUserChangesAndKeepsLastTwenty(t *testing.T) {
	chatHTTPSetup(t)
	super := createTestUser(t, "evgeny", "evgeny@example.com")
	super.IsAdmin = true
	super.IsSuperAdmin = true
	if err := store.GetUserRepository().UpdateUser(super, ""); err != nil {
		t.Fatalf("promote super admin: %v", err)
	}

	token := authToken(t, super.Email, super.Login)
	for i := 0; i < 19; i++ {
		resp, data := doJSONRequest(t, nethttp.MethodPost, "/admin/users", token, map[string]any{
			"login":       "user-log",
			"email":       "user-log-" + string(rune('a'+i)) + "@example.com",
			"password":    "secret-password",
			"default_app": model.DefaultAppChat,
		})
		if resp.StatusCode != nethttp.StatusOK {
			t.Fatalf("expected create to be logged, got %d: %s", resp.StatusCode, string(data))
		}
	}
	resp, data := doJSONRequest(t, nethttp.MethodPost, "/admin/users", token, map[string]any{
		"login":       "user-log",
		"email":       "user-log@example.com",
		"password":    "secret-password",
		"default_app": model.DefaultAppChat,
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected target create to be logged, got %d: %s", resp.StatusCode, string(data))
	}
	_, _ = doJSONRequest(t, nethttp.MethodDelete, "/admin/users/user-log%40example.com", token, nil)

	resp, data = doJSONRequest(t, nethttp.MethodGet, "/admin/audit", token, nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(data))
	}

	var payload struct {
		Items []struct {
			ActorEmail string `json:"actor_email"`
			Action     string `json:"action"`
			Target     string `json:"target"`
			Summary    string `json:"summary"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode audit response: %v", err)
	}
	if len(payload.Items) != 20 {
		t.Fatalf("expected audit response to keep 20 entries, got %d", len(payload.Items))
	}
	if payload.Items[0].ActorEmail != "evgeny@example.com" {
		t.Fatalf("expected actor in audit entry, got %#v", payload.Items[0])
	}
	foundDelete := false
	for _, item := range payload.Items {
		if item.Action == "admin.user.delete" && item.Target == "user-log@example.com" {
			foundDelete = true
		}
	}
	if !foundDelete {
		t.Fatalf("expected delete operation in audit log, got %#v", payload.Items)
	}
}

func TestAdminUsersSupportsDrawingPermission(t *testing.T) {
	chatHTTPSetup(t)
	super := createTestUser(t, "evgeny", "evgeny@example.com")
	super.IsAdmin = true
	super.IsSuperAdmin = true
	if err := store.GetUserRepository().UpdateUser(super, ""); err != nil {
		t.Fatalf("promote super admin: %v", err)
	}

	token := authToken(t, super.Email, super.Login)
	resp, data := doJSONRequest(t, nethttp.MethodPost, "/admin/users", token, map[string]any{
		"login":           "drawer",
		"email":           "drawer@example.com",
		"password":        "secret-password",
		"default_app":     model.DefaultAppDrawing,
		"app_permissions": []string{model.DefaultAppChat, model.DefaultAppDrawing},
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 on create, got %d: %s", resp.StatusCode, string(data))
	}

	user, err := store.GetUserRepository().FindUserByEmail("drawer@example.com")
	if err != nil {
		t.Fatalf("find created user: %v", err)
	}
	if user.DefaultApp != model.DefaultAppDrawing {
		t.Fatalf("expected drawing default app, got %#v", user.DefaultApp)
	}
	if len(user.AppPermissions) != 2 || user.AppPermissions[0] != model.DefaultAppChat || user.AppPermissions[1] != model.DefaultAppDrawing {
		t.Fatalf("unexpected app permissions: %#v", user.AppPermissions)
	}
}
