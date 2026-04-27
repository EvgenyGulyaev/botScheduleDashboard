package routes

import (
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"

	"github.com/go-www/silverlining"
)

func PatchChatGroupMember(ctx *silverlining.Context, conversationID, email string, body []byte) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	var req chatMemberRoleBody
	if err := json.Unmarshal(body, &req); err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if req.Role != model.ChatMemberRoleAdmin && req.Role != model.ChatMemberRoleMember {
		writeChatError(ctx, http.StatusBadRequest, "role must be admin or member")
		return
	}

	_, members, err := ensureGroupMember(user, conversationID)
	if err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}
	currentMember, _ := findMember(members, user.Email)
	if err := forbiddenUnless(canManageGroupRoles(currentMember), "user cannot manage group roles"); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}
	target, ok := findMember(members, email)
	if !ok {
		writeChatError(ctx, http.StatusBadRequest, "member not found")
		return
	}
	if chatMemberRole(target) == model.ChatMemberRoleOwner {
		writeChatError(ctx, http.StatusForbidden, "owner role cannot be changed")
		return
	}

	if _, err := store.GetChatRepository().SetGroupMemberRole(conversationID, email, req.Role); err != nil {
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
