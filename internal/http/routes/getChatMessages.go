package routes

import (
	"botDashboard/internal/store"
	"net/http"

	"github.com/go-www/silverlining"
)

func GetChatMessages(ctx *silverlining.Context, conversationID string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	if _, err := conversationView(ctx, conversationID, user.Email); err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	repoMessages, err := store.GetChatRepository().ListMessages(conversationID)
	if err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	hydratedMessages, replyLookup, err := hydrateMessagesForResponse(repoMessages)
	if err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	messages := make([]chatMessageDTO, 0, len(hydratedMessages))
	for _, message := range hydratedMessages {
		messages = append(messages, chatMessageDTOFromModel(message, replyLookup))
	}

	if err := ctx.WriteJSON(http.StatusOK, messages); err != nil {
		logChatError(err)
	}
}
