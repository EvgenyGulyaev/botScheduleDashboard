package system

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Info struct {
	Hostname string     `json:"hostname"`
	OS       string     `json:"os"`
	Arch     string     `json:"arch"`
	CPU      CPUInfo    `json:"cpu"`
	Memory   MemoryInfo `json:"memory"`
	Disk     DiskInfo   `json:"disk"`
	Uptime   UptimeInfo `json:"uptime"`
}

type CPUInfo struct {
	Cores int         `json:"cores"`
	Load  LoadAverage `json:"load"`
}

type LoadAverage struct {
	One     float64 `json:"one"`
	Five    float64 `json:"five"`
	Fifteen float64 `json:"fifteen"`
}

type MemoryInfo struct {
	TotalBytes     uint64  `json:"total_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	UsedPercent    float64 `json:"used_percent"`
	SwapTotalBytes uint64  `json:"swap_total_bytes"`
	SwapUsedBytes  uint64  `json:"swap_used_bytes"`
	Total          string  `json:"total"`
	Available      string  `json:"available"`
	Used           string  `json:"used"`
	SwapUsed       string  `json:"swap_used"`
}

type DiskInfo struct {
	Path        string  `json:"path"`
	TotalBytes  uint64  `json:"total_bytes"`
	FreeBytes   uint64  `json:"free_bytes"`
	UsedBytes   uint64  `json:"used_bytes"`
	UsedPercent float64 `json:"used_percent"`
	Total       string  `json:"total"`
	Free        string  `json:"free"`
	Used        string  `json:"used"`
}

type UptimeInfo struct {
	Seconds uint64 `json:"seconds"`
	Human   string `json:"human"`
}

func CollectInfo() Info {
	hostname, _ := os.Hostname()
	return Info{
		Hostname: hostname,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		CPU: CPUInfo{
			Cores: runtime.NumCPU(),
			Load:  readLoadAverage(),
		},
		Memory: readMemoryInfo(),
		Disk:   readDiskInfo("/"),
		Uptime: readUptimeInfo(),
	}
}

func readMemoryInfo() MemoryInfo {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return MemoryInfo{}
	}
	return parseMemInfo(string(data))
}

func parseMemInfo(text string) MemoryInfo {
	values := map[string]uint64{}
	for _, line := range strings.Split(text, "\n") {
		key, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		value, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		values[key] = value * 1024
	}

	total := values["MemTotal"]
	available := values["MemAvailable"]
	if available == 0 {
		available = values["MemFree"]
	}
	swapTotal := values["SwapTotal"]
	swapFree := values["SwapFree"]
	used := uint64(0)
	if total > available {
		used = total - available
	}
	swapUsed := uint64(0)
	if swapTotal > swapFree {
		swapUsed = swapTotal - swapFree
	}

	info := MemoryInfo{
		TotalBytes:     total,
		AvailableBytes: available,
		UsedBytes:      used,
		UsedPercent:    percent(used, total),
		SwapTotalBytes: swapTotal,
		SwapUsedBytes:  swapUsed,
	}
	info.Total = formatBytes(info.TotalBytes)
	info.Available = formatBytes(info.AvailableBytes)
	info.Used = formatBytes(info.UsedBytes)
	info.SwapUsed = formatBytes(info.SwapUsedBytes)
	return info
}

func readLoadAverage() LoadAverage {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return LoadAverage{}
	}
	return parseLoadAverage(string(data))
}

func parseLoadAverage(text string) LoadAverage {
	fields := strings.Fields(text)
	parse := func(index int) float64 {
		if len(fields) <= index {
			return 0
		}
		value, _ := strconv.ParseFloat(fields[index], 64)
		return value
	}
	return LoadAverage{
		One:     parse(0),
		Five:    parse(1),
		Fifteen: parse(2),
	}
}

func readDiskInfo(path string) DiskInfo {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return DiskInfo{Path: path}
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	used := uint64(0)
	if total > free {
		used = total - free
	}
	return DiskInfo{
		Path:        path,
		TotalBytes:  total,
		FreeBytes:   free,
		UsedBytes:   used,
		UsedPercent: percent(used, total),
		Total:       formatBytes(total),
		Free:        formatBytes(free),
		Used:        formatBytes(used),
	}
}

func readUptimeInfo() UptimeInfo {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return UptimeInfo{}
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return UptimeInfo{}
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return UptimeInfo{}
	}
	seconds := uint64(value)
	return UptimeInfo{
		Seconds: seconds,
		Human:   formatDuration(seconds),
	}
}

func percent(value, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(value) / float64(total) * 100
}

func formatBytes(value uint64) string {
	units := []string{"B", "K", "M", "G", "T"}
	size := float64(value)
	unit := 0
	for size >= 1024 && unit < len(units)-1 {
		size /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d%s", value, units[unit])
	}
	return fmt.Sprintf("%.1f%s", size, units[unit])
}

func formatDuration(seconds uint64) string {
	duration := time.Duration(seconds) * time.Second
	days := duration / (24 * time.Hour)
	duration -= days * 24 * time.Hour
	hours := duration / time.Hour
	duration -= hours * time.Hour
	minutes := duration / time.Minute
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}
