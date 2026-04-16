package model

import "time"

type ChatMember struct {
	ConversationID    string    `json:"conversation_id"`
	Email             string    `json:"email"`
	Login             string    `json:"login"`
	JoinedAt          time.Time `json:"joined_at"`
	LastReadMessageID string    `json:"last_read_message_id"`
}
