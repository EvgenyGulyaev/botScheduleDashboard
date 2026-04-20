package routes

import (
	"botDashboard/internal/event"
	"botDashboard/internal/event/producer"
	"botDashboard/internal/model"
	"botDashboard/internal/push"
	"botDashboard/internal/store"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-www/silverlining"
)

func PostChatImage(ctx *silverlining.Context, conversationID string, body []byte, contentType string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	postChatImageForUser(ctx, conversationID, user, body, contentType)
}

func PostChatImageWithToken(ctx *silverlining.Context, conversationID, tokenStr string, body []byte, contentType string) {
	user, err := chatUserFromTokenString(tokenStr)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	postChatImageForUser(ctx, conversationID, user, body, contentType)
}

func postChatImageForUser(ctx *silverlining.Context, conversationID string, user model.UserData, body []byte, contentType string) {
	if _, err := conversationView(ctx, conversationID, user.Email); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	imageBytes, mimeType, err := parseImageUpload(contentType, body)
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

func parseImageUpload(contentType string, body []byte) ([]byte, string, error) {
	if contentType == "" {
		contentType = "multipart/form-data"
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType, params, err = fallbackMultipartMediaType(contentType)
		if err != nil {
			mediaType = "multipart/form-data"
			params = map[string]string{}
		}
	}
	if !strings.HasPrefix(strings.ToLower(mediaType), "multipart/") {
		mediaType = "multipart/form-data"
	}
	boundary := params["boundary"]
	if boundary == "" {
		boundary = detectMultipartBoundaryFromBody(body)
	}
	if boundary == "" {
		return nil, "", fmt.Errorf("multipart boundary is required")
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	form, err := reader.ReadForm(store.CHAT_IMAGE_MAX_BYTES)
	if err != nil {
		return nil, "", err
	}
	defer func() {
		_ = form.RemoveAll()
	}()

	files := form.File["image"]
	if len(files) == 0 {
		return nil, "", fmt.Errorf("image file is required")
	}
	file, err := files[0].Open()
	if err != nil {
		return nil, "", err
	}
	defer func() {
		_ = file.Close()
	}()

	imageBytes, err := io.ReadAll(io.LimitReader(file, store.CHAT_IMAGE_MAX_BYTES+1))
	if err != nil {
		return nil, "", err
	}
	if len(imageBytes) == 0 {
		return nil, "", fmt.Errorf("image file is empty")
	}

	mimeType := files[0].Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = http.DetectContentType(imageBytes)
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return nil, "", fmt.Errorf("image file must be an image")
	}
	return imageBytes, mimeType, nil
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
