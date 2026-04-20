package model

type UserNotificationSettings struct {
	Configured   bool `json:"configured,omitempty"`
	PushEnabled  bool `json:"push_enabled"`
	SoundEnabled bool `json:"sound_enabled"`
	ToastEnabled bool `json:"toast_enabled"`
}

func DefaultUserNotificationSettings() UserNotificationSettings {
	return UserNotificationSettings{
		Configured:   true,
		PushEnabled:  false,
		SoundEnabled: true,
		ToastEnabled: true,
	}
}

type UserData struct {
	Login                string                   `json:"login"`
	Email                string                   `json:"email"`
	HashedPassword       []byte                   `json:"hashed_password"`
	IsAdmin              bool                     `json:"is_admin"`
	NotificationSettings UserNotificationSettings `json:"notification_settings"`
}
