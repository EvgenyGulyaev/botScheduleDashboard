package routes

import (
	"botDashboard/internal/event"
	"botDashboard/internal/event/producer"
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-www/silverlining"
)

func GetChatAudio(ctx *silverlining.Context, conversationID, messageID string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	repo := store.GetChatRepository()
	message, err := repo.FindMessageForMember(conversationID, messageID, user.Email)
	if err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}
	if message.Type != "audio" || message.Audio == nil {
		writeChatError(ctx, http.StatusBadRequest, "message is not an audio message")
		return
	}
	if !message.Audio.ExpiresAt.IsZero() && !message.Audio.ExpiresAt.After(time.Now().UTC()) {
		if _, err := repo.CleanupExpiredAudioMessages(); err != nil {
			logChatError(err)
		}
		writeChatError(ctx, http.StatusGone, "audio expired")
		return
	}
	if message.Audio.ExpiredAt != nil {
		writeChatError(ctx, http.StatusGone, "audio expired")
		return
	}
	if message.Audio.ConsumedAt != nil {
		writeChatError(ctx, http.StatusGone, "audio already consumed")
		return
	}
	if message.Audio.FilePath == "" {
		writeChatError(ctx, http.StatusNotFound, "audio file not found")
		return
	}

	data, err := os.ReadFile(message.Audio.FilePath)
	if err != nil {
		writeChatError(ctx, http.StatusNotFound, err.Error())
		return
	}

	filePath := message.Audio.FilePath
	message, err = repo.ConsumeAudioMessage(conversationID, messageID, user.Email, user.Login)
	if err != nil {
		if strings.Contains(err.Error(), "already consumed") || strings.Contains(err.Error(), "expired") {
			writeChatError(ctx, http.StatusGone, err.Error())
			return
		}
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		logChatError(err)
	}
	if err := publishChatMessageConsumed(conversationID, message); err != nil {
		logChatError(err)
	}

	contentType := message.Audio.MimeType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	ctx.ResponseHeaders().Set("Content-Type", contentType)
	ctx.ResponseHeaders().Set("Cache-Control", "no-store")
	if err := ctx.WriteFullBody(http.StatusOK, data); err != nil {
		logChatError(err)
	}
}

func publishChatMessageConsumed(conversationID string, message model.ChatMessage) error {
	repo := store.GetChatRepository()
	conversation, err := repo.FindConversationByID(conversationID)
	if err != nil {
		return err
	}
	members, err := repo.ListConversationMembers(conversationID)
	if err != nil {
		return err
	}
	return producer.PublishChatMessageReadUpdatedEvent(event.ChatMessageReadUpdatedEvent{
		Conversation:       conversation,
		Members:            members,
		MessageID:          message.ID,
		Message:            message,
		AffectedMessageIDs: []string{message.ID},
	})
}
