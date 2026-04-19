package routes

import (
	"botDashboard/internal/store"
	"net/http"

	"github.com/go-www/silverlining"
)

type chatSearchResultDTO struct {
	ConversationID    string `json:"conversation_id"`
	ConversationTitle string `json:"conversation_title"`
	MessageID         string `json:"message_id"`
	SenderEmail       string `json:"sender_email"`
	SenderLogin       string `json:"sender_login"`
	Text              string `json:"text"`
}

func GetChatSearch(ctx *silverlining.Context) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	query, _ := ctx.GetQueryParamString("q")
	results, err := store.GetChatRepository().SearchTextMessagesForUser(user.Email, query)
	if err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response := make([]chatSearchResultDTO, 0, len(results))
	for _, result := range results {
		response = append(response, chatSearchResultDTO{
			ConversationID:    result.ConversationID,
			ConversationTitle: result.ConversationTitle,
			MessageID:         result.Message.ID,
			SenderEmail:       result.Message.SenderEmail,
			SenderLogin:       result.Message.SenderLogin,
			Text:              result.Message.Text,
		})
	}

	if err := ctx.WriteJSON(http.StatusOK, response); err != nil {
		logChatError(err)
	}
}
