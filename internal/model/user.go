package model

type UserNotificationSettings struct {
	Configured   bool `json:"configured,omitempty"`
	PushEnabled  bool `json:"push_enabled"`
	SoundEnabled bool `json:"sound_enabled"`
	ToastEnabled bool `json:"toast_enabled"`
}

type UserAliceSettings struct {
	Configured  bool   `json:"configured,omitempty"`
	AccountID   string `json:"account_id"`
	HouseholdID string `json:"household_id"`
	RoomID      string `json:"room_id"`
	DeviceID    string `json:"device_id"`
	ScenarioID  string `json:"scenario_id"`
}

const (
	DefaultAppChat       = "chat"
	DefaultAppDashboard  = "dashboard"
	DefaultAppMessages   = "messages"
	DefaultAppGeo3D      = "geo3d"
	DefaultAppShortLinks = "short-links"
)

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
	DefaultApp           string                   `json:"default_app"`
	NotificationSettings UserNotificationSettings `json:"notification_settings"`
	AliceSettings        UserAliceSettings        `json:"alice_settings"`
}

func NormalizeDefaultApp(value string) string {
	switch value {
	case DefaultAppDashboard, DefaultAppMessages, DefaultAppGeo3D, DefaultAppShortLinks, DefaultAppChat:
		return value
	default:
		return DefaultAppChat
	}
}
