package store

import (
	"botDashboard/internal/model"
	"botDashboard/pkg/db"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.etcd.io/bbolt"
)

const (
	weddingNameMaxLength = 120
	weddingTextMaxLength = 160
	weddingCodeLength    = 6
	weddingIDFormat      = "%020d"
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

func (wr *WeddingRepository) DeleteRSVP(id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, fmt.Errorf("rsvp id is required")
	}

	deleted := false
	err := wr.repo.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(WeddingRSVPBucket)
		if b == nil {
			return fmt.Errorf("wedding rsvp bucket not found")
		}
		key := []byte(id)
		if b.Get(key) == nil {
			return nil
		}
		if err := b.Delete(key); err != nil {
			return err
		}
		deleted = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return deleted, nil
}

func (wr *WeddingRepository) UpdateRSVP(id string, input model.WeddingRSVP) (model.WeddingRSVP, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.WeddingRSVP{}, false, fmt.Errorf("rsvp id is required")
	}

	var updated model.WeddingRSVP
	found := false
	err := wr.repo.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(WeddingRSVPBucket)
		if b == nil {
			return fmt.Errorf("wedding rsvp bucket not found")
		}
		key := []byte(id)
		data := b.Get(key)
		if data == nil {
			return nil
		}

		var existing model.WeddingRSVP
		if err := json.Unmarshal(data, &existing); err != nil {
			return err
		}

		normalized, err := normalizeWeddingRSVP(input)
		if err != nil {
			return err
		}
		normalized.ID = existing.ID
		normalized.CreatedAt = existing.CreatedAt
		nextData, err := json.Marshal(normalized)
		if err != nil {
			return err
		}
		if err := b.Put(key, nextData); err != nil {
			return err
		}
		updated = normalized
		found = true
		return nil
	})
	if err != nil {
		return model.WeddingRSVP{}, false, err
	}
	return updated, found, nil
}

func (wr *WeddingRepository) GetSettings() (model.WeddingSettings, error) {
	settings := defaultWeddingSettings()
	err := wr.repo.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(WeddingSettingsBucket)
		if b == nil {
			return fmt.Errorf("wedding settings bucket not found")
		}
		data := b.Get([]byte(model.WeddingSettingsKey))
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &settings)
	})
	if err != nil {
		return model.WeddingSettings{}, err
	}
	return normalizeWeddingSettings(settings, defaultWeddingSettings(), false)
}

func (wr *WeddingRepository) SaveSettings(input model.WeddingSettings) (model.WeddingSettings, error) {
	var settings model.WeddingSettings
	err := wr.repo.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(WeddingSettingsBucket)
		if b == nil {
			return fmt.Errorf("wedding settings bucket not found")
		}
		previous := defaultWeddingSettings()
		if data := b.Get([]byte(model.WeddingSettingsKey)); data != nil {
			if err := json.Unmarshal(data, &previous); err != nil {
				return err
			}
			var err error
			previous, err = normalizeWeddingSettings(previous, defaultWeddingSettings(), false)
			if err != nil {
				return err
			}
		}

		var err error
		settings, err = normalizeWeddingSettings(input, previous, true)
		if err != nil {
			return err
		}
		data, err := json.Marshal(settings)
		if err != nil {
			return err
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
	if err := wr.repo.ClearBucket(WeddingSettingsBucket); err != nil {
		return err
	}
	return wr.repo.ClearBucket(WeddingAccessAttemptsBucket)
}

func (wr *WeddingRepository) VerifyAccessCode(ip string, code string, now time.Time) (model.WeddingAccessVerifyResult, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ip = strings.TrimSpace(ip)
	if ip == "" {
		ip = "unknown"
	}
	code = strings.TrimSpace(code)

	settings, err := wr.GetSettings()
	if err != nil {
		return model.WeddingAccessVerifyResult{}, err
	}
	if !settings.AccessCodeEnabled {
		return model.WeddingAccessVerifyResult{OK: true, AccessCodeVersion: settings.AccessCodeVersion}, nil
	}

	var result model.WeddingAccessVerifyResult
	err = wr.repo.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(WeddingAccessAttemptsBucket)
		if b == nil {
			return fmt.Errorf("wedding access attempts bucket not found")
		}

		var attempt model.WeddingAccessAttempt
		key := []byte(ip)
		if data := b.Get(key); data != nil {
			if err := json.Unmarshal(data, &attempt); err != nil {
				return err
			}
		}

		if attempt.BlockedUntil.After(now) {
			result = lockedWeddingAccessResult(attempt.BlockedUntil, now)
			return nil
		}
		if !attempt.BlockedUntil.IsZero() {
			attempt = model.WeddingAccessAttempt{}
			if err := b.Delete(key); err != nil {
				return err
			}
		}

		if code == settings.AccessCode {
			if err := b.Delete(key); err != nil {
				return err
			}
			result = model.WeddingAccessVerifyResult{
				OK:                true,
				AccessCodeVersion: settings.AccessCodeVersion,
			}
			return nil
		}

		attempt.IP = ip
		attempt.FailedAttempts++
		attempt.UpdatedAt = now
		if attempt.FailedAttempts >= model.WeddingAccessMaxAttempts {
			attempt.BlockedUntil = now.Add(model.WeddingAccessLockDuration)
			result = lockedWeddingAccessResult(attempt.BlockedUntil, now)
		} else {
			result = model.WeddingAccessVerifyResult{
				AttemptsLeft: model.WeddingAccessMaxAttempts - attempt.FailedAttempts,
				Message:      "invalid access code",
			}
		}

		data, err := json.Marshal(attempt)
		if err != nil {
			return err
		}
		return b.Put(key, data)
	})
	if err != nil {
		return model.WeddingAccessVerifyResult{}, err
	}
	return result, nil
}

