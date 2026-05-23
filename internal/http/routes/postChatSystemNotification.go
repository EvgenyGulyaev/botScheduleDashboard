package routes

import (
	"botDashboard/internal/event"
	"botDashboard/internal/event/producer"
	"botDashboard/internal/http/middleware"
	"botDashboard/internal/model"
	"botDashboard/internal/push"
	"botDashboard/internal/store"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/go-www/silverlining"
)

const systemNotificationTextLimit = 4000

type chatSystemNotificationBody struct {
	RecipientEmail string `json:"recipient_email"`
	Title          string `json:"title"`
	Text           string `json:"text"`
	Source         string `json:"source"`
	ExternalID     string `json:"external_id"`
	URL            string `json:"url"`
}

type chatSystemNotificationResponse struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
}

func PostChatSystemNotification(ctx *silverlining.Context, body []byte) {
	if !authorizeSystemNotificationRequest(ctx) {
		return
	}

	var payload chatSystemNotificationBody
	if err := json.Unmarshal(body, &payload); err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	recipientEmail := strings.TrimSpace(payload.RecipientEmail)
	if recipientEmail == "" {
		writeChatError(ctx, http.StatusBadRequest, "recipient_email is required")
		return
	}

	messageText, err := buildSystemNotificationText(payload)
	if err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	recipient, err := store.GetUserRepository().FindUserByEmail(recipientEmail)
	if err != nil {
		writeChatError(ctx, http.StatusNotFound, "recipient not found")
		return
	}

	result, err := store.GetChatRepository().AddSystemNotificationWithResult(model.ChatMember{
		Email: recipient.Email,
		Login: recipient.Login,
	}, messageText)
	if err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	if err := producer.PublishChatMessagePersistedEvent(event.ChatMessagePersistedEvent{
		Conversation: result.Conversation,
		Members:      result.Members,
		Message:      result.Message,
	}); err != nil {
		logChatError(err)
	}
	push.NotifyChatMembersAboutMessage(result.Conversation, result.Members, result.Message)
	if len(result.RemovedMessageIDs) > 0 {
		if err := producer.PublishChatConversationUpdatedEvent(event.ChatConversationUpdatedEvent{
			Conversation:      result.Conversation,
			Members:           result.Members,
			RemovedMessageIDs: result.RemovedMessageIDs,
		}); err != nil {
			logChatError(err)
		}
	}

	if err := ctx.WriteJSON(http.StatusOK, chatSystemNotificationResponse{
		ConversationID: result.Conversation.ID,
		MessageID:      result.Message.ID,
	}); err != nil {
		logChatError(err)
	}
}

func authorizeSystemNotificationRequest(ctx *silverlining.Context) bool {
	expectedToken := strings.TrimSpace(os.Getenv("SYSTEM_NOTIFICATIONS_API_TOKEN"))
	if expectedToken == "" {
		writeChatError(ctx, http.StatusServiceUnavailable, "system notifications token is not configured")
		return false
	}

	authHeader, ok := ctx.RequestHeaders().Get("Authorization")
	if !ok {
		writeChatError(ctx, http.StatusUnauthorized, "authorization required")
		return false
	}
	token, err := middleware.ParseBearerToken(authHeader)
	if err != nil || subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) != 1 {
		writeChatError(ctx, http.StatusUnauthorized, "invalid token")
		return false
	}
	return true
}

func buildSystemNotificationText(payload chatSystemNotificationBody) (string, error) {
	lines := make([]string, 0, 5)
	title := strings.TrimSpace(payload.Title)
	text := strings.TrimSpace(payload.Text)
	if title == "" && text == "" {
		return "", fmt.Errorf("title or text is required")
	}
	if title != "" {
		lines = append(lines, title)
	}
	if text != "" {
		lines = append(lines, text)
	}
	if source := strings.TrimSpace(payload.Source); source != "" {
		lines = append(lines, "Источник: "+source)
	}
	if externalID := strings.TrimSpace(payload.ExternalID); externalID != "" {
		lines = append(lines, "ID: "+externalID)
	}
	if url := strings.TrimSpace(payload.URL); url != "" {
		lines = append(lines, "Ссылка: "+url)
	}

	result := strings.Join(lines, "\n")
	if len([]rune(result)) > systemNotificationTextLimit {
		return "", fmt.Errorf("notification text is too long")
	}
	return result, nil
}
