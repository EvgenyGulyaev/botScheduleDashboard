package event

import (
	"botDashboard/internal/model"
	"time"
)

type ChatMessageSendCommand struct {
	ConversationID   string `json:"conversation_id"`
	RecipientEmail   string `json:"recipient_email,omitempty"`
	ClientMessageID  string `json:"client_message_id,omitempty"`
	SenderEmail      string `json:"sender_email"`
	SenderLogin      string `json:"sender_login"`
	Text             string `json:"text"`
	ReplyToMessageID string `json:"reply_to_message_id,omitempty"`
	AnnounceOnAlice  bool   `json:"announce_on_alice,omitempty"`
}

type ChatMessageReadCommand struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	ReaderEmail    string `json:"reader_email"`
	ReaderLogin    string `json:"reader_login"`
}

type ChatMessageDeliveredCommand struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	RecipientEmail string `json:"recipient_email"`
	RecipientLogin string `json:"recipient_login"`
}

type ChatPresenceCommand struct {
	UserEmail string `json:"user_email"`
	UserLogin string `json:"user_login"`
	Online    bool   `json:"online"`
}

type ChatTypingCommand struct {
	ConversationID string `json:"conversation_id"`
	UserEmail      string `json:"user_email"`
	UserLogin      string `json:"user_login"`
	Kind           string `json:"kind"`
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

type ChatMessageDeliveredEvent struct {
	Conversation model.ChatConversation `json:"conversation"`
	Members      []model.ChatMember     `json:"members"`
	MessageID    string                 `json:"message_id"`
	Message      model.ChatMessage      `json:"message"`
	Recipient    ChatParticipant        `json:"recipient"`
}

type ChatConversationUpdatedEvent struct {
	Conversation      model.ChatConversation `json:"conversation"`
	Members           []model.ChatMember     `json:"members"`
	RemovedMessageIDs []string               `json:"removed_message_ids"`
}

type ChatPresenceUpdatedEvent struct {
	ConversationID string                 `json:"conversation_id"`
	Members        []model.ChatMember     `json:"members"`
	User           ChatParticipant        `json:"user"`
	Presence       model.ChatUserPresence `json:"presence"`
}

type ChatTypingEvent struct {
	ConversationID string             `json:"conversation_id"`
	Members        []model.ChatMember `json:"members"`
	User           ChatParticipant    `json:"user"`
	Kind           string             `json:"kind"`
	StartedAt      time.Time          `json:"started_at,omitempty"`
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
