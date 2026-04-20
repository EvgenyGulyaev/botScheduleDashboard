package event

import "botDashboard/internal/model"

type ChatMessageSendCommand struct {
	ConversationID   string `json:"conversation_id"`
	RecipientEmail   string `json:"recipient_email,omitempty"`
	SenderEmail      string `json:"sender_email"`
	SenderLogin      string `json:"sender_login"`
	Text             string `json:"text"`
	ReplyToMessageID string `json:"reply_to_message_id,omitempty"`
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

type ChatMessageUpdatedEvent struct {
	Conversation model.ChatConversation `json:"conversation"`
	Members      []model.ChatMember     `json:"members"`
	Message      model.ChatMessage      `json:"message"`
}

type ChatMessageDeletedEvent struct {
	Conversation     model.ChatConversation `json:"conversation"`
	Members          []model.ChatMember     `json:"members"`
	MessageID        string                 `json:"message_id"`
	AffectedMessages []model.ChatMessage    `json:"affected_messages,omitempty"`
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

type ChatCallStartedEvent struct {
	Conversation model.ChatConversation `json:"conversation"`
	Members      []model.ChatMember     `json:"members"`
	Call         model.ChatCall         `json:"call"`
	Message      model.ChatMessage      `json:"message"`
}

type ChatCallUpdatedEvent struct {
	Conversation model.ChatConversation `json:"conversation"`
	Members      []model.ChatMember     `json:"members"`
	Call         model.ChatCall         `json:"call"`
	Message      model.ChatMessage      `json:"message"`
}

type ChatCallEndedEvent struct {
	Conversation model.ChatConversation `json:"conversation"`
	Members      []model.ChatMember     `json:"members"`
	Call         model.ChatCall         `json:"call"`
	Message      model.ChatMessage      `json:"message"`
}
