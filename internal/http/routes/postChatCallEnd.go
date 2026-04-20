package routes

import (
	"botDashboard/internal/event"
	"botDashboard/internal/event/producer"
	"botDashboard/internal/store"
	"net/http"

	"github.com/go-www/silverlining"
)

func PostChatCallEnd(ctx *silverlining.Context, conversationID, callID string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}
	if _, err := conversationView(ctx, conversationID, user.Email); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	call, message, err := store.GetChatRepository().EndCall(conversationID, callID)
	if err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	conversation, members, snapshotErr := chatSnapshot(conversationID)
	if snapshotErr == nil {
		if publishErr := producer.PublishChatCallEndedEvent(event.ChatCallEndedEvent{
			Conversation: conversation,
			Members:      members,
			Call:         call,
			Message:      message,
		}); publishErr != nil {
			logChatError(publishErr)
		}
	}

	if err := ctx.WriteJSON(http.StatusOK, map[string]any{
		"ended":   true,
		"message": chatMessageDTOFromModel(message, nil),
	}); err != nil {
		logChatError(err)
	}
}
