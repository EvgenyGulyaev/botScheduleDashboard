package routes

import (
	"botDashboard/internal/store"
	"net/http"

	"github.com/go-www/silverlining"
)

func DeleteChatDraft(ctx *silverlining.Context, conversationID string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}
	if _, err := conversationView(ctx, conversationID, user.Email); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	if err := store.GetChatRepository().ClearChatDraft(conversationID, user.Email); err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, chatDraftDTO{}); err != nil {
		logChatError(err)
	}
}
