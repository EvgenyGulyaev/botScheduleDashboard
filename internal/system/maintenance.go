package system

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type MaintenanceOptions struct {
	AptCachePath     string
	TempPath         string
	TempMaxAge       time.Duration
	ChatMediaPaths   []string
	ChatMediaMaxAge  time.Duration
	Now              time.Time
	EnableAptCleanup bool
}

type MaintenancePlan struct {
	GeneratedAt            time.Time         `json:"generated_at"`
	Items                  []MaintenanceItem `json:"items"`
	TotalReclaimableBytes  uint64            `json:"total_reclaimable_bytes"`
	TotalReclaimable       string            `json:"total_reclaimable"`
	SelectedReclaimable    string            `json:"selected_reclaimable,omitempty"`
	SelectedReclaimableB   uint64            `json:"selected_reclaimable_bytes,omitempty"`
	CleanedBytes           uint64            `json:"cleaned_bytes,omitempty"`
	Cleaned                string            `json:"cleaned,omitempty"`
	SelectedMaintenanceIDs []string          `json:"selected_items,omitempty"`
}

type MaintenanceItem struct {
	Key              string `json:"key"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	Path             string `json:"path,omitempty"`
	Enabled          bool   `json:"enabled"`
	ReclaimableBytes uint64 `json:"reclaimable_bytes"`
	Reclaimable      string `json:"reclaimable"`
	Reason           string `json:"reason,omitempty"`
}

func DefaultMaintenanceOptions() MaintenanceOptions {
	return MaintenanceOptions{
		AptCachePath:     "/var/cache/apt",
		TempPath:         os.TempDir(),
		TempMaxAge:       24 * time.Hour,
		ChatMediaMaxAge:  25 * time.Hour,
		EnableAptCleanup: true,
	}
}

func CollectMaintenancePlan(options MaintenanceOptions) MaintenancePlan {
	options = normalizeMaintenanceOptions(options)
	items := []MaintenanceItem{
		buildMaintenanceItem(
			"apt_cache",
			"APT cache",
			"Кэш установочных пакетов Linux. Безопасно чистится через apt-get clean.",
			options.AptCachePath,
			options.EnableAptCleanup,
			directorySize(options.AptCachePath),
			disabledReason(options.EnableAptCleanup, "APT cleanup отключён для этого окружения"),
		),
		buildMaintenanceItem(
			"tmp_old",
			"Старые временные файлы",
			"Файлы и папки во временной директории старше заданного порога.",
			options.TempPath,
			safeCleanupRoot(options.TempPath),
			oldRootEntriesSize(options.TempPath, options.Now.Add(-options.TempMaxAge)),
			unsafeRootReason(options.TempPath),
		),
		buildMaintenanceItem(
			"chat_media_old",
			"Старые медиа чата",
			"Аудио, изображения и файлы чата старше TTL. Перед очисткой сообщения помечаются истёкшими.",
			strings.Join(options.ChatMediaPaths, ", "),
			len(options.ChatMediaPaths) > 0 && safeCleanupRoots(options.ChatMediaPaths),
			oldRootEntriesSizeMany(options.ChatMediaPaths, options.Now.Add(-options.ChatMediaMaxAge)),
			unsafeRootsReason(options.ChatMediaPaths),
		),
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Key < items[j].Key
	})

	plan := MaintenancePlan{
		GeneratedAt: options.Now,
		Items:       items,
	}
	for _, item := range items {
		if !item.Enabled {
			continue
		}
		plan.TotalReclaimableBytes += item.ReclaimableBytes
	}
	plan.TotalReclaimable = formatBytes(plan.TotalReclaimableBytes)
	return plan
}

func RunMaintenanceCleanup(items []string, options MaintenanceOptions) (MaintenancePlan, error) {
	options = normalizeMaintenanceOptions(options)
	selected := selectedMaintenanceItems(items)
	var cleaned uint64

	if selected["apt_cache"] && options.EnableAptCleanup && options.AptCachePath != "" {
		before := directorySize(options.AptCachePath)
		if err := exec.Command("apt-get", "clean").Run(); err != nil {
			return MaintenancePlan{}, err
		}
		after := directorySize(options.AptCachePath)
		if before > after {
			cleaned += before - after
		}
	}
	if selected["tmp_old"] && safeCleanupRoot(options.TempPath) {
		removed, err := removeOldRootEntries(options.TempPath, options.Now.Add(-options.TempMaxAge))
		if err != nil {
			return MaintenancePlan{}, err
		}
		cleaned += removed
	}
	if selected["chat_media_old"] && safeCleanupRoots(options.ChatMediaPaths) {
		removed, err := removeOldRootEntriesMany(options.ChatMediaPaths, options.Now.Add(-options.ChatMediaMaxAge))
		if err != nil {
			return MaintenancePlan{}, err
		}
		cleaned += removed
	}

	plan := CollectMaintenancePlan(options)
	plan.CleanedBytes = cleaned
	plan.Cleaned = formatBytes(cleaned)
	plan.SelectedMaintenanceIDs = orderedSelectedItems(selected)
	return plan, nil
}

func (plan MaintenancePlan) Item(key string) MaintenanceItem {
	for _, item := range plan.Items {
		if item.Key == key {
			return item
		}
	}
	return MaintenanceItem{}
}

func normalizeMaintenanceOptions(options MaintenanceOptions) MaintenanceOptions {
	if options.TempMaxAge <= 0 {
		options.TempMaxAge = 24 * time.Hour
	}
	if options.ChatMediaMaxAge <= 0 {
		options.ChatMediaMaxAge = 25 * time.Hour
	}
	if options.Now.IsZero() {
		options.Now = time.Now().UTC()
	}
	return options
}

func buildMaintenanceItem(key, title, description, path string, enabled bool, reclaimableBytes uint64, reason string) MaintenanceItem {
	item := MaintenanceItem{
		Key:              key,
		Title:            title,
		Description:      description,
		Path:             path,
		Enabled:          enabled,
		ReclaimableBytes: reclaimableBytes,
		Reason:           reason,
	}
	item.Reclaimable = formatBytes(reclaimableBytes)
	return item
}

func directorySize(path string) uint64 {
	if strings.TrimSpace(path) == "" {
		return 0
	}
	var total uint64
	_ = filepath.WalkDir(path, func(itemPath string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Size() <= 0 {
			return nil
		}
		total += uint64(info.Size())
		_ = itemPath
		return nil
	})
	return total
}

func oldRootEntriesSize(path string, cutoff time.Time) uint64 {
	entries, err := oldRootEntries(path, cutoff)
	if err != nil {
		return 0
	}
	var total uint64
	for _, entry := range entries {
		total += directorySize(entry)
	}
	return total
}

func oldRootEntriesSizeMany(paths []string, cutoff time.Time) uint64 {
	var total uint64
	for _, path := range paths {
		total += oldRootEntriesSize(path, cutoff)
	}
	return total
}

func oldRootEntries(path string, cutoff time.Time) ([]string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, nil
	}
	result := make([]string, 0)
	for _, entry := range entries {
		entryPath := filepath.Join(path, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		result = append(result, entryPath)
	}
	return result, nil
}

func removeOldRootEntries(path string, cutoff time.Time) (uint64, error) {
	entries, err := oldRootEntries(path, cutoff)
	if err != nil {
		return 0, err
	}
	var removed uint64
	for _, entry := range entries {
		size := directorySize(entry)
		if err := os.RemoveAll(entry); err != nil {
			return removed, err
		}
		removed += size
	}
	return removed, nil
}

func removeOldRootEntriesMany(paths []string, cutoff time.Time) (uint64, error) {
	var total uint64
	for _, path := range paths {
		removed, err := removeOldRootEntries(path, cutoff)
		if err != nil {
			return total, err
		}
		total += removed
	}
	return total, nil
}

func selectedMaintenanceItems(items []string) map[string]bool {
	selected := make(map[string]bool, len(items))
	for _, item := range items {
		key := strings.TrimSpace(item)
		if key == "" {
			continue
		}
		selected[key] = true
	}
	return selected
}

func orderedSelectedItems(selected map[string]bool) []string {
	items := make([]string, 0, len(selected))
	for key := range selected {
		items = append(items, key)
	}
	sort.Strings(items)
	return items
}

func safeCleanupRoots(paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	for _, path := range paths {
		if !safeCleanupRoot(path) {
			return false
		}
	}
	return true
}

func safeCleanupRoot(path string) bool {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if cleaned == "." || cleaned == string(filepath.Separator) || cleaned == "" {
		return false
	}
	if !filepath.IsAbs(cleaned) {
		absolute, err := filepath.Abs(cleaned)
		if err != nil {
			return false
		}
		cleaned = absolute
	}
	if cleaned == filepath.Clean(os.TempDir()) || cleaned == filepath.Clean("/tmp") || cleaned == filepath.Clean("/var/tmp") {
		return true
	}
	volume := filepath.VolumeName(cleaned)
	withoutVolume := strings.TrimPrefix(cleaned, volume)
	parts := strings.Split(strings.Trim(withoutVolume, string(filepath.Separator)), string(filepath.Separator))
	return len(parts) >= 2
}

func disabledReason(enabled bool, reason string) string {
	if enabled {
		return ""
	}
	return reason
}

func unsafeRootReason(path string) string {
	if safeCleanupRoot(path) {
		return ""
	}
	return "Путь слишком общий, очистка отключена"
}

func unsafeRootsReason(paths []string) string {
	if len(paths) == 0 {
		return "Пути медиа чата не настроены"
	}
	if safeCleanupRoots(paths) {
		return ""
	}
	return "Один из путей слишком общий, очистка отключена"
}
