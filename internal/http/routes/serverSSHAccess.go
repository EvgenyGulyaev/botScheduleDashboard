package routes

import (
	"botDashboard/internal/model"
	"botDashboard/internal/system"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-www/silverlining"
)

type sshAccessBody struct {
	Username     string `json:"username"`
	PublicKey    string `json:"public_key"`
	VarGoAccess  bool   `json:"var_go_access"`
	VarWWWAccess bool   `json:"var_www_access"`
}

type sshAccessListDTO struct {
	Items []system.SSHAccess `json:"items"`
}

var sshAccessManager system.SSHAccessManager = system.NewLocalSSHAccessManager(system.DefaultSSHAccessOptions())

func SetSSHAccessManagerForTests(manager system.SSHAccessManager) func() {
	prev := sshAccessManager
	sshAccessManager = manager
	return func() {
		sshAccessManager = prev
	}
}

func GetServerSSHAccesses(ctx *silverlining.Context) {
	reqCtx, cancel := sshAccessContext()
	defer cancel()
	items, err := sshAccessManager.List(reqCtx)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, sshAccessListDTO{Items: items}); err != nil {
		logChatError(err)
	}
}

func PostServerSSHAccess(ctx *silverlining.Context, body []byte) {
	var req sshAccessBody
	if err := json.Unmarshal(body, &req); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}
	actor, err := currentChatUser(ctx)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusUnauthorized})
		return
	}
	reqCtx, cancel := sshAccessContext()
	defer cancel()
	access, err := sshAccessManager.Upsert(reqCtx, system.SSHAccessInput{
		Username:     req.Username,
		PublicKey:    req.PublicKey,
		VarGoAccess:  req.VarGoAccess,
		VarWWWAccess: req.VarWWWAccess,
		Actor:        actor.Email,
	})
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "setfacl is required") || strings.Contains(err.Error(), "create linux user") {
			status = http.StatusInternalServerError
		}
		GetError(ctx, &Error{Message: err.Error(), Status: status})
		return
	}
	recordAdminAudit(ctx, model.AuditActionSSHAccessUpsert, access.Username, "Обновлён SSH доступ "+access.Username, map[string]string{
		"var_go_access":  boolString(access.VarGoAccess),
		"var_www_access": boolString(access.VarWWWAccess),
	})
	if err := ctx.WriteJSON(http.StatusOK, access); err != nil {
		logChatError(err)
	}
}

func DeleteServerSSHAccess(ctx *silverlining.Context, username string) {
	reqCtx, cancel := sshAccessContext()
	defer cancel()
	if err := sshAccessManager.Delete(reqCtx, username); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, system.ErrSSHAccessNotFound()) {
			status = http.StatusNotFound
		}
		GetError(ctx, &Error{Message: err.Error(), Status: status})
		return
	}
	recordAdminAudit(ctx, model.AuditActionSSHAccessDelete, username, "Удалён SSH доступ "+username, nil)
	if err := ctx.WriteJSON(http.StatusOK, map[string]bool{"ok": true}); err != nil {
		logChatError(err)
	}
}

func sshAccessContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
