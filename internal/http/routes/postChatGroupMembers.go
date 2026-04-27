package routes

import (
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"

	"github.com/go-www/silverlining"
)

func PostChatGroupMembers(ctx *silverlining.Context, conversationID string, body []byte) {
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

	_, existingMembers, err := ensureGroupMember(user, conversationID)
	if err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}
	currentMember, _ := findMember(existingMembers, user.Email)
	if err := forbiddenUnless(canAddGroupMembers(currentMember), "user cannot add group members"); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	members := make([]model.ChatMember, 0, len(req.Emails))
	for _, email := range uniqueEmails(req.Emails) {
		member, err := store.GetUserRepository().FindUserByEmail(email)
		if err != nil {
			writeChatError(ctx, http.StatusBadRequest, err.Error())
			return
		}
		members = append(members, model.ChatMember{Email: member.Email, Login: member.Login})
	}

	if _, err := store.GetChatRepository().AddGroupMembers(conversationID, members); err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
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
