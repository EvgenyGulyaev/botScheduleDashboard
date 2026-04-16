package event

import "botDashboard/internal/model"

type ChatMessageSendCommand struct {
	ConversationID string `json:"conversation_id"`
	RecipientEmail string `json:"recipient_email,omitempty"`
	SenderEmail    string `json:"sender_email"`
	SenderLogin    string `json:"sender_login"`
	Text           string `json:"text"`
}

type ChatMessageReadCommand struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	ReaderEmail    string `json:"reader_email"`
	ReaderLogin    string `json:"reader_login"`
}

type ChatParticipant struct {
	Email string `json:"email"`
	Login string `json:"login"`
}

type ChatMessagePersistedEvent struct {
	Conversation model.ChatConversation `json:"conversation"`
	Members      []model.ChatMember     `json:"members"`
	Message      model.ChatMessage      `json:"message"`
}

type ChatMessageReadUpdatedEvent struct {
	Conversation       model.ChatConversation `json:"conversation"`
	Members            []model.ChatMember     `json:"members"`
	MessageID          string                 `json:"message_id"`
	Message            model.ChatMessage      `json:"message"`
	Reader             ChatParticipant        `json:"reader"`
	AffectedMessageIDs []string               `json:"affected_message_ids,omitempty"`
}

type ChatConversationUpdatedEvent struct {
	Conversation      model.ChatConversation `json:"conversation"`
	Members           []model.ChatMember     `json:"members"`
	RemovedMessageIDs []string               `json:"removed_message_ids"`
}
