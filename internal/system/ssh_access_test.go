package system

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type sshAccessRunner struct {
	users    map[string]bool
	commands []string
}

func newSSHAccessRunner() *sshAccessRunner {
	return &sshAccessRunner{users: map[string]bool{}}
}

func (r *sshAccessRunner) run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, name+" "+strings.Join(args, " "))
	if name == "id" && len(args) == 2 && args[0] == "-u" {
		if r.users[args[1]] {
			return []byte("1001\n"), nil
		}
		return nil, errors.New("not found")
	}
	if name == "useradd" && len(args) > 0 {
		r.users[args[len(args)-1]] = true
	}
	if name == "userdel" && len(args) > 0 {
		delete(r.users, args[len(args)-1])
	}
	return nil, nil
}

func testPublicKey() string {
	return "ssh-ed25519 " + base64.StdEncoding.EncodeToString([]byte("demo-public-key")) + " evgeny@example.com"
}

func TestLocalSSHAccessManagerUpsertCreatesUserKeyAndACL(t *testing.T) {
	runner := newSSHAccessRunner()
	dir := t.TempDir()
	manager := NewLocalSSHAccessManager(SSHAccessOptions{
		StateDir: filepath.Join(dir, "state"),
		HomeRoot: filepath.Join(dir, "home"),
		Host:     "example.com",
		Runner:   runner.run,
	})

	access, err := manager.Upsert(context.Background(), SSHAccessInput{
		Username:     " Deploy_User ",
		PublicKey:    testPublicKey(),
		VarGoAccess:  true,
		VarWWWAccess: false,
		Actor:        "root@example.com",
	})
	if err != nil {
		t.Fatalf("upsert ssh access: %v", err)
	}
	if access.Username != "deploy_user" || access.ConnectionString != "ssh deploy_user@example.com" {
		t.Fatalf("unexpected access response: %#v", access)
	}
	authKeys, err := os.ReadFile(filepath.Join(dir, "home", "deploy_user", ".ssh", "authorized_keys"))
	if err != nil {
		t.Fatalf("read authorized_keys: %v", err)
	}
	if strings.TrimSpace(string(authKeys)) != testPublicKey() {
		t.Fatalf("unexpected authorized_keys: %q", string(authKeys))
	}
	commands := strings.Join(runner.commands, "\n")
	if !strings.Contains(commands, "useradd --create-home --shell /bin/bash --comment bot-dashboard:ssh-access deploy_user") {
		t.Fatalf("expected useradd command, got:\n%s", commands)
	}
	if !strings.Contains(commands, "setfacl -R -m u:deploy_user:rx /var/go") {
		t.Fatalf("expected /var/go acl grant, got:\n%s", commands)
	}
	if !strings.Contains(commands, "setfacl -m u:deploy_user:--- /var/www") {
		t.Fatalf("expected /var/www acl deny, got:\n%s", commands)
	}
}

func TestLocalSSHAccessManagerRejectsUnsafeInput(t *testing.T) {
	manager := NewLocalSSHAccessManager(SSHAccessOptions{
		StateDir: t.TempDir(),
		HomeRoot: t.TempDir(),
		Runner:   newSSHAccessRunner().run,
	})
	if _, err := manager.Upsert(context.Background(), SSHAccessInput{Username: "root", PublicKey: testPublicKey()}); err == nil {
		t.Fatalf("expected root username to be rejected")
	}
	if _, err := manager.Upsert(context.Background(), SSHAccessInput{Username: "deploy", PublicKey: "command=bad ssh-ed25519 AAAA"}); err == nil {
		t.Fatalf("expected key options to be rejected")
	}
}

func TestLocalSSHAccessManagerUpdatesFoldersWithoutReplacingKey(t *testing.T) {
	runner := newSSHAccessRunner()
	dir := t.TempDir()
	manager := NewLocalSSHAccessManager(SSHAccessOptions{
		StateDir: filepath.Join(dir, "state"),
		HomeRoot: filepath.Join(dir, "home"),
		Runner:   runner.run,
	})
	if _, err := manager.Upsert(context.Background(), SSHAccessInput{
		Username:  "deploy",
		PublicKey: testPublicKey(),
		Actor:     "root@example.com",
	}); err != nil {
		t.Fatalf("create ssh access: %v", err)
	}

	updated, err := manager.Upsert(context.Background(), SSHAccessInput{
		Username:     "deploy",
		VarGoAccess:  true,
		VarWWWAccess: true,
		Actor:        "root@example.com",
	})
	if err != nil {
		t.Fatalf("update ssh access without key: %v", err)
	}
	if !updated.VarGoAccess || !updated.VarWWWAccess || updated.KeyPreview == "" {
		t.Fatalf("unexpected updated access: %#v", updated)
	}
	authKeys, err := os.ReadFile(filepath.Join(dir, "home", "deploy", ".ssh", "authorized_keys"))
	if err != nil {
		t.Fatalf("read authorized_keys: %v", err)
	}
	if strings.TrimSpace(string(authKeys)) != testPublicKey() {
		t.Fatalf("authorized key should be preserved, got %q", string(authKeys))
	}
}
