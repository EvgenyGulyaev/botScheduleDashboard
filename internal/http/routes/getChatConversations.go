package routes

import (
	"botDashboard/internal/model"
	"botDashboard/internal/store"
	"net/http"

	"github.com/go-www/silverlining"
)

type chatConversationWithDraftDTO struct {
	chatConversationDTO
	LastReadMessageID string       `json:"last_read_message_id"`
	Draft             chatDraftDTO `json:"draft"`
}

func GetChatConversations(ctx *silverlining.Context) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	if _, err := store.GetChatRepository().EnsureSystemConversationForUser(model.ChatMember{
		Email: user.Email,
		Login: user.Login,
	}); err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	conversations, err := conversationViewsForUser(user.Email)
	if err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	drafts, _ := store.GetChatRepository().ListChatDraftsForUser(user.Email)

	response := make([]chatConversationWithDraftDTO, 0, len(conversations))
	for _, conversation := range conversations {
		draft := chatDraftDTO{}
		if storedDraft, ok := drafts[conversation.ID]; ok {
			draft = chatDraftDTOFromStore(storedDraft)
		}
		response = append(response, chatConversationWithDraftDTO{
			chatConversationDTO: conversation,
			LastReadMessageID:   lastReadMessageIDFromView(conversation.Members, user.Email),
			Draft:               draft,
		})
	}

	if err := ctx.WriteJSON(http.StatusOK, response); err != nil {
		logChatError(err)
	}
}
