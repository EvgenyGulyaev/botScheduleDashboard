package store

import (
	"botDashboard/internal/model"
	"reflect"
	"testing"
	"time"
)

func TestWeddingRepositoryCreatesAndListsRSVPsNewestFirst(t *testing.T) {
	_ = newChatRepo(t)
	repo := GetWeddingRepository()
	if err := repo.ClearAll(); err != nil {
		t.Fatalf("clear wedding data: %v", err)
	}

	first, err := repo.CreateRSVP(model.WeddingRSVP{
		FullName:   "  Анна Иванова  ",
		Attendance: model.WeddingAttendanceAttending,
		Drinks:     []string{" Белое сухое ", "Белое сухое", ""},
		OtherDrink: "  Сидр  ",
		Song:       "  ABBA - Dancing Queen  ",
	})
	if err != nil {
		t.Fatalf("create first rsvp: %v", err)
	}
	if first.ID == "" {
		t.Fatal("expected id to be assigned")
	}
	if first.FullName != "Анна Иванова" {
		t.Fatalf("expected full name to be trimmed, got %q", first.FullName)
	}
	if !reflect.DeepEqual(first.Drinks, []string{"Белое сухое"}) {
		t.Fatalf("expected drinks to be normalized, got %#v", first.Drinks)
	}
	if first.OtherDrink != "Сидр" || first.Song != "ABBA - Dancing Queen" {
		t.Fatalf("expected text fields to be trimmed, got %#v", first)
	}
	if first.CreatedAt.IsZero() {
		t.Fatal("expected created_at to be set")
	}

	second, err := repo.CreateRSVP(model.WeddingRSVP{
		FullName:   "Петр Петров",
		Attendance: model.WeddingAttendanceNotAttending,
	})
	if err != nil {
		t.Fatalf("create second rsvp: %v", err)
	}

	items, err := repo.ListRSVPs()
	if err != nil {
		t.Fatalf("list rsvps: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 rsvps, got %d", len(items))
	}
	if items[0].ID != second.ID || items[1].ID != first.ID {
		t.Fatalf("expected newest rsvp first, got %#v", items)
	}

	if deleted, err := repo.DeleteRSVP(first.ID); err != nil || !deleted {
		t.Fatalf("delete first rsvp: deleted=%v err=%v", deleted, err)
	}
	items, err = repo.ListRSVPs()
	if err != nil {
		t.Fatalf("list rsvps after delete: %v", err)
	}
	if len(items) != 1 || items[0].ID != second.ID {
		t.Fatalf("expected only second rsvp after delete, got %#v", items)
	}
	if deleted, err := repo.DeleteRSVP(first.ID); err != nil || deleted {
		t.Fatalf("expected duplicate delete to report missing without error, deleted=%v err=%v", deleted, err)
	}
}

func TestWeddingRepositoryValidatesRequiredRSVPFields(t *testing.T) {
	_ = newChatRepo(t)
	repo := GetWeddingRepository()
	if err := repo.ClearAll(); err != nil {
		t.Fatalf("clear wedding data: %v", err)
	}

	if _, err := repo.CreateRSVP(model.WeddingRSVP{
		Attendance: model.WeddingAttendanceAttending,
	}); err == nil {
		t.Fatal("expected empty full name to fail")
	}

	if _, err := repo.CreateRSVP(model.WeddingRSVP{
		FullName:   "Анна Иванова",
		Attendance: "maybe",
	}); err == nil {
		t.Fatal("expected invalid attendance to fail")
	}
}

func TestWeddingRepositoryUpdatesRSVPFields(t *testing.T) {
	_ = newChatRepo(t)
	repo := GetWeddingRepository()
	if err := repo.ClearAll(); err != nil {
		t.Fatalf("clear wedding data: %v", err)
	}

	created, err := repo.CreateRSVP(model.WeddingRSVP{
		FullName:   "Анна Иванова",
		Attendance: model.WeddingAttendanceAttending,
		Drinks:     []string{"Белое сухое"},
		CreatedAt:  time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("create rsvp: %v", err)
	}

	updated, ok, err := repo.UpdateRSVP(created.ID, model.WeddingRSVP{
		FullName:   "  Анна и Андрей  ",
		Attendance: model.WeddingAttendanceNotAttending,
		Drinks:     []string{" Коньяк ", "Коньяк", "Другое"},
		OtherDrink: "  Сидр  ",
		Song:       "  Queen - Love of My Life  ",
	})
	if err != nil || !ok {
		t.Fatalf("update rsvp: ok=%v err=%v", ok, err)
	}
	if updated.ID != created.ID || !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("expected id and created_at to be preserved, got %#v", updated)
	}
	if updated.FullName != "Анна и Андрей" ||
		updated.Attendance != model.WeddingAttendanceNotAttending ||
		!reflect.DeepEqual(updated.Drinks, []string{"Коньяк", "Другое"}) ||
		updated.OtherDrink != "Сидр" ||
		updated.Song != "Queen - Love of My Life" {
		t.Fatalf("expected normalized updated rsvp, got %#v", updated)
	}

	items, err := repo.ListRSVPs()
	if err != nil {
		t.Fatalf("list rsvps: %v", err)
	}
	if len(items) != 1 || items[0].FullName != updated.FullName {
		t.Fatalf("expected updated item in list, got %#v", items)
	}

	if _, ok, err := repo.UpdateRSVP(created.ID+"-missing", updated); err != nil || ok {
		t.Fatalf("expected missing update to report false without error, ok=%v err=%v", ok, err)
	}
}

