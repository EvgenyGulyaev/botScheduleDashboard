package routes

import (
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"

	"github.com/go-www/silverlining"
)

func DeleteChatGroupMembers(ctx *silverlining.Context, conversationID string, body []byte) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	var req chatMemberBody
	if err := json.Unmarshal(body, &req); err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	emails := uniqueEmails(req.Emails)
	selfRemoved := containsEmail(emails, user.Email)

	_, members, err := ensureGroupMember(user, conversationID)
	if err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}
	currentMember, _ := findMember(members, user.Email)
	for _, email := range emails {
		target, ok := findMember(members, email)
		if !ok {
			continue
		}
		if !canRemoveTarget(currentMember, target) {
			writeChatError(ctx, http.StatusForbidden, "user cannot remove group member")
			return
		}
	}

	if _, err := store.GetChatRepository().RemoveGroupMembers(conversationID, emails); err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	if selfRemoved {
		if err := ctx.WriteJSON(http.StatusOK, map[string]string{"message": "ok"}); err != nil {
			logChatError(err)
		}
		return
	}

	view, err := conversationView(ctx, conversationID, user.Email)
	if err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, view); err != nil {
		logChatError(err)
	}
}
