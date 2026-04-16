package model

import "time"

type ChatConversation struct {
	ID              string    `json:"id"`
	Type            string    `json:"type"`
	Title           string    `json:"title"`
	CreatedByEmail  string    `json:"created_by_email"`
	CreatedByLogin  string    `json:"created_by_login"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	LastMessageID   string    `json:"last_message_id"`
	LastMessageText string    `json:"last_message_text"`
	LastMessageAt   time.Time `json:"last_message_at"`
}
