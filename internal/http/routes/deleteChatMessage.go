package routes

import (
	"botDashboard/internal/event"
	"botDashboard/internal/event/producer"
	"botDashboard/internal/store"
	"net/http"

	"github.com/go-www/silverlining"
)

func DeleteChatMessage(ctx *silverlining.Context, conversationID, messageID string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}
	if _, err := conversationView(ctx, conversationID, user.Email); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	result, err := store.GetChatRepository().DeleteMessage(conversationID, messageID, user.Email)
	if err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	conversation, members, err := chatSnapshot(conversationID)
	if err == nil {
		if publishErr := producer.PublishChatMessageDeletedEvent(event.ChatMessageDeletedEvent{
			Conversation:     conversation,
			Members:          members,
			MessageID:        result.DeletedMessageID,
			AffectedMessages: result.AffectedMessages,
		}); publishErr != nil {
			logChatError(publishErr)
		}
	}

	if err := ctx.WriteJSON(http.StatusOK, map[string]any{
		"message_id": result.DeletedMessageID,
	}); err != nil {
		logChatError(err)
	}
}
