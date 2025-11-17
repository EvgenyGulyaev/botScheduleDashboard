package command

import (
	"os/exec"
)

type Restart struct {
	ServiceName string
}

func (r *Restart) Execute() string {
	cmd := exec.Command("sudo", "systemctl", "restart", r.ServiceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return err.Error()
	}
	return string(output)
}