func defaultWeddingSettings() model.WeddingSettings {
	return model.WeddingSettings{
		DrinkOptions:      model.DefaultWeddingDrinkOptions(),
		AccessCodeEnabled: false,
		AccessCode:        model.DefaultWeddingAccessCode,
		AccessCodeVersion: "1",
	}
}

func normalizeWeddingSettings(input model.WeddingSettings, previous model.WeddingSettings, saving bool) (model.WeddingSettings, error) {
	settings := model.WeddingSettings{
		DrinkOptions:      normalizeWeddingStringList(input.DrinkOptions, weddingTextMaxLength),
		AccessCodeEnabled: input.AccessCodeEnabled,
		AccessCode:        truncateWeddingText(strings.TrimSpace(input.AccessCode), weddingTextMaxLength),
		AccessCodeVersion: strings.TrimSpace(input.AccessCodeVersion),
	}
	if len(settings.DrinkOptions) == 0 {
		if saving {
			return model.WeddingSettings{}, fmt.Errorf("drink options are required")
		}
		settings.DrinkOptions = model.DefaultWeddingDrinkOptions()
	}

	if settings.AccessCode == "" {
		settings.AccessCode = previous.AccessCode
	}
	if settings.AccessCode == "" {
		settings.AccessCode = model.DefaultWeddingAccessCode
	}
	if settings.AccessCodeVersion == "" {
		settings.AccessCodeVersion = previous.AccessCodeVersion
	}
	if settings.AccessCodeVersion == "" {
		settings.AccessCodeVersion = "1"
	}
	if settings.AccessCodeEnabled && !isWeddingAccessCodeValid(settings.AccessCode) {
		return model.WeddingSettings{}, fmt.Errorf("access code must contain 6 digits")
	}

	if saving && (settings.AccessCodeEnabled != previous.AccessCodeEnabled || settings.AccessCode != previous.AccessCode) {
		settings.AccessCodeVersion = newWeddingAccessCodeVersion()
	}
	return settings, nil
}

func isWeddingAccessCodeValid(code string) bool {
	if len(code) != weddingCodeLength {
		return false
	}
	for _, char := range code {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func newWeddingAccessCodeVersion() string {
	return strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
}

func lockedWeddingAccessResult(blockedUntil time.Time, now time.Time) model.WeddingAccessVerifyResult {
	retryAfter := int(blockedUntil.Sub(now).Seconds())
	if retryAfter < 1 {
		retryAfter = 1
	}
	return model.WeddingAccessVerifyResult{
		Locked:            true,
		RetryAfterSeconds: retryAfter,
		Message:           "too many attempts",
	}
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