func TestWeddingRepositorySettingsDefaultAndSave(t *testing.T) {
	_ = newChatRepo(t)
	repo := GetWeddingRepository()
	if err := repo.ClearAll(); err != nil {
		t.Fatalf("clear wedding data: %v", err)
	}

	settings, err := repo.GetSettings()
	if err != nil {
		t.Fatalf("get default settings: %v", err)
	}
	if !reflect.DeepEqual(settings.DrinkOptions, model.DefaultWeddingDrinkOptions()) {
		t.Fatalf("expected default drink options, got %#v", settings.DrinkOptions)
	}
	if settings.AccessCodeEnabled || settings.AccessCode != model.DefaultWeddingAccessCode || settings.AccessCodeVersion == "" {
		t.Fatalf("expected default access code settings, got %#v", settings)
	}

	saved, err := repo.SaveSettings(model.WeddingSettings{
		DrinkOptions:      []string{"  Вино  ", "", "Вино", "Сидр"},
		AccessCodeEnabled: true,
		AccessCode:        "171026",
	})
	if err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if !reflect.DeepEqual(saved.DrinkOptions, []string{"Вино", "Сидр"}) {
		t.Fatalf("expected normalized drink options, got %#v", saved.DrinkOptions)
	}
	if !saved.AccessCodeEnabled || saved.AccessCode != "171026" || saved.AccessCodeVersion == "" {
		t.Fatalf("expected access code settings to be normalized, got %#v", saved)
	}

	loaded, err := repo.GetSettings()
	if err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	if !reflect.DeepEqual(loaded.DrinkOptions, saved.DrinkOptions) {
		t.Fatalf("expected saved settings to persist, got %#v", loaded)
	}
	if loaded.AccessCode != saved.AccessCode || loaded.AccessCodeVersion != saved.AccessCodeVersion {
		t.Fatalf("expected access code settings to persist, got %#v", loaded)
	}

	if _, err := repo.SaveSettings(model.WeddingSettings{}); err == nil {
		t.Fatal("expected empty drink options to fail")
	}
	if _, err := repo.SaveSettings(model.WeddingSettings{
		DrinkOptions:      model.DefaultWeddingDrinkOptions(),
		AccessCodeEnabled: true,
		AccessCode:        "123",
	}); err == nil {
		t.Fatal("expected invalid access code to fail")
	}
}

func TestWeddingRepositoryVerifiesAccessCodeAndExpiresAttempts(t *testing.T) {
	_ = newChatRepo(t)
	repo := GetWeddingRepository()
	if err := repo.ClearAll(); err != nil {
		t.Fatalf("clear wedding data: %v", err)
	}
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	settings, err := repo.SaveSettings(model.WeddingSettings{
		DrinkOptions:      model.DefaultWeddingDrinkOptions(),
		AccessCodeEnabled: true,
		AccessCode:        "171026",
	})
	if err != nil {
		t.Fatalf("save settings: %v", err)
	}

	for i := 1; i <= 2; i++ {
		result, err := repo.VerifyAccessCode("198.51.100.7", "000000", now)
		if err != nil {
			t.Fatalf("verify wrong code attempt %d: %v", i, err)
		}
		if result.OK || result.Locked || result.RetryAfterSeconds != 0 {
			t.Fatalf("expected wrong code without lock on attempt %d, got %#v", i, result)
		}
	}

	result, err := repo.VerifyAccessCode("198.51.100.7", "000000", now)
	if err != nil {
		t.Fatalf("verify third wrong code: %v", err)
	}
	if !result.Locked || result.RetryAfterSeconds <= 0 {
		t.Fatalf("expected lock on third wrong code, got %#v", result)
	}

	result, err = repo.VerifyAccessCode("198.51.100.7", "171026", now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("verify locked correct code: %v", err)
	}
	if !result.Locked || result.OK {
		t.Fatalf("expected locked correct code to stay blocked, got %#v", result)
	}

	result, err = repo.VerifyAccessCode("198.51.100.7", "171026", now.Add(11*time.Minute))
	if err != nil {
		t.Fatalf("verify after lock expiry: %v", err)
	}
	if !result.OK || result.Locked || result.AccessCodeVersion != settings.AccessCodeVersion {
		t.Fatalf("expected success after expiry, got %#v", result)
	}
}
