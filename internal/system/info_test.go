package system

import "testing"

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
