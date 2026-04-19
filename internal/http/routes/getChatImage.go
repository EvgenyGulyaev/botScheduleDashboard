package routes

import (
	"botDashboard/internal/store"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-www/silverlining"
)

func GetChatImage(ctx *silverlining.Context, conversationID, messageID string) {
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
	if message.Type != "image" || message.Image == nil {
		writeChatError(ctx, http.StatusBadRequest, "message is not an image message")
		return
	}
	if !message.Image.ExpiresAt.IsZero() && !message.Image.ExpiresAt.After(time.Now().UTC()) {
		if _, err := repo.CleanupExpiredImageMessages(); err != nil {
			logChatError(err)
		}
		writeChatError(ctx, http.StatusGone, "image expired")
		return
	}
	if message.Image.ExpiredAt != nil {
		writeChatError(ctx, http.StatusGone, "image expired")
		return
	}
	if message.Image.ConsumedAt != nil {
		writeChatError(ctx, http.StatusGone, "image already consumed")
		return
	}
	if message.Image.FilePath == "" {
		writeChatError(ctx, http.StatusNotFound, "image file not found")
		return
	}

	data, err := os.ReadFile(message.Image.FilePath)
	if err != nil {
		writeChatError(ctx, http.StatusNotFound, err.Error())
		return
	}

	filePath := message.Image.FilePath
	message, err = repo.ConsumeImageMessage(conversationID, messageID, user.Email, user.Login)
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

	contentType := message.Image.MimeType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	ctx.ResponseHeaders().Set("Content-Type", contentType)
	ctx.ResponseHeaders().Set("Cache-Control", "no-store")
	if err := ctx.WriteFullBody(http.StatusOK, data); err != nil {
		logChatError(err)
	}
}
