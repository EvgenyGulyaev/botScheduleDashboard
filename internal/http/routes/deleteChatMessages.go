package routes

import (
	"botDashboard/internal/event"
	"botDashboard/internal/event/producer"
	"botDashboard/internal/store"
	"net/http"

	"github.com/go-www/silverlining"
)

func DeleteChatMessages(ctx *silverlining.Context, conversationID string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}
	if err := ensureConversationAccess(user, conversationID); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	result, err := store.GetChatRepository().ClearConversationMessages(conversationID, user.Email)
	if err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if conversation, members, err := chatSnapshot(conversationID); err == nil {
		if publishErr := producer.PublishChatConversationUpdatedEvent(event.ChatConversationUpdatedEvent{
			Conversation:      conversation,
			Members:           members,
			RemovedMessageIDs: result.DeletedMessageIDs,
		}); publishErr != nil {
			logChatError(publishErr)
		}
	}

	if err := ctx.WriteJSON(http.StatusOK, map[string]any{
		"deleted_message_ids": result.DeletedMessageIDs,
	}); err != nil {
		logChatError(err)
	}
}
