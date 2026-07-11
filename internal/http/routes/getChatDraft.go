package routes

import (
	"botDashboard/internal/store"
	"net/http"
	"time"

	"github.com/go-www/silverlining"
)

type chatDraftDTO struct {
	Text      string     `json:"text"`
	UpdatedAt *time.Time `json:"updated_at"`
}

func chatDraftDTOFromStore(draft store.ChatDraft) chatDraftDTO {
	var updatedAt *time.Time
	if !draft.UpdatedAt.IsZero() {
		value := draft.UpdatedAt
		updatedAt = &value
	}
	return chatDraftDTO{
		Text:      draft.Text,
		UpdatedAt: updatedAt,
	}
}

func GetChatDraft(ctx *silverlining.Context, conversationID string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}
	if err := ensureConversationAccess(user, conversationID); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	draft, ok, err := store.GetChatRepository().GetChatDraft(conversationID, user.Email)
	if err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	payload := chatDraftDTO{}
	if ok {
		payload = chatDraftDTOFromStore(draft)
	}
	if err := ctx.WriteJSON(http.StatusOK, payload); err != nil {
		logChatError(err)
	}
}
