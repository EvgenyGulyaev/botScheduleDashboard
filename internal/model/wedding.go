package model

import "time"

const (
	WeddingAttendanceAttending    = "attending"
	WeddingAttendanceNotAttending = "not_attending"

	WeddingSettingsKey = "settings"

	DefaultWeddingAccessCode  = "171026"
	WeddingAccessMaxAttempts  = 3
	WeddingAccessLockDuration = 10 * time.Minute
)

type WeddingRSVP struct {
	ID         string    `json:"id"`
	FullName   string    `json:"full_name"`
	Attendance string    `json:"attendance"`
	Drinks     []string  `json:"drinks"`
	OtherDrink string    `json:"other_drink"`
	Song       string    `json:"song"`
	CreatedAt  time.Time `json:"created_at"`
}

type WeddingSettings struct {
	DrinkOptions      []string `json:"drink_options"`
	AccessCodeEnabled bool     `json:"access_code_enabled"`
	AccessCode        string   `json:"access_code,omitempty"`
	AccessCodeVersion string   `json:"access_code_version"`
}

type WeddingPublicSettings struct {
	DrinkOptions      []string `json:"drink_options"`
	AccessCodeEnabled bool     `json:"access_code_enabled"`
	AccessCodeVersion string   `json:"access_code_version"`
}

type WeddingAccessVerifyInput struct {
	Code string `json:"code"`
}

type WeddingAccessVerifyResult struct {
	OK                bool   `json:"ok"`
	Locked            bool   `json:"locked,omitempty"`
	AttemptsLeft      int    `json:"attempts_left,omitempty"`
	RetryAfterSeconds int    `json:"retry_after_seconds,omitempty"`
	AccessCodeVersion string `json:"access_code_version,omitempty"`
	Message           string `json:"message,omitempty"`
}

type WeddingAccessAttempt struct {
	IP             string    `json:"ip"`
	FailedAttempts int       `json:"failed_attempts"`
	BlockedUntil   time.Time `json:"blocked_until"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (settings WeddingSettings) Public() WeddingPublicSettings {
	return WeddingPublicSettings{
		DrinkOptions:      settings.DrinkOptions,
		AccessCodeEnabled: settings.AccessCodeEnabled,
		AccessCodeVersion: settings.AccessCodeVersion,
	}
}

func DefaultWeddingDrinkOptions() []string {
	return []string{
		"Белое сухое",
		"Белое полусладкое",
		"Красное сухое",
		"Красное полусладкое",
		"Шампанское брют",
		"Шампанское полусладкое",
		"Коньяк",
		"Водка",
		"Другое",
	}
}
