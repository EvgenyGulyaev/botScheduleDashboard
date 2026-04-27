package routes

import (
	"botDashboard/internal/event"
	"botDashboard/internal/event/producer"
	"botDashboard/internal/store"
	"encoding/json"
	"net/http"

	"github.com/go-www/silverlining"
)

type chatForwardBody struct {
	SourceConversationID string   `json:"source_conversation_id"`
	MessageIDs           []string `json:"message_ids"`
}

func PostChatForward(ctx *silverlining.Context, targetConversationID string, body []byte) {
	user, err := currentChatUser(ctx)
	if err != nil {
		writeChatError(ctx, http.StatusUnauthorized, err.Error())
		return
	}

	var payload chatForwardBody
	if err := json.Unmarshal(body, &payload); err != nil {
		writeChatError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	result, err := store.GetChatRepository().ForwardMessages(
		targetConversationID,
		payload.SourceConversationID,
		payload.MessageIDs,
		user.Email,
		user.Login,
	)
	if err != nil {
		writeChatError(ctx, http.StatusForbidden, err.Error())
		return
	}

	for _, message := range result.Messages {
		if publishErr := producer.PublishChatMessagePersistedEvent(event.ChatMessagePersistedEvent{
			Conversation: result.Conversation,
			Members:      result.Members,
			Message:      message,
		}); publishErr != nil {
			logChatError(publishErr)
		}
	}
	if len(result.RemovedMessageIDs) > 0 {
		if publishErr := producer.PublishChatConversationUpdatedEvent(event.ChatConversationUpdatedEvent{
			Conversation:      result.Conversation,
			Members:           result.Members,
			RemovedMessageIDs: result.RemovedMessageIDs,
		}); publishErr != nil {
			logChatError(publishErr)
		}
	}

	hydratedMessages, replyLookup, err := hydrateMessagesForResponse(result.Messages)
	if err != nil {
		writeChatError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	messages := make([]chatMessageDTO, 0, len(hydratedMessages))
	for _, message := range hydratedMessages {
		messages = append(messages, chatMessageDTOFromModel(message, replyLookup))
	}

	if err := ctx.WriteJSON(http.StatusOK, chatMessagesDTO{Messages: messages}); err != nil {
		logChatError(err)
	}
}
