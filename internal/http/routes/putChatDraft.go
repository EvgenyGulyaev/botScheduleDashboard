package routes

import (
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"

	"github.com/go-www/silverlining"
)

type chatDraftBody struct {
	Text string `json:"text"`
}

func PutChatDraft(ctx *silverlining.Context, conversationID string, body []byte) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}
	if _, err := conversationView(ctx, conversationID, user.Email); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	var payload chatDraftBody
	if err := json.Unmarshal(body, &payload); err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if payload.Text == "" {
		if err := store.GetChatRepository().ClearChatDraft(conversationID, user.Email); err != nil {
			writeChatError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		if err := ctx.WriteJSON(http.StatusOK, chatDraftDTO{}); err != nil {
			logChatError(err)
		}
		return
	}

	draft, err := store.GetChatRepository().SaveChatDraft(conversationID, user.Email, payload.Text)
	if err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, chatDraftDTOFromStore(draft)); err != nil {
		logChatError(err)
	}
}
