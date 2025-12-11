package command

import (
	"bufio"
	"os/exec"
	"strings"
)

type Status struct {
	ServiceName string
}

type StatusInfo struct {
	Service string `json:"service"`
	Raw     string `json:"raw"`
	Status  string `json:"status"`
	Stats   struct {
		Pid    string `json:"pid"`
		Memory string `json:"memory"`
		CPU    string `json:"cpu"`
	}
}

func (r *Status) Execute() string {
	cmd := exec.Command("sudo", "systemctl", "status", r.ServiceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return err.Error()
	}
	return string(output)
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

		case strings.HasPrefix(line, "Main PID:"):
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				res.Stats.Pid = parts[2]
			}

		case strings.HasPrefix(line, "Memory:"):
			res.Stats.Memory = strings.TrimSpace(strings.TrimPrefix(line, "Memory:"))

		case strings.HasPrefix(line, "CPU:"):
			res.Stats.CPU = strings.TrimSpace(strings.TrimPrefix(line, "CPU:"))
		}
	}

	return res
}
