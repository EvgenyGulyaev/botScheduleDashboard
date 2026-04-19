package routes

import (
	"botDashboard/internal/event"
	"botDashboard/internal/event/producer"
	"botDashboard/internal/store"
	"net/http"

	"github.com/go-www/silverlining"
)

func DeleteChatPin(ctx *silverlining.Context, conversationID string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}
	if _, err := conversationView(ctx, conversationID, user.Email); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	if _, err := store.GetChatRepository().ClearPinnedMessage(conversationID, user.Email); err != nil {
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
