package routes

import (
	"net/http"

	"github.com/go-www/silverlining"
)

func GetChatConversations(ctx *silverlining.Context) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	conversations, err := conversationViewsForUser(ctx, user.Email)
	if err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, conversations); err != nil {
		logChatError(err)
	}
}
