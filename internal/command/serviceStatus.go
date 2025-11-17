package command

import (
	"os/exec"
)

type Status struct {
	ServiceName string
}

func (r *Status) Execute() string {
	cmd := exec.Command("sudo", "systemctl", "status", r.ServiceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return err.Error()
	}
	return string(output)
}
