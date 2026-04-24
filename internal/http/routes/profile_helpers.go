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

type profileAliceSettingsDTO struct {
	AccountID         string `json:"account_id"`
	HouseholdID       string `json:"household_id"`
	RoomID            string `json:"room_id"`
	DeviceID          string `json:"device_id"`
	ScenarioID        string `json:"scenario_id"`
	Voice             string `json:"voice"`
	Disabled          bool   `json:"disabled"`
	QuietHoursEnabled bool   `json:"quiet_hours_enabled"`
	QuietHoursStart   string `json:"quiet_hours_start"`
	QuietHoursEnd     string `json:"quiet_hours_end"`
}

type profileDTO struct {
	Login                string                         `json:"login"`
	Email                string                         `json:"email"`
	IsAdmin              bool                           `json:"is_admin"`
	DefaultApp           string                         `json:"default_app"`
	NotificationSettings profileNotificationSettingsDTO `json:"notification_settings"`
	Push                 profilePushDTO                 `json:"push"`
	AliceSettings        profileAliceSettingsDTO        `json:"alice_settings"`
}

func profileDTOFromUser(user model.UserData) profileDTO {
	return profileDTO{
		Login:      user.Login,
		Email:      user.Email,
		IsAdmin:    user.IsAdmin,
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
		AliceSettings: profileAliceSettingsDTO{
			AccountID:         user.AliceSettings.AccountID,
			HouseholdID:       user.AliceSettings.HouseholdID,
			RoomID:            user.AliceSettings.RoomID,
			DeviceID:          user.AliceSettings.DeviceID,
			ScenarioID:        user.AliceSettings.ScenarioID,
			Voice:             user.AliceSettings.Voice,
			Disabled:          user.AliceSettings.Disabled,
			QuietHoursEnabled: user.AliceSettings.QuietHoursEnabled,
			QuietHoursStart:   user.AliceSettings.QuietHoursStart,
			QuietHoursEnd:     user.AliceSettings.QuietHoursEnd,
		},
	}
}
