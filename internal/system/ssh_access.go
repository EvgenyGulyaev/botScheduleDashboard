package system

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	defaultSSHAccessStateDir = "/var/lib/bot-dashboard/ssh-accesses"
	defaultSSHHomeRoot       = "/home"
	sshAccessMarker          = "bot-dashboard:ssh-access"
)

var (
	errSSHAccessNotFound = errors.New("ssh access not found")
	linuxUsernameRe      = regexp.MustCompile(`^[a-z_][a-z0-9_-]{1,31}$`)
	sshKeyTypes          = map[string]struct{}{
		"ssh-ed25519":                        {},
		"ssh-rsa":                            {},
		"ecdsa-sha2-nistp256":                {},
		"ecdsa-sha2-nistp384":                {},
		"ecdsa-sha2-nistp521":                {},
		"sk-ssh-ed25519@openssh.com":         {},
		"sk-ecdsa-sha2-nistp256@openssh.com": {},
	}
	reservedSSHUsers = map[string]struct{}{
		"root":     {},
		"daemon":   {},
		"bin":      {},
		"sys":      {},
		"sync":     {},
		"games":    {},
		"man":      {},
		"lp":       {},
		"mail":     {},
		"news":     {},
		"uucp":     {},
		"proxy":    {},
		"www-data": {},
		"backup":   {},
		"list":     {},
		"irc":      {},
		"gnats":    {},
		"nobody":   {},
		"nats":     {},
	}
)

func ErrSSHAccessNotFound() error {
	return errSSHAccessNotFound
}

