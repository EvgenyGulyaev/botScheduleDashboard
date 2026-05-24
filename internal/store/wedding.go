package store

import (
	"botDashboard/internal/model"
	"botDashboard/pkg/db"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.etcd.io/bbolt"
)

const (
	weddingNameMaxLength  = 120
	weddingTextMaxLength  = 160
	weddingIDFormat       = "%020d"
)

type WeddingRepository struct {
	repo *db.Repository
}

func GetWeddingRepository() *WeddingRepository {
	return &WeddingRepository{
		repo: db.GetRepository(),
	}
}

func (wr *WeddingRepository) CreateRSVP(input model.WeddingRSVP) (model.WeddingRSVP, error) {
	rsvp, err := normalizeWeddingRSVP(input)
	if err != nil {
		return model.WeddingRSVP{}, err
	}
	if rsvp.CreatedAt.IsZero() {
		rsvp.CreatedAt = time.Now().UTC()
	}

	err = wr.repo.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(WeddingRSVPBucket)
		if b == nil {
			return fmt.Errorf("wedding rsvp bucket not found")
		}
		seq, err := b.NextSequence()
		if err != nil {
			return err
		}
		rsvp.ID = fmt.Sprintf(weddingIDFormat, seq)
		data, err := json.Marshal(rsvp)
		if err != nil {
			return err
		}
		return b.Put([]byte(rsvp.ID), data)
	})
	if err != nil {
		return model.WeddingRSVP{}, err
	}
	return rsvp, nil
}

func (wr *WeddingRepository) ListRSVPs() ([]model.WeddingRSVP, error) {
	result := make([]model.WeddingRSVP, 0)
	err := wr.repo.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(WeddingRSVPBucket)
		if b == nil {
			return fmt.Errorf("wedding rsvp bucket not found")
		}
		cursor := b.Cursor()
		for key, value := cursor.Last(); key != nil; key, value = cursor.Prev() {
			var item model.WeddingRSVP
			if err := json.Unmarshal(value, &item); err != nil {
				return err
			}
			result = append(result, item)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].ID > result[j].ID
	})
	return result, nil
}

func (wr *WeddingRepository) GetSettings() (model.WeddingSettings, error) {
	var settings model.WeddingSettings
	err := wr.repo.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(WeddingSettingsBucket)
		if b == nil {
			return fmt.Errorf("wedding settings bucket not found")
		}
		data := b.Get([]byte(model.WeddingSettingsKey))
		if data == nil {
			settings = model.WeddingSettings{DrinkOptions: model.DefaultWeddingDrinkOptions()}
			return nil
		}
		return json.Unmarshal(data, &settings)
	})
	if err != nil {
		return model.WeddingSettings{}, err
	}
	settings.DrinkOptions = normalizeWeddingStringList(settings.DrinkOptions, weddingTextMaxLength)
	if len(settings.DrinkOptions) == 0 {
		settings.DrinkOptions = model.DefaultWeddingDrinkOptions()
	}
	return settings, nil
}

func (wr *WeddingRepository) SaveSettings(input model.WeddingSettings) (model.WeddingSettings, error) {
	settings := model.WeddingSettings{
		DrinkOptions: normalizeWeddingStringList(input.DrinkOptions, weddingTextMaxLength),
	}
	if len(settings.DrinkOptions) == 0 {
		return model.WeddingSettings{}, fmt.Errorf("drink options are required")
	}

	data, err := json.Marshal(settings)
	if err != nil {
		return model.WeddingSettings{}, err
	}
	err = wr.repo.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(WeddingSettingsBucket)
		if b == nil {
			return fmt.Errorf("wedding settings bucket not found")
		}
		return b.Put([]byte(model.WeddingSettingsKey), data)
	})
	if err != nil {
		return model.WeddingSettings{}, err
	}
	return settings, nil
}

func (wr *WeddingRepository) ClearAll() error {
	if err := wr.repo.ClearBucket(WeddingRSVPBucket); err != nil {
		return err
	}
	return wr.repo.ClearBucket(WeddingSettingsBucket)
}

func normalizeWeddingRSVP(input model.WeddingRSVP) (model.WeddingRSVP, error) {
	rsvp := model.WeddingRSVP{
		ID:         strings.TrimSpace(input.ID),
		FullName:   truncateWeddingText(strings.TrimSpace(input.FullName), weddingNameMaxLength),
		Attendance: strings.TrimSpace(input.Attendance),
		Drinks:     normalizeWeddingStringList(input.Drinks, weddingTextMaxLength),
		OtherDrink: truncateWeddingText(strings.TrimSpace(input.OtherDrink), weddingTextMaxLength),
		Song:       truncateWeddingText(strings.TrimSpace(input.Song), weddingTextMaxLength),
		CreatedAt:  input.CreatedAt,
	}
	if rsvp.FullName == "" {
		return model.WeddingRSVP{}, fmt.Errorf("full name is required")
	}
	if rsvp.Attendance != model.WeddingAttendanceAttending && rsvp.Attendance != model.WeddingAttendanceNotAttending {
		return model.WeddingRSVP{}, fmt.Errorf("attendance is invalid")
	}
	return rsvp, nil
}

func normalizeWeddingStringList(values []string, maxLength int) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		item := truncateWeddingText(strings.TrimSpace(value), maxLength)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

func truncateWeddingText(value string, maxLength int) string {
	if maxLength <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxLength {
		return value
	}
	return string(runes[:maxLength])
}
