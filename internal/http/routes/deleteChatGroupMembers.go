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

	if _, err := store.GetChatRepository().RemoveGroupMembers(conversationID, emails); err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, map[string]string{"message": "ok"}); err != nil {
		logChatError(err)
	}

	if selfRemoved {
		return
	}
}
