package routes

import (
	"botDashboard/internal/store"
	"net/http"

	"github.com/go-www/silverlining"
)

func DeleteChatFavorite(ctx *silverlining.Context, conversationID, messageID string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	message, err := store.GetChatRepository().DeleteMessageFavorite(conversationID, messageID, user.Email)
	if err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}
	if err := ctx.WriteJSON(http.StatusOK, chatMessageDTOFromModel(message, nil)); err != nil {
		logChatError(err)
	}
}
