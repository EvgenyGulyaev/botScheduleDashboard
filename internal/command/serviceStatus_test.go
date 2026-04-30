package command

import "testing"

func TestStatusInfoParsesDetailedSystemctlOutput(t *testing.T) {
	status := (&Status{ServiceName: "dashboard"}).Info(`● dashboard.service - Dashboard
     Loaded: loaded (/etc/systemd/system/dashboard.service; enabled; preset: enabled)
     Active: active (running) since Wed 2026-04-29 10:00:00 MSK; 2h 15min ago
   Main PID: 213066 (app)
      Tasks: 8 (limit: 9378)
     Memory: 9.8M (peak: 12.8M)
        CPU: 44.812s
`)

	status.ApplyShowProperties(`Description=Dashboard
LoadState=loaded
ActiveState=active
SubState=running
MainPID=213066
NRestarts=2
TasksCurrent=8
MemoryCurrent=10276044
CPUUsageNSec=44812000000
ActiveEnterTimestamp=Wed 2026-04-29 10:00:00 MSK
`)
	status.ApplyJournal(`Apr 29 10:01:00 host app[213066]: started
Apr 29 10:02:00 host app[213066]: ready
`)

	if status.Status != "active" || status.SubState != "running" || status.Loaded != "loaded" {
		t.Fatalf("unexpected state fields: %#v", status)
	}
	if status.Description != "Dashboard" {
		t.Fatalf("expected description from systemctl show, got %#v", status.Description)
	}
	if status.Stats.Pid != "213066" || status.Stats.Tasks != "8" || status.Stats.Restarts != "2" {
		t.Fatalf("unexpected process stats: %#v", status.Stats)
	}
	if status.Stats.MemoryBytes != 10276044 || status.Stats.Memory == "" {
		t.Fatalf("expected memory stats, got %#v", status.Stats)
	}
	if status.Stats.CPUSeconds != 44.812 || status.Stats.CPU == "" {
		t.Fatalf("expected cpu stats, got %#v", status.Stats)
	}
	if status.Stats.Uptime != "2h 15min ago" || status.Stats.ActiveSince != "Wed 2026-04-29 10:00:00 MSK" {
		t.Fatalf("expected uptime and active timestamp, got %#v", status.Stats)
	}
	if status.Health.Level != "ok" || status.Health.Message == "" {
		t.Fatalf("expected ok health, got %#v", status.Health)
	}
	if len(status.Logs) != 2 || status.Logs[1] == "" {
		t.Fatalf("expected journal lines, got %#v", status.Logs)
	}
}

func TestStatusInfoMarksFailedServicesAsError(t *testing.T) {
	status := (&Status{ServiceName: "bot"}).Info(`Active: failed (Result: exit-code) since Wed 2026-04-29 10:00:00 MSK; 1min ago`)
	status.ApplyShowProperties(`LoadState=loaded
ActiveState=failed
SubState=failed
NRestarts=5
`)

	if status.Status != "failed" {
		t.Fatalf("expected failed status, got %#v", status.Status)
	}
	if status.Health.Level != "error" {
		t.Fatalf("expected error health, got %#v", status.Health)
	}
}

func TestStatusJournalLineLimitDefaultsAndClamps(t *testing.T) {
	cases := []struct {
		name  string
		lines int
		want  int
	}{
		{name: "default", lines: 0, want: 8},
		{name: "negative", lines: -10, want: 8},
		{name: "custom", lines: 30, want: 30},
		{name: "too high", lines: 1000, want: 200},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status := &Status{ServiceName: "dashboard", LogLines: tc.lines}
			if got := status.JournalLineLimit(); got != tc.want {
				t.Fatalf("expected %d journal lines, got %d", tc.want, got)
			}
		})
	}
}
