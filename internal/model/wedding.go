package model

import "time"

const (
	WeddingAttendanceAttending    = "attending"
	WeddingAttendanceNotAttending = "not_attending"

	WeddingSettingsKey = "settings"
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
	DrinkOptions []string `json:"drink_options"`
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
