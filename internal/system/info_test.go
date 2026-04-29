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
