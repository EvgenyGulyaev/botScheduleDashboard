package routes

import (
	"botDashboard/internal/store"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-www/silverlining"
)

func GetChatFile(ctx *silverlining.Context, conversationID, messageID string) {
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
	if message.Type != "file" || message.File == nil {
		writeChatError(ctx, http.StatusBadRequest, "message is not a file message")
		return
	}
	if !message.File.ExpiresAt.IsZero() && !message.File.ExpiresAt.After(time.Now().UTC()) {
		if _, err := repo.CleanupExpiredFileMessages(); err != nil {
			logChatError(err)
		}
		writeChatError(ctx, http.StatusGone, "file expired")
		return
	}
	if message.File.ExpiredAt != nil {
		writeChatError(ctx, http.StatusGone, "file expired")
		return
	}
	if message.File.ConsumedAt != nil {
		writeChatError(ctx, http.StatusGone, "file already consumed")
		return
	}
	if message.File.FilePath == "" {
		writeChatError(ctx, http.StatusNotFound, "file not found")
		return
	}

	data, err := os.ReadFile(message.File.FilePath)
	if err != nil {
		writeChatError(ctx, http.StatusNotFound, err.Error())
		return
	}

	filePath := message.File.FilePath
	filename := sanitizeChatFileName(message.File.Filename)
	message, err = repo.ConsumeFileMessage(conversationID, messageID, user.Email, user.Login)
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

	contentType := message.File.MimeType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	ctx.ResponseHeaders().Set("Content-Type", contentType)
	ctx.ResponseHeaders().Set("Cache-Control", "no-store")
	ctx.ResponseHeaders().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
		"filename": filename,
	}))
	if err := ctx.WriteFullBody(http.StatusOK, data); err != nil {
		logChatError(err)
	}
}
