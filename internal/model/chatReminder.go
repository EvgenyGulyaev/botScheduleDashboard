package model

import "time"

type ChatReminder struct {
	ID                string    `json:"id"`
	UserEmail         string    `json:"user_email"`
	UserLogin         string    `json:"user_login"`
	ConversationID    string    `json:"conversation_id"`
	ConversationTitle string    `json:"conversation_title"`
	MessageID         string    `json:"message_id"`
	MessageText       string    `json:"message_text"`
	SenderEmail       string    `json:"sender_email"`
	SenderLogin       string    `json:"sender_login"`
	RemindAt          time.Time `json:"remind_at"`
	CreatedAt         time.Time `json:"created_at"`
}
