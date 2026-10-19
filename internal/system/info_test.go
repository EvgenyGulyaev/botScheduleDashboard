package system

import (
	"encoding/json"
	"testing"
)

func TestCPUUsedPercent(t *testing.T) {
	tests := []struct {
		name     string
		previous string
		current  string
		want     float64
		valid    bool
	}{
		{"busy and idle", "cpu 100 0 50 800 50 0 0 0", "cpu 120 0 60 860 60 0 0 0", 30, true},
		{"all idle", "cpu 100 0 50 800 50 0 0 0", "cpu 100 0 50 900 50 0 0 0", 0, true},
		{"all busy", "cpu 100 0 50 800 50 0 0 0", "cpu 200 0 50 800 50 0 0 0", 100, true},
		{"guest time already included", "cpu 100 0 50 800 50 0 0 0 20 0", "cpu 120 0 60 860 60 0 0 0 40 0", 30, true},
		{"aggregate across cores", "cpu 100 0 50 800 50 0 0 0\ncpu0 0 0 0 0", "cpu 120 0 60 860 60 0 0 0\ncpu0 900 0 0 0", 30, true},
		{"unchanged counters", "cpu 100 0 50 800", "cpu 100 0 50 800", 0, false},
		{"counters reset", "cpu 100 0 50 800", "cpu 10 0 5 80", 0, false},
		{"idle counter decreased", "cpu 100 0 50 800", "cpu 300 0 50 790", 0, false},
		{"invalid idle delta", "cpu 100 0 50 800", "cpu 90 0 50 900", 0, false},
		{"missing stat", "", "cpu 100 0 50 800", 0, false},
		{"missing aggregate", "cpu0 100 0 50 800", "cpu 100 0 50 800", 0, false},
		{"malformed counter", "cpu 100 0 50 800", "cpu nope 0 50 800", 0, false},
		{"incomplete counters", "cpu 100 0 50 800", "cpu 100 0 50", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cpuUsedPercent(tt.previous, tt.current)
			if !tt.valid {
				if got != nil {
					t.Fatalf("expected unavailable usage, got %v", *got)
				}
				return
			}
			if got == nil || *got != tt.want {
				t.Fatalf("expected %v%% CPU usage, got %v", tt.want, got)
			}
		})
	}
}

func TestCPUInfoJSONIncludesUsage(t *testing.T) {
	usage := 30.0
	for _, value := range []*float64{nil, &usage} {
		data, err := json.Marshal(CPUInfo{UsedPercent: value})
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatal(err)
		}
		got, exists := payload["used_percent"]
		if !exists || (value == nil && got != nil) || (value != nil && got != *value) {
			t.Fatalf("unexpected CPU payload: %s", data)
		}
	}
}

func TestParseMemInfoCalculatesUsedMemory(t *testing.T) {
	info := parseMemInfo(`MemTotal:       1000000 kB
MemAvailable:    250000 kB
SwapTotal:       500000 kB
SwapFree:        400000 kB
`)

	if info.TotalBytes != 1024000000 {
		t.Fatalf("unexpected total memory: %#v", info)
	}
	if info.AvailableBytes != 256000000 || info.UsedBytes != 768000000 {
		t.Fatalf("unexpected used memory: %#v", info)
	}
	if info.UsedPercent != 75 {
		t.Fatalf("expected 75%% memory used, got %#v", info.UsedPercent)
	}
	if info.SwapUsedBytes != 102400000 {
		t.Fatalf("unexpected swap used: %#v", info)
	}
}

func TestParseLoadAverage(t *testing.T) {
	load := parseLoadAverage("0.15 0.30 0.45 1/100 12345")

	if load.One != 0.15 || load.Five != 0.30 || load.Fifteen != 0.45 {
		t.Fatalf("unexpected load average: %#v", load)
	}
}

func TestFormatBytes(t *testing.T) {
	if formatBytes(1024*1024*3) != "3.0M" {
		t.Fatalf("unexpected byte formatting")
	}
}

func TestBuildAlertsForHighResourceUsage(t *testing.T) {
	alerts := buildAlerts(Info{
		CPU:    CPUInfo{Cores: 2, Load: LoadAverage{One: 3.2}},
		Memory: MemoryInfo{UsedPercent: 92},
		Disk:   DiskInfo{UsedPercent: 84, Free: "3.0G"},
	})

	if len(alerts) != 3 {
		t.Fatalf("expected three alerts, got %#v", alerts)
	}
	if alerts[0].Level != "danger" || alerts[0].Metric != "memory" {
		t.Fatalf("expected memory danger first, got %#v", alerts[0])
	}
	if alerts[1].Level != "warning" || alerts[1].Metric != "disk" {
		t.Fatalf("expected disk warning second, got %#v", alerts[1])
	}
	if alerts[2].Metric != "cpu" {
		t.Fatalf("expected cpu alert third, got %#v", alerts[2])
	}
}
