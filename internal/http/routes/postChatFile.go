package routes

import (
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

func PostChatFile(ctx *silverlining.Context, conversationID string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	postChatFileForUser(ctx, conversationID, user)
}

func PostChatFileWithToken(ctx *silverlining.Context, conversationID, tokenStr string) {
	user, err := chatUserFromTokenString(tokenStr)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	postChatFileForUser(ctx, conversationID, user)
}

func postChatFileForUser(ctx *silverlining.Context, conversationID string, user model.UserData) {
	if _, err := conversationView(ctx, conversationID, user.Email); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	upload, fileBytes, err := parseFileUpload(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if int64(len(fileBytes)) > store.CHAT_FILE_MAX_BYTES {
		writeChatError(ctx, http.StatusBadRequest, fmt.Sprintf("file size must be <= %d bytes", store.CHAT_FILE_MAX_BYTES))
		return
	}

	if err := os.MkdirAll(store.CHAT_FILE_DIR, 0750); err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	filePath := filepath.Join(store.CHAT_FILE_DIR, storedFileName(upload.Filename, upload.MimeType))
	if err := os.WriteFile(filePath, fileBytes, 0600); err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	upload.FilePath = filePath
	upload.SizeBytes = int64(len(fileBytes))
	result, err := store.GetChatRepository().AddFileMessageWithResult(conversationID, user.Email, user.Login, upload)
	if err != nil {
		_ = os.Remove(filePath)
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if result.Message.File != nil && result.Message.File.FilePath != filePath {
		_ = os.Remove(filePath)
	}
	if !result.Created {
		if err := ctx.WriteJSON(http.StatusOK, chatMessageDTOFromModel(result.Message, nil)); err != nil {
			logChatError(err)
		}
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

func parseFileUpload(ctx *silverlining.Context) (store.ChatFileUpload, []byte, error) {
	reader, err := ctx.MultipartReader()
	if err != nil {
		return store.ChatFileUpload{}, nil, err
	}
	defer ctx.CloseBodyReader()

	upload := store.ChatFileUpload{}
	for {
		part, err := reader.NextPart()
		if err != nil {
			if err == io.EOF {
				break
			}
			return store.ChatFileUpload{}, nil, err
		}

		if part.FormName() == "client_message_id" {
			raw, err := io.ReadAll(io.LimitReader(part, 256))
			closeErr := part.Close()
			if err != nil {
				return store.ChatFileUpload{}, nil, err
			}
			if closeErr != nil {
				return store.ChatFileUpload{}, nil, closeErr
			}
			upload.ClientMessageID = strings.TrimSpace(string(raw))
			continue
		}

		if part.FormName() != "file" {
			if closeErr := part.Close(); closeErr != nil {
				return store.ChatFileUpload{}, nil, closeErr
			}
			continue
		}

		fileBytes, err := io.ReadAll(io.LimitReader(part, store.CHAT_FILE_MAX_BYTES+1))
		mimeType := part.Header.Get("Content-Type")
		filename := sanitizeChatFileName(part.FileName())
		closeErr := part.Close()
		if err != nil {
			return store.ChatFileUpload{}, nil, err
		}
		if closeErr != nil {
			return store.ChatFileUpload{}, nil, closeErr
		}
		if len(fileBytes) == 0 {
			return store.ChatFileUpload{}, nil, fmt.Errorf("file is empty")
		}
		if mimeType == "" {
			mimeType = http.DetectContentType(fileBytes)
		}
		upload.Filename = filename
		upload.MimeType = mimeType
		return upload, fileBytes, nil
	}

	return store.ChatFileUpload{}, nil, fmt.Errorf("file is required")
}

func sanitizeChatFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == string(filepath.Separator) {
		name = ""
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\' || r == 0:
			return -1
		case r < 32:
			return -1
		default:
			return r
		}
	}, name)
	if strings.TrimSpace(name) == "" {
		return "file"
	}
	return name
}

func storedFileName(originalName, mimeType string) string {
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return fmt.Sprintf("%d-%s", time.Now().UTC().UnixNano(), sanitizeChatFileName(originalName))
	}

	extension := filepath.Ext(originalName)
	if extension == "" {
		if extensions, err := mime.ExtensionsByType(mimeType); err == nil && len(extensions) > 0 {
			extension = extensions[0]
		}
	}
	return fmt.Sprintf("%d-%s%s", time.Now().UTC().UnixNano(), hex.EncodeToString(randomBytes), extension)
}
