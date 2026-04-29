package store

import (
	"botDashboard/internal/model"
	"testing"
	"time"
)

func TestAuditRepositoryKeepsOnlyLastTwentyEntries(t *testing.T) {
	_ = newChatRepo(t)
	repo := GetAuditRepository()
	if err := repo.ClearAll(); err != nil {
		t.Fatalf("clear audit: %v", err)
	}

	for i := 0; i < 25; i++ {
		if _, err := repo.Append(model.AuditEntry{
			ActorEmail: "evgeny@example.com",
			ActorLogin: "evgeny",
			Action:     "admin.user.update",
			Target:     "user@example.com",
			Summary:    "updated user",
		}); err != nil {
			t.Fatalf("append audit entry %d: %v", i, err)
		}
	}

	items, err := repo.ListRecent()
	if err != nil {
		t.Fatalf("list audit entries: %v", err)
	}
	if len(items) != 20 {
		t.Fatalf("expected 20 audit entries, got %d", len(items))
	}
	if items[0].ID <= items[len(items)-1].ID {
		t.Fatalf("expected newest entries first, got first=%s last=%s", items[0].ID, items[len(items)-1].ID)
	}
}

func TestAuditRepositoryDropsEntriesOlderThanThreeDays(t *testing.T) {
	_ = newChatRepo(t)
	repo := GetAuditRepository()
	if err := repo.ClearAll(); err != nil {
		t.Fatalf("clear audit: %v", err)
	}

	oldEntry, err := repo.Append(model.AuditEntry{
		ActorEmail: "evgeny@example.com",
		ActorLogin: "evgeny",
		Action:     "admin.user.update",
		Target:     "old@example.com",
		Summary:    "old entry",
		CreatedAt:  time.Now().UTC().Add(-4 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("append old entry: %v", err)
	}
	newEntry, err := repo.Append(model.AuditEntry{
		ActorEmail: "evgeny@example.com",
		ActorLogin: "evgeny",
		Action:     "admin.user.update",
		Target:     "fresh@example.com",
		Summary:    "fresh entry",
		CreatedAt:  time.Now().UTC().Add(-2 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("append fresh entry: %v", err)
	}

	items, err := repo.ListRecent()
	if err != nil {
		t.Fatalf("list audit entries: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one fresh entry after ttl cleanup, got %#v", items)
	}
	if items[0].ID == oldEntry.ID || items[0].ID != newEntry.ID {
		t.Fatalf("expected only fresh entry to remain, got %#v", items)
	}
}
