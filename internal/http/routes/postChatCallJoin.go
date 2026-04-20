package routes

import (
	"botDashboard/internal/event"
	"botDashboard/internal/event/producer"
	"botDashboard/internal/store"
	"net/http"

	"github.com/go-www/silverlining"
)

func PostChatCallJoin(ctx *silverlining.Context, conversationID, callID string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}
	if _, err := conversationView(ctx, conversationID, user.Email); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	call, err := store.GetChatRepository().JoinCall(conversationID, callID, user.Email, user.Login)
	if err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	conversation, members, err := chatSnapshot(conversationID)
	if err == nil {
		message, messageErr := store.GetChatRepository().FindMessageForMember(conversationID, call.MessageID, user.Email)
		if messageErr == nil {
			if publishErr := producer.PublishChatCallUpdatedEvent(event.ChatCallUpdatedEvent{
				Conversation: conversation,
				Members:      members,
				Call:         call,
				Message:      message,
			}); publishErr != nil {
				logChatError(publishErr)
			}
		}
	}

	if err := ctx.WriteJSON(http.StatusOK, chatCallDTOFromModel(call)); err != nil {
		logChatError(err)
	}
}
