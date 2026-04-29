package routes

import (
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"log"
	"net/http"

	"github.com/go-www/silverlining"
)

type adminAuditDTO struct {
	Items []model.AuditEntry `json:"items"`
}

func GetAdminAudit(ctx *silverlining.Context) {
	items, err := store.GetAuditRepository().ListRecent()
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, adminAuditDTO{Items: items}); err != nil {
		logChatError(err)
	}
}

func recordAdminAudit(ctx *silverlining.Context, action, target, summary string, metadata map[string]string) {
	actor, err := currentChatUser(ctx)
	if err != nil {
		log.Printf("failed to resolve audit actor: %v", err)
		return
	}
	_, err = store.GetAuditRepository().Append(model.AuditEntry{
		ActorEmail: actor.Email,
		ActorLogin: actor.Login,
		Action:     action,
		Target:     target,
		Summary:    summary,
		Metadata:   metadata,
	})
	if err != nil {
		log.Printf("failed to write audit entry: %v", err)
	}
}
