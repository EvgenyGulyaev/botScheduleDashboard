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
	"strings"
	"time"

	"github.com/go-www/silverlining"
)

func PostChatImage(ctx *silverlining.Context, conversationID string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	postChatImageForUser(ctx, conversationID, user)
}

func PostChatImageWithToken(ctx *silverlining.Context, conversationID, tokenStr string) {
	user, err := chatUserFromTokenString(tokenStr)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	postChatImageForUser(ctx, conversationID, user)
}

func postChatImageForUser(ctx *silverlining.Context, conversationID string, user model.UserData) {
	if _, err := conversationView(ctx, conversationID, user.Email); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	imageBytes, mimeType, err := parseImageUpload(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if int64(len(imageBytes)) > store.CHAT_IMAGE_MAX_BYTES {
		writeChatError(ctx, http.StatusBadRequest, fmt.Sprintf("image size must be <= %d bytes", store.CHAT_IMAGE_MAX_BYTES))
		return
	}

	if err := os.MkdirAll(store.CHAT_IMAGE_DIR, 0750); err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	filePath := filepath.Join(store.CHAT_IMAGE_DIR, imageFileName(mimeType))
	if err := os.WriteFile(filePath, imageBytes, 0600); err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	result, err := store.GetChatRepository().AddImageMessageWithResult(conversationID, user.Email, user.Login, store.ChatImageUpload{
		FilePath:  filePath,
		MimeType:  mimeType,
		SizeBytes: int64(len(imageBytes)),
	})
	if err != nil {
		_ = os.Remove(filePath)
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	if err := publishImageMessagePersisted(conversationID, result.Message); err != nil {
		logChatError(err)
	}
	if conversation, members, err := chatSnapshot(conversationID); err == nil {
		push.NotifyChatMembersAboutMessage(conversation, members, result.Message)
	}
	if len(result.RemovedMessageIDs) > 0 {
		if err := publishImageConversationUpdated(conversationID, result.RemovedMessageIDs); err != nil {
			logChatError(err)
		}
	}

	if err := ctx.WriteJSON(http.StatusOK, chatMessageDTOFromModel(result.Message, nil)); err != nil {
		logChatError(err)
	}
}

func parseImageUpload(ctx *silverlining.Context) ([]byte, string, error) {
	reader, err := ctx.MultipartReader()
	if err != nil {
		return nil, "", err
	}
	defer ctx.CloseBodyReader()

	for {
		part, err := reader.NextPart()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, "", err
		}

		if part.FormName() != "image" {
			if closeErr := part.Close(); closeErr != nil {
				return nil, "", closeErr
			}
			continue
		}

		imageBytes, err := io.ReadAll(io.LimitReader(part, store.CHAT_IMAGE_MAX_BYTES+1))
		mimeType := part.Header.Get("Content-Type")
		closeErr := part.Close()
		if err != nil {
			return nil, "", err
		}
		if closeErr != nil {
			return nil, "", closeErr
		}
		if len(imageBytes) == 0 {
			return nil, "", fmt.Errorf("image file is empty")
		}
		if mimeType == "" {
			mimeType = http.DetectContentType(imageBytes)
		}
		if !strings.HasPrefix(mimeType, "image/") {
			return nil, "", fmt.Errorf("image file must be an image")
		}
		return imageBytes, mimeType, nil
	}

	return nil, "", fmt.Errorf("image file is required")
}

func publishImageMessagePersisted(conversationID string, message model.ChatMessage) error {
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

func publishImageConversationUpdated(conversationID string, removedMessageIDs []string) error {
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

func imageFileName(mimeType string) string {
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return fmt.Sprintf("%d.png", time.Now().UTC().UnixNano())
	}

	extension := ".png"
	if extensions, err := mime.ExtensionsByType(mimeType); err == nil && len(extensions) > 0 {
		extension = extensions[0]
	}
	return fmt.Sprintf("%d-%s%s", time.Now().UTC().UnixNano(), hex.EncodeToString(randomBytes), extension)
}
