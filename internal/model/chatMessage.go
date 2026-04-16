package model

import "time"

type MessageReceipt struct {
	Email string    `json:"email"`
	Login string    `json:"login"`
	At    time.Time `json:"at"`
}

type ChatMessage struct {
	ID             string           `json:"id"`
	ConversationID string           `json:"conversation_id"`
	SenderEmail    string           `json:"sender_email"`
	SenderLogin    string           `json:"sender_login"`
	Text           string           `json:"text"`
	CreatedAt      time.Time        `json:"created_at"`
	DeliveredTo    []MessageReceipt `json:"delivered_to"`
	ReadBy         []MessageReceipt `json:"read_by"`
}
