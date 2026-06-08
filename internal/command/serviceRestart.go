package command

import (
	"os/exec"
)

type Restart struct {
	ServiceName string
}

func (r *Restart) Execute() string {
	cmd := exec.Command("sudo", "systemctl", "restart", ResolveServiceName(r.ServiceName))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return err.Error()
	}
	return string(output)
}

type Start struct {
	ServiceName string
}

func (s *Start) Execute() string {
	cmd := exec.Command("sudo", "systemctl", "start", ResolveServiceName(s.ServiceName))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return err.Error()
	}
	return string(output)
}

type Stop struct {
	ServiceName string
}

func (s *Stop) Execute() string {
	cmd := exec.Command("sudo", "systemctl", "stop", ResolveServiceName(s.ServiceName))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return err.Error()
	}
	return string(output)
}
