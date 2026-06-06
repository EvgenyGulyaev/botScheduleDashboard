package model

import "time"

const (
	AuditActionAdminUserCreate   = "admin.user.create"
	AuditActionAdminUserUpdate   = "admin.user.update"
	AuditActionAdminUserDelete   = "admin.user.delete"
	AuditActionServiceRestart    = "service.restart"
	AuditActionServerMaintenance = "server.maintenance"
	AuditActionSSHAccessUpsert   = "server.ssh_access.upsert"
	AuditActionSSHAccessDelete   = "server.ssh_access.delete"
	AuditMaxRecentEntries        = 20
	AuditRetention               = 3 * 24 * time.Hour
)

type AuditEntry struct {
	ID         string            `json:"id"`
	ActorEmail string            `json:"actor_email"`
	ActorLogin string            `json:"actor_login"`
	Action     string            `json:"action"`
	Target     string            `json:"target"`
	Summary    string            `json:"summary"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}
