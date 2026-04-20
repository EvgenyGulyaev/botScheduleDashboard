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
	"strconv"
	"strings"
	"time"

	"github.com/go-www/silverlining"
)

func PostChatAudio(ctx *silverlining.Context, conversationID string, body []byte, contentType string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	postChatAudioForUser(ctx, conversationID, user, body, contentType)
}

func PostChatAudioWithToken(ctx *silverlining.Context, conversationID, tokenStr string, body []byte, contentType string) {
	user, err := chatUserFromTokenString(tokenStr)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	postChatAudioForUser(ctx, conversationID, user, body, contentType)
}

func postChatAudioForUser(ctx *silverlining.Context, conversationID string, user model.UserData, body []byte, contentType string) {
	if _, err := conversationView(ctx, conversationID, user.Email); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	durationSeconds, audioBytes, mimeType, err := parseAudioUpload(contentType, body)
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

func parseAudioUpload(contentType string, body []byte) (int, []byte, string, error) {
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
		return 0, nil, "", fmt.Errorf("multipart boundary is required")
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	form, err := reader.ReadForm(store.CHAT_AUDIO_MAX_BYTES)
	if err != nil {
		return 0, nil, "", err
	}
	defer func() {
		_ = form.RemoveAll()
	}()

	durationSeconds, err := strconv.Atoi(firstFormValue(form, "duration_seconds"))
	if err != nil || durationSeconds <= 0 {
		return 0, nil, "", fmt.Errorf("duration_seconds is required")
	}

	files := form.File["audio"]
	if len(files) == 0 {
		return 0, nil, "", fmt.Errorf("audio file is required")
	}
	file, err := files[0].Open()
	if err != nil {
		return 0, nil, "", err
	}
	defer func() {
		_ = file.Close()
	}()

	audioBytes, err := io.ReadAll(io.LimitReader(file, store.CHAT_AUDIO_MAX_BYTES+1))
	if err != nil {
		return 0, nil, "", err
	}
	if len(audioBytes) == 0 {
		return 0, nil, "", fmt.Errorf("audio file is empty")
	}

	mimeType := files[0].Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = http.DetectContentType(audioBytes)
	}
	return durationSeconds, audioBytes, mimeType, nil
}

func fallbackMultipartMediaType(contentType string) (string, map[string]string, error) {
	normalized := strings.TrimSpace(contentType)
	if normalized == "" {
		return "", nil, fmt.Errorf("content-type is required")
	}

	lower := strings.ToLower(normalized)
	if !strings.Contains(lower, "multipart/") {
		return "", nil, fmt.Errorf("content-type must be multipart")
	}

	boundaryIndex := strings.Index(lower, "boundary=")
	if boundaryIndex == -1 {
		return "multipart/form-data", map[string]string{}, nil
	}

	rawBoundary := normalized[boundaryIndex+len("boundary="):]
	if separator := strings.Index(rawBoundary, ";"); separator >= 0 {
		rawBoundary = rawBoundary[:separator]
	}

	boundary := strings.Trim(strings.TrimSpace(rawBoundary), `"`)
	if boundary == "" {
		return "multipart/form-data", map[string]string{}, nil
	}

	return "multipart/form-data", map[string]string{"boundary": boundary}, nil
}

func detectMultipartBoundaryFromBody(body []byte) string {
	if len(body) < 4 || body[0] != '-' || body[1] != '-' {
		return ""
	}

	lineEnd := bytes.Index(body, []byte("\r\n"))
	if lineEnd == -1 {
		lineEnd = bytes.IndexByte(body, '\n')
	}
	if lineEnd <= 2 {
		return ""
	}

	return strings.TrimSpace(string(body[2:lineEnd]))
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

func firstFormValue(form *multipart.Form, key string) string {
	values := form.Value[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
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
