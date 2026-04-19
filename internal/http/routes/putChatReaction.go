package routes

import (
	"botDashboard/internal/event"
	"botDashboard/internal/event/producer"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-www/silverlining"
)

type chatReactionBody struct {
	Emoji string `json:"emoji"`
}

func PutChatReaction(ctx *silverlining.Context, conversationID, messageID string, body []byte) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}
	if _, err := conversationView(ctx, conversationID, user.Email); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	var payload chatReactionBody
	if err := json.Unmarshal(body, &payload); err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	payload.Emoji = strings.TrimSpace(payload.Emoji)
	if payload.Emoji == "" {
		writeChatError(ctx, http.StatusBadRequest, "emoji is required")
		return
	}

	message, err := store.GetChatRepository().SetMessageReaction(conversationID, messageID, user.Email, user.Login, payload.Emoji)
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

	replyLookup := map[string]model.ChatMessage{}
	if message.ReplyToMessageID != "" {
		if source, err := store.GetChatRepository().FindMessageForMember(conversationID, message.ReplyToMessageID, user.Email); err == nil {
			replyLookup[source.ID] = source
		}
	}
	if err := ctx.WriteJSON(http.StatusOK, chatMessageDTOFromModel(message, replyLookup)); err != nil {
		logChatError(err)
	}
}
