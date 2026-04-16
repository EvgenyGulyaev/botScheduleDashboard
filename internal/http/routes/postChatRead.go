package routes

import (
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"

	"github.com/go-www/silverlining"
)

func PostChatRead(ctx *silverlining.Context, conversationID string, body []byte) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	var req struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if req.MessageID == "" {
		writeChatError(ctx, http.StatusBadRequest, "message_id is required")
		return
	}

	repo := store.GetChatRepository()
	if _, err := repo.FindConversationByID(conversationID); err != nil {
		writeChatError(ctx, http.StatusNotFound, err.Error())
		return
	}

	members, err := repo.ListConversationMembers(conversationID)
	if err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if _, ok := findMember(members, user.Email); !ok {
		writeChatError(ctx, http.StatusForbidden, "user is not a member of conversation")
		return
	}

	messages, err := repo.ListMessages(conversationID)
	if err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	if _, ok := findMessage(messages, req.MessageID); !ok {
		writeChatError(ctx, http.StatusNotFound, "message not found in conversation")
		return
	}

	if err := repo.MarkMessagesReadUpTo(conversationID, req.MessageID, user.Email, user.Login); err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	view, err := conversationView(ctx, conversationID, user.Email)
	if err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, view); err != nil {
		logChatError(err)
	}
}

func findMessage(messages []model.ChatMessage, messageID string) (model.ChatMessage, bool) {
	for _, message := range messages {
		if message.ID == messageID {
			return message, true
		}
	}
	return model.ChatMessage{}, false
}
