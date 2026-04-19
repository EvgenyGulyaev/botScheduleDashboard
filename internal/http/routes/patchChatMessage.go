package routes

import (
	"botDashboard/internal/event"
	"botDashboard/internal/event/producer"
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-www/silverlining"
)

func PatchChatMessage(ctx *silverlining.Context, conversationID, messageID string, body []byte) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}
	if _, err := conversationView(ctx, conversationID, user.Email); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	var payload chatMessagePatchBody
	if err := json.Unmarshal(body, &payload); err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	payload.Text = strings.TrimSpace(payload.Text)
	if payload.Text == "" {
		writeChatError(ctx, http.StatusBadRequest, "text is required")
		return
	}

	message, err := store.GetChatRepository().UpdateTextMessage(conversationID, messageID, user.Email, payload.Text)
	if err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	conversation, members, err := chatSnapshot(conversationID)
	if err == nil {
		if publishErr := producer.PublishChatMessageUpdatedEvent(event.ChatMessageUpdatedEvent{
			Conversation: conversation,
			Members:      members,
			Message:      message,
		}); publishErr != nil {
			logChatError(publishErr)
		}
	}

	if err := ctx.WriteJSON(http.StatusOK, chatMessageDTOFromModel(message, nil)); err != nil {
		logChatError(err)
	}
}
