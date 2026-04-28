package model

type UserNotificationSettings struct {
	Configured   bool `json:"configured,omitempty"`
	PushEnabled  bool `json:"push_enabled"`
	SoundEnabled bool `json:"sound_enabled"`
	ToastEnabled bool `json:"toast_enabled"`
}

type UserAliceSettings struct {
	Configured        bool   `json:"configured,omitempty"`
	AccountID         string `json:"account_id"`
	HouseholdID       string `json:"household_id"`
	RoomID            string `json:"room_id"`
	DeviceID          string `json:"device_id"`
	ScenarioID        string `json:"scenario_id"`
	Voice             string `json:"voice"`
	Disabled          bool   `json:"disabled"`
	AnnounceSender    bool   `json:"announce_sender"`
	QuietHoursEnabled bool   `json:"quiet_hours_enabled"`
	QuietHoursStart   string `json:"quiet_hours_start"`
	QuietHoursEnd     string `json:"quiet_hours_end"`
}

const (
	DefaultAppChat       = "chat"
	DefaultAppDashboard  = "dashboard"
	DefaultAppMessages   = "messages"
	DefaultAppGeo3D      = "geo3d"
	DefaultAppShortLinks = "short-links"
	DefaultAppAlice      = "alice"
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
	IsSuperAdmin         bool                     `json:"is_super_admin"`
	DefaultApp           string                   `json:"default_app"`
	AppPermissions       []string                 `json:"app_permissions"`
	NotificationSettings UserNotificationSettings `json:"notification_settings"`
	AliceSettings        UserAliceSettings        `json:"alice_settings"`
}

func NormalizeDefaultApp(value string) string {
	switch value {
	case DefaultAppDashboard, DefaultAppMessages, DefaultAppGeo3D, DefaultAppShortLinks, DefaultAppChat, DefaultAppAlice:
		return value
	default:
		return DefaultAppChat
	}
}

func AllAppPermissions(isAdmin, isSuperAdmin bool) []string {
	apps := []string{DefaultAppMessages, DefaultAppChat, DefaultAppGeo3D, DefaultAppShortLinks}
	if isAdmin || isSuperAdmin {
		apps = append(apps, DefaultAppAlice)
	}
	if isSuperAdmin {
		apps = append([]string{DefaultAppDashboard}, apps...)
	}
	return apps
}

func NormalizeAppPermissions(values []string, isAdmin, isSuperAdmin bool) []string {
	allowed := map[string]bool{}
	for _, app := range AllAppPermissions(isAdmin, isSuperAdmin) {
		allowed[app] = true
	}
	if len(values) == 0 {
		return AllAppPermissions(isAdmin, isSuperAdmin)
	}

	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		app := NormalizeDefaultApp(value)
		if !allowed[app] || seen[app] {
			continue
		}
		seen[app] = true
		result = append(result, app)
	}
	if len(result) == 0 {
		return []string{DefaultAppChat}
	}
	return result
}

func AppAllowed(value string, permissions []string) bool {
	app := NormalizeDefaultApp(value)
	for _, permission := range permissions {
		if permission == app {
			return true
		}
	}
	return false
}

func NormalizeDefaultAppForPermissions(value string, permissions []string) string {
	app := NormalizeDefaultApp(value)
	if AppAllowed(app, permissions) {
		return app
	}
	if AppAllowed(DefaultAppChat, permissions) {
		return DefaultAppChat
	}
	if len(permissions) > 0 {
		return permissions[0]
	}
	return DefaultAppChat
}
