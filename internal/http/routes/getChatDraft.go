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

func chatDraftDTOForUser(conversationID, currentUserEmail string) chatDraftDTO {
	draft, ok, err := store.GetChatRepository().GetChatDraft(conversationID, currentUserEmail)
	if err != nil || !ok {
		return chatDraftDTO{}
	}
	return chatDraftDTOFromStore(draft)
}

func GetChatDraft(ctx *silverlining.Context, conversationID string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}
	if _, err := conversationView(ctx, conversationID, user.Email); err != nil {
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
