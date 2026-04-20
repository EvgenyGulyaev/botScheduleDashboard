package routes

import (
	"botDashboard/internal/event"
	"botDashboard/internal/event/producer"
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"

	"github.com/go-www/silverlining"
)

type chatCallMuteBody struct {
	Muted bool `json:"muted"`
}

func PutChatCallMute(ctx *silverlining.Context, conversationID, callID string, body []byte) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}
	if _, err := conversationView(ctx, conversationID, user.Email); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	var payload chatCallMuteBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			writeChatError(ctx, http.StatusBadRequest, err.Error())
			return
		}
	}

	call, err := store.GetChatRepository().SetCallMuted(conversationID, callID, user.Email, payload.Muted)
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
