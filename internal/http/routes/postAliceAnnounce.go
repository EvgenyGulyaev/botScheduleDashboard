package routes

import (
	"botDashboard/internal/alice"
	"botDashboard/internal/event"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"

	"github.com/go-www/silverlining"
)

type postAliceAnnounceBody struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	Text           string `json:"text"`
}

func PostAliceAnnounce(ctx *silverlining.Context, body []byte) {
	user, err := currentChatUser(ctx)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusUnauthorized})
		return
	}

	var payload postAliceAnnounceBody
	if err := json.Unmarshal(body, &payload); err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadRequest})
		return
	}
	if payload.ConversationID == "" {
		GetError(ctx, &Error{Message: "conversation_id is required", Status: http.StatusBadRequest})
		return
	}

	chatRepo := store.GetChatRepository()
	conversation, err := chatRepo.FindConversationByID(payload.ConversationID)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusNotFound})
		return
	}

	members, err := chatRepo.ListConversationMembers(payload.ConversationID)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	message := model.ChatMessage{
		ConversationID: payload.ConversationID,
		Type:           "text",
		SenderEmail:    user.Email,
		SenderLogin:    user.Login,
		Text:           payload.Text,
	}
	if payload.MessageID != "" {
		storedMessage, err := chatRepo.FindMessageForMember(payload.ConversationID, payload.MessageID, user.Email)
		if err != nil {
			GetError(ctx, &Error{Message: err.Error(), Status: http.StatusNotFound})
			return
		}
		message = storedMessage
	}
	if payload.MessageID == "" && message.Text == "" {
		GetError(ctx, &Error{Message: "text or message_id is required", Status: http.StatusBadRequest})
		return
	}
	if !alice.NewClient().Enabled() {
		GetError(ctx, &Error{Message: "alice service is not configured", Status: http.StatusServiceUnavailable})
		return
	}

	updatedMessage, deliveries := event.AnnounceChatMessageOnAliceWithCount(
		user.Email,
		user.Login,
		true,
		conversation,
		members,
		message,
	)
	if deliveries == 0 {
		GetError(ctx, &Error{Message: "no Alice recipients are available right now", Status: http.StatusBadRequest})
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, map[string]any{
		"status":          "sent",
		"deliveries":      deliveries,
		"message_id":      updatedMessage.ID,
		"alice_announced": updatedMessage.AliceAnnounced,
	}); err != nil {
		logChatError(err)
	}
}
