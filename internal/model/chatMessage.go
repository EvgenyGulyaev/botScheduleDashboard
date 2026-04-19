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
	Type           string           `json:"type"`
	SenderEmail    string           `json:"sender_email"`
	SenderLogin    string           `json:"sender_login"`
	Text           string           `json:"text"`
	CreatedAt      time.Time        `json:"created_at"`
	DeliveredTo    []MessageReceipt `json:"delivered_to"`
	ReadBy         []MessageReceipt `json:"read_by"`
	Audio          *ChatAudio       `json:"audio,omitempty"`
}

type ChatAudio struct {
	ID              string     `json:"id"`
	MimeType        string     `json:"mime_type"`
	SizeBytes       int64      `json:"size_bytes"`
	DurationSeconds int        `json:"duration_seconds"`
	FilePath        string     `json:"file_path,omitempty"`
	ConsumedAt      *time.Time `json:"consumed_at,omitempty"`
}
