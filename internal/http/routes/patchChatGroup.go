package routes

import (
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"

	"github.com/go-www/silverlining"
)

func PatchChatGroup(ctx *silverlining.Context, conversationID string, body []byte) {
	_, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	var req chatRenameBody
	if err := json.Unmarshal(body, &req); err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if req.Title == "" {
		writeChatError(ctx, http.StatusBadRequest, "title is required")
		return
	}

	if _, err := store.GetChatRepository().RenameGroupConversation(conversationID, req.Title); err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, map[string]string{"message": "ok"}); err != nil {
		logChatError(err)
	}
}
