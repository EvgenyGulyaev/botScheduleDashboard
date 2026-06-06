package routes_test

import (
	"botDashboard/internal/http/routes"
	"botDashboard/internal/store"
	"botDashboard/internal/system"
	"context"
	"encoding/json"
	nethttp "net/http"
	"testing"
	"time"
)

type fakeSSHAccessManager struct {
	items           []system.SSHAccess
	upserted        system.SSHAccessInput
	deletedUsername string
}

func (m *fakeSSHAccessManager) List(_ context.Context) ([]system.SSHAccess, error) {
	return m.items, nil
}

func (m *fakeSSHAccessManager) Upsert(_ context.Context, input system.SSHAccessInput) (system.SSHAccess, error) {
	m.upserted = input
	access := system.SSHAccess{
		Username:         input.Username,
		KeyPreview:       "ssh-ed25519 AAAA...",
		VarGoAccess:      input.VarGoAccess,
		VarWWWAccess:     input.VarWWWAccess,
		ConnectionString: "ssh " + input.Username + "@95.181.224.178",
		CreatedBy:        input.Actor,
		UpdatedBy:        input.Actor,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
	m.items = []system.SSHAccess{access}
	return access, nil
}

func (m *fakeSSHAccessManager) Delete(_ context.Context, username string) error {
	m.deletedUsername = username
	m.items = nil
	return nil
}

func TestServerSSHAccessRequiresSuperAdmin(t *testing.T) {
	chatHTTPSetup(t)
	restore := routes.SetSSHAccessManagerForTests(&fakeSSHAccessManager{})
	defer restore()

	createTestUser(t, "alice", "alice@example.com")
	resp, data := doJSONRequest(t, nethttp.MethodGet, "/server/ssh-accesses", authToken(t, "alice@example.com", "alice"), nil)
	if resp.StatusCode != nethttp.StatusUnauthorized && resp.StatusCode != nethttp.StatusForbidden {
		t.Fatalf("expected non-super-admin to be rejected, got %d: %s", resp.StatusCode, string(data))
	}
}

func TestServerSSHAccessCRUD(t *testing.T) {
	chatHTTPSetup(t)
	fake := &fakeSSHAccessManager{}
	restore := routes.SetSSHAccessManagerForTests(fake)
	defer restore()

	super := createTestUser(t, "root-admin", "root-admin@example.com")
	super.IsAdmin = true
	super.IsSuperAdmin = true
	if err := store.GetUserRepository().UpdateUser(super, super.Email); err != nil {
		t.Fatalf("update super user: %v", err)
	}
	token := authToken(t, super.Email, super.Login)

	resp, data := doJSONRequest(t, nethttp.MethodPost, "/server/ssh-accesses", token, map[string]any{
		"username":       "deploy",
		"public_key":     "ssh-ed25519 AAAA root@example.com",
		"var_go_access":  true,
		"var_www_access": false,
	})
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 create, got %d: %s", resp.StatusCode, string(data))
	}
	if fake.upserted.Username != "deploy" || !fake.upserted.VarGoAccess || fake.upserted.VarWWWAccess {
		t.Fatalf("unexpected upsert input: %#v", fake.upserted)
	}

	resp, data = doJSONRequest(t, nethttp.MethodGet, "/server/ssh-accesses", token, nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 list, got %d: %s", resp.StatusCode, string(data))
	}
	var list struct {
		Items []system.SSHAccess `json:"items"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Username != "deploy" {
		t.Fatalf("unexpected list response: %#v", list)
	}

	resp, data = doJSONRequest(t, nethttp.MethodDelete, "/server/ssh-accesses/deploy", token, nil)
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("expected 200 delete, got %d: %s", resp.StatusCode, string(data))
	}
	if fake.deletedUsername != "deploy" {
		t.Fatalf("expected delete deploy, got %q", fake.deletedUsername)
	}
}
