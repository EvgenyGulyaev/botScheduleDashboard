package routes

import (
	"botDashboard/internal/store"
	"net/http"

	"github.com/go-www/silverlining"
)

type chatMessagesDTO struct {
	Messages          []chatMessageDTO `json:"messages"`
	LastReadMessageID string           `json:"last_read_message_id"`
}

func GetChatMessages(ctx *silverlining.Context, conversationID string) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	view, err := conversationView(ctx, conversationID, user.Email)
	if err != nil {
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

	if err := ctx.WriteJSON(http.StatusOK, chatMessagesDTO{
		Messages:          messages,
		LastReadMessageID: lastReadMessageIDFromView(view.Members, user.Email),
	}); err != nil {
		logChatError(err)
	}
}

func lastReadMessageIDFromView(members []chatMemberDTO, email string) string {
	for _, member := range members {
		if member.Email == email {
			return member.LastReadMessageID
		}
	}
	return ""
}
