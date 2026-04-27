package routes

import (
	"net/http"

	"github.com/go-www/silverlining"
)

type chatConversationWithDraftDTO struct {
	chatConversationDTO
	Draft chatDraftDTO `json:"draft"`
}

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

	response := make([]chatConversationWithDraftDTO, 0, len(conversations))
	for _, conversation := range conversations {
		response = append(response, chatConversationWithDraftDTO{
			chatConversationDTO: conversation,
			Draft:               chatDraftDTOForUser(conversation.ID, user.Email),
		})
	}

	if err := ctx.WriteJSON(http.StatusOK, response); err != nil {
		logChatError(err)
	}
}
