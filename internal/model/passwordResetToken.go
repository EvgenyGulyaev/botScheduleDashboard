package model

import "time"

type PasswordResetToken struct {
	Email     string     `json:"email"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
}
