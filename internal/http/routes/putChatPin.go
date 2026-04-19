package routes

import (
	"botDashboard/internal/event"
	"botDashboard/internal/event/producer"
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"

	"github.com/go-www/silverlining"
)

type chatPinBody struct {
	MessageID string `json:"message_id"`
}

func PutChatPin(ctx *silverlining.Context, conversationID string, body []byte) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}
	if _, err := conversationView(ctx, conversationID, user.Email); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	var payload chatPinBody
	if err := json.Unmarshal(body, &payload); err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if payload.MessageID == "" {
		writeChatError(ctx, http.StatusBadRequest, "message_id is required")
		return
	}

	if _, err := store.GetChatRepository().SetPinnedMessage(conversationID, payload.MessageID, user.Email, user.Login); err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	view, err := conversationView(ctx, conversationID, user.Email)
	if err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if conversation, members, err := chatSnapshot(conversationID); err == nil {
		if publishErr := producer.PublishChatConversationUpdatedEvent(event.ChatConversationUpdatedEvent{
			Conversation: conversation,
			Members:      members,
		}); publishErr != nil {
			logChatError(publishErr)
		}
	}

	if err := ctx.WriteJSON(http.StatusOK, view); err != nil {
		logChatError(err)
	}
}
