package routes

import (
	"botDashboard/internal/alice"
	"botDashboard/internal/store"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-www/silverlining"
)

type postAliceAnnounceBody struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
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
	if conversation.Type != "direct" {
		GetError(ctx, &Error{Message: "alice announce is supported only for direct conversations", Status: http.StatusBadRequest})
		return
	}

	members, err := chatRepo.ListConversationMembers(payload.ConversationID)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusInternalServerError})
		return
	}

	var recipientEmail string
	for _, member := range members {
		if member.Email != user.Email {
			recipientEmail = member.Email
			break
		}
	}
	if recipientEmail == "" {
		GetError(ctx, &Error{Message: "recipient for alice announce not found", Status: http.StatusBadRequest})
		return
	}

	recipient, err := store.GetUserRepository().FindUserByEmail(recipientEmail)
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusNotFound})
		return
	}
	if recipient.AliceSettings.AccountID == "" || recipient.AliceSettings.ScenarioID == "" {
		GetError(ctx, &Error{Message: fmt.Sprintf("user %s has not configured Alice speaker settings", recipient.Login), Status: http.StatusBadRequest})
		return
	}

	client := alice.NewClient()
	if !client.Enabled() {
		GetError(ctx, &Error{Message: "alice service is not configured", Status: http.StatusServiceUnavailable})
		return
	}

	response, err := client.AnnounceScenario(alice.AnnounceRequest{
		AccountID:      recipient.AliceSettings.AccountID,
		ScenarioID:     recipient.AliceSettings.ScenarioID,
		InitiatorEmail: user.Email,
		RecipientEmail: recipient.Email,
		ConversationID: payload.ConversationID,
		MessageID:      payload.MessageID,
	})
	if err != nil {
		GetError(ctx, &Error{Message: err.Error(), Status: http.StatusBadGateway})
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, response); err != nil {
		logChatError(err)
	}
}
