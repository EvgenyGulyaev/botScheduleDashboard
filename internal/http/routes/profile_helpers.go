package routes

import (
	"botDashboard/internal/model"
	"botDashboard/internal/push"
)

type profileNotificationSettingsDTO struct {
	PushEnabled  bool `json:"push_enabled"`
	SoundEnabled bool `json:"sound_enabled"`
	ToastEnabled bool `json:"toast_enabled"`
}

type profilePushDTO struct {
	Supported bool   `json:"supported"`
	PublicKey string `json:"public_key"`
}

type profileDTO struct {
	Login                string                         `json:"login"`
	Email                string                         `json:"email"`
	IsAdmin              bool                           `json:"is_admin"`
	DefaultApp           string                         `json:"default_app"`
	NotificationSettings profileNotificationSettingsDTO `json:"notification_settings"`
	Push                 profilePushDTO                 `json:"push"`
}

func profileDTOFromUser(user model.UserData) profileDTO {
	return profileDTO{
		Login:   user.Login,
		Email:   user.Email,
		IsAdmin: user.IsAdmin,
		DefaultApp: user.DefaultApp,
		NotificationSettings: profileNotificationSettingsDTO{
			PushEnabled:  user.NotificationSettings.PushEnabled,
			SoundEnabled: user.NotificationSettings.SoundEnabled,
			ToastEnabled: user.NotificationSettings.ToastEnabled,
		},
		Push: profilePushDTO{
			Supported: push.Enabled(),
			PublicKey: push.PublicKey(),
		},
	}
}
