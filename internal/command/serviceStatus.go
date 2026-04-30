package command

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type Status struct {
	ServiceName string
	LogLines    int
}

type StatusStats struct {
	Pid          string  `json:"pid"`
	Memory       string  `json:"memory"`
	MemoryBytes  uint64  `json:"memory_bytes"`
	CPU          string  `json:"cpu"`
	CPUSeconds   float64 `json:"cpu_seconds"`
	Uptime       string  `json:"uptime"`
	ActiveSince  string  `json:"active_since"`
	Restarts     string  `json:"restarts"`
	Tasks        string  `json:"tasks"`
	FragmentPath string  `json:"fragment_path"`
}

type StatusHealth struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

type StatusInfo struct {
	Service     string       `json:"service"`
	Raw         string       `json:"raw"`
	Status      string       `json:"status"`
	Loaded      string       `json:"loaded"`
	SubState    string       `json:"sub_state"`
	Description string       `json:"description"`
	Stats       StatusStats  `json:"Stats"`
	Health      StatusHealth `json:"health"`
	Logs        []string     `json:"logs"`
}

func (r *Status) Execute() string {
	cmd := exec.Command("sudo", "systemctl", "status", r.ServiceName, "--no-pager")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return err.Error()
	}
	return string(output)
}

func (r *Status) ExecuteShow() string {
	cmd := exec.Command("sudo", "systemctl", "show", r.ServiceName, "--no-pager",
		"--property=Description,LoadState,ActiveState,SubState,MainPID,NRestarts,TasksCurrent,MemoryCurrent,CPUUsageNSec,FragmentPath,ActiveEnterTimestamp")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return string(output)
}

func (r *Status) ExecuteJournal() string {
	cmd := exec.Command("sudo", "journalctl", "-u", r.ServiceName, "-n", strconv.Itoa(r.JournalLineLimit()), "--no-pager", "-o", "short-iso")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return string(output)
}

func (r *Status) JournalLineLimit() int {
	if r.LogLines <= 0 {
		return 8
	}
	if r.LogLines > 200 {
		return 200
	}
	return r.LogLines
}

func (r *Status) Details() StatusInfo {
	info := r.Info(r.Execute())
	info.ApplyShowProperties(r.ExecuteShow())
	info.ApplyJournal(r.ExecuteJournal())
	return info
}

func (r *Status) Info(text string) (res StatusInfo) {
	res.Service = r.ServiceName
	res.Raw = text

	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		switch {
		case strings.HasPrefix(line, "Active:"):
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				res.Status = parts[1]
			}
			if uptime := parseStatusUptime(line); uptime != "" {
				res.Stats.Uptime = uptime
			}

		case strings.HasPrefix(line, "Main PID:"):
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				res.Stats.Pid = parts[2]
			}

		case strings.HasPrefix(line, "Tasks:"):
			parts := strings.Fields(strings.TrimPrefix(line, "Tasks:"))
			if len(parts) >= 1 {
				res.Stats.Tasks = parts[0]
			}

		case strings.HasPrefix(line, "Memory:"):
			res.Stats.Memory = strings.TrimSpace(strings.TrimPrefix(line, "Memory:"))

		case strings.HasPrefix(line, "CPU:"):
			res.Stats.CPU = strings.TrimSpace(strings.TrimPrefix(line, "CPU:"))
		}
	}

	res.setHealth()
	return res
}

func (res *StatusInfo) ApplyShowProperties(text string) {
	props := parseShowProperties(text)
	res.Description = firstNonEmpty(props["Description"], res.Description)
	res.Loaded = firstNonEmpty(props["LoadState"], res.Loaded)
	res.SubState = firstNonEmpty(props["SubState"], res.SubState)
	res.Status = firstNonEmpty(props["ActiveState"], res.Status)
	res.Stats.Pid = firstNonEmpty(nonZeroValue(props["MainPID"]), res.Stats.Pid)
	res.Stats.Restarts = firstNonEmpty(props["NRestarts"], res.Stats.Restarts)
	res.Stats.Tasks = firstNonEmpty(nonZeroValue(props["TasksCurrent"]), res.Stats.Tasks)
	res.Stats.ActiveSince = firstNonEmpty(props["ActiveEnterTimestamp"], res.Stats.ActiveSince)
	res.Stats.FragmentPath = firstNonEmpty(props["FragmentPath"], res.Stats.FragmentPath)

	if memoryBytes, err := strconv.ParseUint(props["MemoryCurrent"], 10, 64); err == nil && memoryBytes > 0 {
		res.Stats.MemoryBytes = memoryBytes
		res.Stats.Memory = formatBytes(memoryBytes)
	}
	if cpuNSec, err := strconv.ParseUint(props["CPUUsageNSec"], 10, 64); err == nil && cpuNSec > 0 {
		res.Stats.CPUSeconds = float64(cpuNSec) / 1_000_000_000
		res.Stats.CPU = formatSeconds(res.Stats.CPUSeconds)
	}

	res.setHealth()
}

func (res *StatusInfo) ApplyJournal(text string) {
	scanner := bufio.NewScanner(strings.NewReader(text))
	lines := make([]string, 0, 8)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	res.Logs = lines
}

func parseShowProperties(text string) map[string]string {
	props := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := scanner.Text()
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		props[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return props
}

func parseStatusUptime(line string) string {
	_, uptime, ok := strings.Cut(line, ";")
	if !ok {
		return ""
	}
	return strings.TrimSpace(uptime)
}

func (res *StatusInfo) setHealth() {
	switch res.Status {
	case "active":
		if res.SubState == "" || res.SubState == "running" || res.SubState == "exited" {
			res.Health = StatusHealth{Level: "ok", Message: "Сервис работает"}
			return
		}
		res.Health = StatusHealth{Level: "warning", Message: "Сервис активен, но подстатус требует внимания"}
	case "failed":
		res.Health = StatusHealth{Level: "error", Message: "Сервис упал или завершился с ошибкой"}
	case "inactive", "deactivating":
		res.Health = StatusHealth{Level: "warning", Message: "Сервис не активен"}
	default:
		res.Health = StatusHealth{Level: "warning", Message: "Статус сервиса неизвестен"}
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func nonZeroValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "0" {
		return ""
	}
	return value
}

func formatBytes(value uint64) string {
	units := []string{"B", "K", "M", "G", "T"}
	size := float64(value)
	unit := 0
	for size >= 1024 && unit < len(units)-1 {
		size = size / 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d%s", value, units[unit])
	}
	return fmt.Sprintf("%.1f%s", size, units[unit])
}

func formatSeconds(value float64) string {
	if value < 60 {
		return fmt.Sprintf("%.3fs", value)
	}
	minutes := int(value / 60)
	seconds := value - float64(minutes*60)
	return fmt.Sprintf("%dm %.1fs", minutes, seconds)
}
