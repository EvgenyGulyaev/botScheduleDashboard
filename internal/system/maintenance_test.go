package system

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectMaintenancePlanCountsSafeCleanupTargets(t *testing.T) {
	root := t.TempDir()
	aptCache := filepath.Join(root, "apt")
	tmpDir := filepath.Join(root, "tmp")
	chatDir := filepath.Join(root, "chat")
	for _, dir := range []string{aptCache, tmpDir, chatDir} {
		if err := os.MkdirAll(dir, 0750); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	writeSizedFile(t, filepath.Join(aptCache, "pkg.deb"), 10)
	oldTmp := filepath.Join(tmpDir, "old.tmp")
	newTmp := filepath.Join(tmpDir, "new.tmp")
	oldChat := filepath.Join(chatDir, "old.webm")
	writeSizedFile(t, oldTmp, 20)
	writeSizedFile(t, newTmp, 40)
	writeSizedFile(t, oldChat, 30)

	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(oldTmp, now.Add(-48*time.Hour), now.Add(-48*time.Hour)); err != nil {
		t.Fatalf("chtimes old tmp: %v", err)
	}
	if err := os.Chtimes(newTmp, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatalf("chtimes new tmp: %v", err)
	}
	if err := os.Chtimes(oldChat, now.Add(-48*time.Hour), now.Add(-48*time.Hour)); err != nil {
		t.Fatalf("chtimes old chat: %v", err)
	}

	plan := CollectMaintenancePlan(MaintenanceOptions{
		AptCachePath:     aptCache,
		TempPath:         tmpDir,
		TempMaxAge:       24 * time.Hour,
		ChatMediaPaths:   []string{chatDir},
		ChatMediaMaxAge:  25 * time.Hour,
		Now:              now,
		EnableAptCleanup: false,
	})

	if plan.TotalReclaimableBytes != 50 {
		t.Fatalf("expected 50 bytes reclaimable, got %#v", plan)
	}
	if item := plan.Item("apt_cache"); item.ReclaimableBytes != 10 || item.Enabled {
		t.Fatalf("expected disabled apt cache item with 10 bytes, got %#v", item)
	}
	if item := plan.Item("tmp_old"); item.ReclaimableBytes != 20 || !item.Enabled {
		t.Fatalf("expected old tmp item with 20 bytes, got %#v", item)
	}
	if item := plan.Item("chat_media_old"); item.ReclaimableBytes != 30 || !item.Enabled {
		t.Fatalf("expected old chat item with 30 bytes, got %#v", item)
	}
}

func TestRunMaintenanceCleanupRemovesOnlySelectedOldFiles(t *testing.T) {
	root := t.TempDir()
	tmpDir := filepath.Join(root, "tmp")
	chatDir := filepath.Join(root, "chat")
	for _, dir := range []string{tmpDir, chatDir} {
		if err := os.MkdirAll(dir, 0750); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	oldTmp := filepath.Join(tmpDir, "old.tmp")
	newTmp := filepath.Join(tmpDir, "new.tmp")
	oldChat := filepath.Join(chatDir, "old.webm")
	writeSizedFile(t, oldTmp, 20)
	writeSizedFile(t, newTmp, 40)
	writeSizedFile(t, oldChat, 30)

	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	for _, path := range []string{oldTmp, oldChat} {
		if err := os.Chtimes(path, now.Add(-48*time.Hour), now.Add(-48*time.Hour)); err != nil {
			t.Fatalf("chtimes old file: %v", err)
		}
	}
	if err := os.Chtimes(newTmp, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatalf("chtimes new tmp: %v", err)
	}

	result, err := RunMaintenanceCleanup([]string{"tmp_old"}, MaintenanceOptions{
		TempPath:        tmpDir,
		TempMaxAge:      24 * time.Hour,
		ChatMediaPaths:  []string{chatDir},
		ChatMediaMaxAge: 25 * time.Hour,
		Now:             now,
	})
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if result.CleanedBytes != 20 {
		t.Fatalf("expected 20 cleaned bytes, got %#v", result)
	}
	if _, err := os.Stat(oldTmp); !os.IsNotExist(err) {
		t.Fatalf("expected old tmp removed, stat err: %v", err)
	}
	if _, err := os.Stat(newTmp); err != nil {
		t.Fatalf("expected new tmp to stay, stat err: %v", err)
	}
	if _, err := os.Stat(oldChat); err != nil {
		t.Fatalf("expected unselected chat file to stay, stat err: %v", err)
	}
}

func writeSizedFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
