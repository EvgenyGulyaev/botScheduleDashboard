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

	_, members, err := ensureGroupMember(user, conversationID)
	if err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}
	member, _ := findMember(members, user.Email)
	if err := forbiddenUnless(canDeleteGroup(member), "user cannot delete group"); err != nil {
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