type SSHAccess struct {
	Username         string    `json:"username"`
	KeyPreview       string    `json:"key_preview"`
	VarGoAccess      bool      `json:"var_go_access"`
	VarWWWAccess     bool      `json:"var_www_access"`
	ConnectionString string    `json:"connection_string"`
	CreatedBy        string    `json:"created_by"`
	UpdatedBy        string    `json:"updated_by"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type SSHAccessInput struct {
	Username     string
	PublicKey    string
	VarGoAccess  bool
	VarWWWAccess bool
	Actor        string
}

type SSHAccessManager interface {
	List(ctx context.Context) ([]SSHAccess, error)
	Upsert(ctx context.Context, input SSHAccessInput) (SSHAccess, error)
	Delete(ctx context.Context, username string) error
}

type SSHAccessOptions struct {
	StateDir string
	HomeRoot string
	Host     string
	Runner   func(ctx context.Context, name string, args ...string) ([]byte, error)
}

type LocalSSHAccessManager struct {
	stateDir string
	homeRoot string
	host     string
	runner   func(ctx context.Context, name string, args ...string) ([]byte, error)
}

func DefaultSSHAccessOptions() SSHAccessOptions {
	return SSHAccessOptions{
		StateDir: defaultSSHAccessStateDir,
		HomeRoot: defaultSSHHomeRoot,
		Host:     "95.181.224.178",
	}
}

func NewLocalSSHAccessManager(options SSHAccessOptions) *LocalSSHAccessManager {
	if strings.TrimSpace(options.StateDir) == "" {
		options.StateDir = defaultSSHAccessStateDir
	}
	if strings.TrimSpace(options.HomeRoot) == "" {
		options.HomeRoot = defaultSSHHomeRoot
	}
	if strings.TrimSpace(options.Host) == "" {
		options.Host = "95.181.224.178"
	}
	if options.Runner == nil {
		options.Runner = runCommand
	}
	return &LocalSSHAccessManager{
		stateDir: options.StateDir,
		homeRoot: options.HomeRoot,
		host:     options.Host,
		runner:   options.Runner,
	}
}

func (m *LocalSSHAccessManager) List(_ context.Context) ([]SSHAccess, error) {
	if err := os.MkdirAll(m.stateDir, 0o700); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(m.stateDir)
	if err != nil {
		return nil, err
	}
	result := make([]SSHAccess, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		access, err := m.readState(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		result = append(result, access)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Username < result[j].Username
	})
	return result, nil
}

func (m *LocalSSHAccessManager) Upsert(ctx context.Context, input SSHAccessInput) (SSHAccess, error) {
	username, err := normalizeSSHUsername(input.Username)
	if err != nil {
		return SSHAccess{}, err
	}
	if err := os.MkdirAll(m.stateDir, 0o700); err != nil {
		return SSHAccess{}, err
	}
	existing, stateErr := m.readState(username)
	if stateErr != nil && !errors.Is(stateErr, errSSHAccessNotFound) {
		return SSHAccess{}, stateErr
	}
	keyLine := ""
	keyPreview := ""
	if strings.TrimSpace(input.PublicKey) == "" {
		if errors.Is(stateErr, errSSHAccessNotFound) {
			return SSHAccess{}, errors.New("public key is required")
		}
		keyLine, err = m.readAuthorizedKey(username)
		if err != nil {
			return SSHAccess{}, err
		}
		keyPreview = existing.KeyPreview
	} else {
		keyLine, keyPreview, err = normalizePublicKey(input.PublicKey)
		if err != nil {
			return SSHAccess{}, err
		}
	}
	userExists := m.userExists(ctx, username)
	if userExists && errors.Is(stateErr, errSSHAccessNotFound) {
		return SSHAccess{}, fmt.Errorf("linux user %q already exists outside ssh access manager", username)
	}
	if !userExists {
		if _, err := m.run(ctx, "useradd", "--create-home", "--shell", "/bin/bash", "--comment", sshAccessMarker, username); err != nil {
			return SSHAccess{}, fmt.Errorf("create linux user: %w", err)
		}
		_, _ = m.run(ctx, "passwd", "-l", username)
	}
	if err := m.writeAuthorizedKey(ctx, username, keyLine); err != nil {
		return SSHAccess{}, err
	}
	if err := m.applyPathACLs(ctx, username, input.VarGoAccess, input.VarWWWAccess); err != nil {
		return SSHAccess{}, err
	}
	now := time.Now().UTC()
	if existing.CreatedAt.IsZero() {
		existing.CreatedAt = now
		existing.CreatedBy = strings.TrimSpace(input.Actor)
	}
	existing.Username = username
	existing.KeyPreview = keyPreview
	existing.VarGoAccess = input.VarGoAccess
	existing.VarWWWAccess = input.VarWWWAccess
	existing.ConnectionString = fmt.Sprintf("ssh %s@%s", username, m.host)
	existing.UpdatedAt = now
	existing.UpdatedBy = strings.TrimSpace(input.Actor)
	if err := m.writeState(existing); err != nil {
		return SSHAccess{}, err
	}
	return existing, nil
}

func (m *LocalSSHAccessManager) Delete(ctx context.Context, username string) error {
	normalized, err := normalizeSSHUsername(username)
	if err != nil {
		return err
	}
	if _, err := m.readState(normalized); err != nil {
		return err
	}
	m.removePathACLs(ctx, normalized)
	if m.userExists(ctx, normalized) {
		if _, err := m.run(ctx, "userdel", "-r", normalized); err != nil {
			return fmt.Errorf("delete linux user: %w", err)
		}
	}
	return os.Remove(m.statePath(normalized))
}

func (m *LocalSSHAccessManager) writeAuthorizedKey(ctx context.Context, username, keyLine string) error {
	sshDir := filepath.Join(m.homeRoot, username, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return err
	}
	authKeys := filepath.Join(sshDir, "authorized_keys")
	if err := os.WriteFile(authKeys, []byte(keyLine+"\n"), 0o600); err != nil {
		return err
	}
	if _, err := m.run(ctx, "chown", "-R", username+":"+username, sshDir); err != nil {
		return fmt.Errorf("chown authorized_keys: %w", err)
	}
	return nil
}

func (m *LocalSSHAccessManager) readAuthorizedKey(username string) (string, error) {
	data, err := os.ReadFile(filepath.Join(m.homeRoot, username, ".ssh", "authorized_keys"))
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	if line == "" {
		return "", errors.New("existing authorized_keys is empty")
	}
	return line, nil
}

func (m *LocalSSHAccessManager) applyPathACLs(ctx context.Context, username string, varGo, varWWW bool) error {
	if _, err := m.run(ctx, "setfacl", "--version"); err != nil {
		return fmt.Errorf("setfacl is required for folder permissions: %w", err)
	}
	if err := m.applyPathACL(ctx, username, "/var/go", varGo); err != nil {
		return err
	}
	return m.applyPathACL(ctx, username, "/var/www", varWWW)
}

func (m *LocalSSHAccessManager) applyPathACL(ctx context.Context, username, path string, allowed bool) error {
	if allowed {
		if _, err := m.run(ctx, "setfacl", "-R", "-m", "u:"+username+":rx", path); err != nil {
			return fmt.Errorf("grant %s access: %w", path, err)
		}
		return nil
	}
	if _, err := m.run(ctx, "setfacl", "-m", "u:"+username+":---", path); err != nil {
		return fmt.Errorf("deny %s access: %w", path, err)
	}
	return nil
}

func (m *LocalSSHAccessManager) removePathACLs(ctx context.Context, username string) {
	for _, path := range []string{"/var/go", "/var/www"} {
		_, _ = m.run(ctx, "setfacl", "-R", "-x", "u:"+username, path)
	}
}

func (m *LocalSSHAccessManager) userExists(ctx context.Context, username string) bool {
	_, err := m.run(ctx, "id", "-u", username)
	return err == nil
}

func (m *LocalSSHAccessManager) readState(username string) (SSHAccess, error) {
	data, err := os.ReadFile(m.statePath(username))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SSHAccess{}, errSSHAccessNotFound
		}
		return SSHAccess{}, err
	}
	var access SSHAccess
	if err := json.Unmarshal(data, &access); err != nil {
		return SSHAccess{}, err
	}
	return access, nil
}

func (m *LocalSSHAccessManager) writeState(access SSHAccess) error {
	data, err := json.MarshalIndent(access, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.statePath(access.Username), append(data, '\n'), 0o600)
}

func (m *LocalSSHAccessManager) statePath(username string) string {
	return filepath.Join(m.stateDir, username+".json")
}

func (m *LocalSSHAccessManager) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return m.runner(ctx, name, args...)
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func normalizeSSHUsername(value string) (string, error) {
	username := strings.ToLower(strings.TrimSpace(value))
	if !linuxUsernameRe.MatchString(username) {
		return "", errors.New("username must start with a letter or underscore and contain only lowercase letters, digits, underscore or dash")
	}
	if _, reserved := reservedSSHUsers[username]; reserved {
		return "", fmt.Errorf("username %q is reserved", username)
	}
	return username, nil
}

func normalizePublicKey(value string) (string, string, error) {
	line := strings.TrimSpace(value)
	if line == "" {
		return "", "", errors.New("public key is required")
	}
	if strings.ContainsAny(line, "\r\n") {
		return "", "", errors.New("public key must be a single line")
	}
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return "", "", errors.New("public key format is invalid")
	}
	if _, ok := sshKeyTypes[parts[0]]; !ok {
		return "", "", errors.New("only OpenSSH public keys are allowed")
	}
	if _, err := base64.StdEncoding.DecodeString(parts[1]); err != nil {
		return "", "", errors.New("public key payload is invalid")
	}
	previewKey := parts[1]
	if len(previewKey) > 16 {
		previewKey = previewKey[:16]
	}
	preview := parts[0] + " " + previewKey + "..."
	if len(parts) > 2 {
		preview += " " + strings.Join(parts[2:], " ")
	}
	return line, preview, nil
}
