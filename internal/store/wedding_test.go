package store

import (
	"botDashboard/internal/model"
	"reflect"
	"testing"
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

	saved, err := repo.SaveSettings(model.WeddingSettings{
		DrinkOptions: []string{"  Вино  ", "", "Вино", "Сидр"},
	})
	if err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if !reflect.DeepEqual(saved.DrinkOptions, []string{"Вино", "Сидр"}) {
		t.Fatalf("expected normalized drink options, got %#v", saved.DrinkOptions)
	}

	loaded, err := repo.GetSettings()
	if err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	if !reflect.DeepEqual(loaded.DrinkOptions, saved.DrinkOptions) {
		t.Fatalf("expected saved settings to persist, got %#v", loaded)
	}

	if _, err := repo.SaveSettings(model.WeddingSettings{}); err == nil {
		t.Fatal("expected empty drink options to fail")
	}
}
