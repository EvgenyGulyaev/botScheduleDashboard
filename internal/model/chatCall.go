package model

import "time"

type ChatCallParticipant struct {
	Email    string    `json:"email"`
	Login    string    `json:"login"`
	JoinedAt time.Time `json:"joined_at"`
	Muted    bool      `json:"muted"`
}

type ChatCall struct {
	ID              string                `json:"id"`
	ConversationID  string                `json:"conversation_id"`
	MessageID       string                `json:"message_id"`
	StartedByEmail  string                `json:"started_by_email"`
	StartedByLogin  string                `json:"started_by_login"`
	StartedAt       time.Time             `json:"started_at"`
	EndedAt         *time.Time            `json:"ended_at,omitempty"`
	MaxParticipants int                   `json:"max_participants"`
	Participants    []ChatCallParticipant `json:"participants"`
}

type ChatCallMessage struct {
	CallID           string     `json:"call_id"`
	StartedByEmail   string     `json:"started_by_email"`
	StartedByLogin   string     `json:"started_by_login"`
	StartedAt        time.Time  `json:"started_at"`
	EndedAt          *time.Time `json:"ended_at,omitempty"`
	Joinable         bool       `json:"joinable"`
	ParticipantCount int        `json:"participant_count"`
}
