package model

import "time"

const (
	ChatMemberRoleOwner  = "owner"
	ChatMemberRoleAdmin  = "admin"
	ChatMemberRoleMember = "member"
)

type ChatMember struct {
	ConversationID    string    `json:"conversation_id"`
	Email             string    `json:"email"`
	Login             string    `json:"login"`
	Role              string    `json:"role"`
	JoinedAt          time.Time `json:"joined_at"`
	LastReadMessageID string    `json:"last_read_message_id"`
}
