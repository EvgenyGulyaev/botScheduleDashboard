package routes

import (
	"botDashboard/internal/store"
	"net/http"

	"github.com/go-www/silverlining"
)

func DeleteChatDirect(ctx *silverlining.Context, conversationID string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	conversation, err := conversationView(ctx, conversationID, user.Email)
	if err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}
	if conversation.Type != "direct" {
		writeChatError(ctx, http.StatusBadRequest, "conversation is not a direct chat")
		return
	}

	if err := store.GetChatRepository().DeleteDirectConversation(conversationID); err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	if err := ctx.WriteJSON(http.StatusOK, map[string]string{"message": "ok"}); err != nil {
		logChatError(err)
	}
}
