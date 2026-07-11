package routes

import (
	"botDashboard/internal/store"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-www/silverlining"
)

func PostChatRead(ctx *silverlining.Context, conversationID string, body []byte) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	var req struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if req.MessageID == "" {
		writeChatError(ctx, http.StatusBadRequest, "message_id is required")
		return
	}

	repo := store.GetChatRepository()
	if err := repo.MarkMessagesReadUpTo(conversationID, req.MessageID, user.Email, user.Login); err != nil {
		switch {
		case errors.Is(err, store.ErrChatConversationNotFound):
			writeChatError(ctx, http.StatusNotFound, err.Error())
		case errors.Is(err, store.ErrChatMemberNotFound):
			writeChatError(ctx, http.StatusForbidden, "user is not a member of conversation")
		case errors.Is(err, store.ErrChatMessageNotFound):
			writeChatError(ctx, http.StatusNotFound, "message not found in conversation")
		default:
			writeChatError(ctx, http.StatusInternalServerError, err.Error())
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
