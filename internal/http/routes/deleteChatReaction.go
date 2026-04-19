package routes

import (
	"botDashboard/internal/event"
	"botDashboard/internal/event/producer"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"net/http"

	"github.com/go-www/silverlining"
)

func DeleteChatReaction(ctx *silverlining.Context, conversationID, messageID string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}
	if _, err := conversationView(ctx, conversationID, user.Email); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	message, err := store.GetChatRepository().DeleteMessageReaction(conversationID, messageID, user.Email)
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
