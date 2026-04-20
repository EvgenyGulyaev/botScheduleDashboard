package routes

import (
	"botDashboard/internal/event"
	"botDashboard/internal/event/producer"
	"botDashboard/internal/model"
	"botDashboard/internal/push"
	"botDashboard/internal/store"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-www/silverlining"
)

func PostChatAudio(ctx *silverlining.Context, conversationID string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	postChatAudioForUser(ctx, conversationID, user)
}

func PostChatAudioWithToken(ctx *silverlining.Context, conversationID, tokenStr string) {
	user, err := chatUserFromTokenString(tokenStr)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	postChatAudioForUser(ctx, conversationID, user)
}

func postChatAudioForUser(ctx *silverlining.Context, conversationID string, user model.UserData) {
	if _, err := conversationView(ctx, conversationID, user.Email); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	durationSeconds, audioBytes, mimeType, err := parseAudioUpload(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if durationSeconds > store.CHAT_AUDIO_MAX_SECONDS {
		writeChatError(ctx, http.StatusBadRequest, fmt.Sprintf("audio duration must be <= %d seconds", store.CHAT_AUDIO_MAX_SECONDS))
		return
	}
	if int64(len(audioBytes)) > store.CHAT_AUDIO_MAX_BYTES {
		writeChatError(ctx, http.StatusBadRequest, fmt.Sprintf("audio size must be <= %d bytes", store.CHAT_AUDIO_MAX_BYTES))
		return
	}

	if err := os.MkdirAll(store.CHAT_AUDIO_DIR, 0750); err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	filePath := filepath.Join(store.CHAT_AUDIO_DIR, audioFileName(mimeType))
	if err := os.WriteFile(filePath, audioBytes, 0600); err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	result, err := store.GetChatRepository().AddAudioMessageWithResult(conversationID, user.Email, user.Login, store.ChatAudioUpload{
		FilePath:        filePath,
		MimeType:        mimeType,
		SizeBytes:       int64(len(audioBytes)),
		DurationSeconds: durationSeconds,
	})
	if err != nil {
		_ = os.Remove(filePath)
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	if err := publishAudioMessagePersisted(conversationID, result.Message); err != nil {
		logChatError(err)
	}
	if conversation, members, err := chatSnapshot(conversationID); err == nil {
		push.NotifyChatMembersAboutMessage(conversation, members, result.Message)
	}
	if len(result.RemovedMessageIDs) > 0 {
		if err := publishAudioConversationUpdated(conversationID, result.RemovedMessageIDs); err != nil {
			logChatError(err)
		}
	}

	if err := ctx.WriteJSON(http.StatusOK, chatMessageDTOFromModel(result.Message, nil)); err != nil {
		logChatError(err)
	}
}

func parseAudioUpload(ctx *silverlining.Context) (int, []byte, string, error) {
	reader, err := ctx.MultipartReader()
	if err != nil {
		return 0, nil, "", err
	}
	defer ctx.CloseBodyReader()

	var durationSeconds int
	var audioBytes []byte
	var mimeType string

	for {
		part, err := reader.NextPart()
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, nil, "", err
		}

		fieldName := part.FormName()
		switch fieldName {
		case "duration_seconds":
			durationRaw, err := io.ReadAll(io.LimitReader(part, 64))
			if err != nil {
				_ = part.Close()
				return 0, nil, "", err
			}
			durationSeconds, err = strconv.Atoi(string(durationRaw))
			if err != nil || durationSeconds <= 0 {
				_ = part.Close()
				return 0, nil, "", fmt.Errorf("duration_seconds is required")
			}
		case "audio":
			audioBytes, err = io.ReadAll(io.LimitReader(part, store.CHAT_AUDIO_MAX_BYTES+1))
			if err != nil {
				_ = part.Close()
				return 0, nil, "", err
			}
			mimeType = part.Header.Get("Content-Type")
		}

		if closeErr := part.Close(); closeErr != nil {
			return 0, nil, "", closeErr
		}
	}

	if durationSeconds <= 0 {
		return 0, nil, "", fmt.Errorf("duration_seconds is required")
	}
	if len(audioBytes) == 0 {
		return 0, nil, "", fmt.Errorf("audio file is required")
	}
	if mimeType == "" {
		mimeType = http.DetectContentType(audioBytes)
	}
	return durationSeconds, audioBytes, mimeType, nil
}

func publishAudioMessagePersisted(conversationID string, message model.ChatMessage) error {
	repo := store.GetChatRepository()
	conversation, err := repo.FindConversationByID(conversationID)
	if err != nil {
		return err
	}
	members, err := repo.ListConversationMembers(conversationID)
	if err != nil {
		return err
	}
	return producer.PublishChatMessagePersistedEvent(event.ChatMessagePersistedEvent{
		Conversation: conversation,
		Members:      members,
		Message:      message,
	})
}

func publishAudioConversationUpdated(conversationID string, removedMessageIDs []string) error {
	repo := store.GetChatRepository()
	conversation, err := repo.FindConversationByID(conversationID)
	if err != nil {
		return err
	}
	members, err := repo.ListConversationMembers(conversationID)
	if err != nil {
		return err
	}
	return producer.PublishChatConversationUpdatedEvent(event.ChatConversationUpdatedEvent{
		Conversation:      conversation,
		Members:           members,
		RemovedMessageIDs: removedMessageIDs,
	})
}

func audioFileName(mimeType string) string {
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return fmt.Sprintf("%d.webm", time.Now().UTC().UnixNano())
	}

	extension := ".webm"
	if extensions, err := mime.ExtensionsByType(mimeType); err == nil && len(extensions) > 0 {
		extension = extensions[0]
	}
	return fmt.Sprintf("%d-%s%s", time.Now().UTC().UnixNano(), hex.EncodeToString(randomBytes), extension)
}
