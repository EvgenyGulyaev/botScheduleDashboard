package routes

import (
	"botDashboard/internal/store"
	"net/http"

	"github.com/go-www/silverlining"
)

type chatFavoritesDTO struct {
	Messages []chatMessageDTO `json:"messages"`
}

func GetChatFavorites(ctx *silverlining.Context) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	repoMessages, err := store.GetChatRepository().ListFavoriteMessages(user.Email)
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

	if err := ctx.WriteJSON(http.StatusOK, chatFavoritesDTO{Messages: messages}); err != nil {
		logChatError(err)
	}
}
