package routes

import (
	"botDashboard/internal/store"
	"net/http"
	"os"
	"strings"

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

	message, err = repo.ConsumeAudioMessage(conversationID, messageID, user.Email)
	if err != nil {
		if strings.Contains(err.Error(), "already consumed") {
			writeChatError(ctx, http.StatusGone, err.Error())
			return
		}
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}
	if err := os.Remove(message.Audio.FilePath); err != nil && !os.IsNotExist(err) {
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
