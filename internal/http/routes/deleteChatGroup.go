package routes

import (
	"botDashboard/internal/store"
	"net/http"

	"github.com/go-www/silverlining"
)

func DeleteChatGroup(ctx *silverlining.Context, conversationID string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	if _, _, err := ensureGroupMember(user, conversationID); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	if err := store.GetChatRepository().DeleteGroupConversation(conversationID); err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, map[string]string{"message": "ok"}); err != nil {
		logChatError(err)
	}
}
