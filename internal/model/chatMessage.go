package model

import "time"

type MessageReceipt struct {
	Email string    `json:"email"`
	Login string    `json:"login"`
	At    time.Time `json:"at"`
}

type ChatReaction struct {
	ConversationID string    `json:"conversation_id"`
	MessageID      string    `json:"message_id"`
	UserEmail      string    `json:"user_email"`
	UserLogin      string    `json:"user_login"`
	Emoji          string    `json:"emoji"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ChatMessage struct {
	ID               string           `json:"id"`
	ConversationID   string           `json:"conversation_id"`
	Type             string           `json:"type"`
	SenderEmail      string           `json:"sender_email"`
	SenderLogin      string           `json:"sender_login"`
	Text             string           `json:"text"`
	AliceAnnounced   bool             `json:"alice_announced,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at,omitempty"`
	EditedAt         *time.Time       `json:"edited_at,omitempty"`
	ReplyToMessageID string           `json:"reply_to_message_id,omitempty"`
	DeliveredTo      []MessageReceipt `json:"delivered_to"`
	ReadBy           []MessageReceipt `json:"read_by"`
	Reactions        []ChatReaction   `json:"reactions,omitempty"`
	Audio            *ChatAudio       `json:"audio,omitempty"`
	Image            *ChatImage       `json:"image,omitempty"`
	Call             *ChatCallMessage `json:"call,omitempty"`
}

type ChatAudio struct {
	ID              string     `json:"id"`
	MimeType        string     `json:"mime_type"`
	SizeBytes       int64      `json:"size_bytes"`
	DurationSeconds int        `json:"duration_seconds"`
	FilePath        string     `json:"file_path,omitempty"`
	ExpiresAt       time.Time  `json:"expires_at,omitempty"`
	ConsumedAt      *time.Time `json:"consumed_at,omitempty"`
	ConsumedByEmail string     `json:"consumed_by_email,omitempty"`
	ConsumedByLogin string     `json:"consumed_by_login,omitempty"`
	ExpiredAt       *time.Time `json:"expired_at,omitempty"`
}

type ChatImage struct {
	ID              string     `json:"id"`
	MimeType        string     `json:"mime_type"`
	SizeBytes       int64      `json:"size_bytes"`
	FilePath        string     `json:"file_path,omitempty"`
	ExpiresAt       time.Time  `json:"expires_at,omitempty"`
	ConsumedAt      *time.Time `json:"consumed_at,omitempty"`
	ConsumedByEmail string     `json:"consumed_by_email,omitempty"`
	ConsumedByLogin string     `json:"consumed_by_login,omitempty"`
	ExpiredAt       *time.Time `json:"expired_at,omitempty"`
}
