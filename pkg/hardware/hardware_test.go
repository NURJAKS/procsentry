package hardware

import (
	"testing"
)

func TestGetSystemTelemetry(t *testing.T) {
	telemetry := GetSystemTelemetry()

	if telemetry.Status == "" {
		t.Error("expected telemetry Status to be non-empty")
	}

	if telemetry.CPU.CoreCount < 0 {
		t.Errorf("invalid CPU core count: %d", telemetry.CPU.CoreCount)
	}

	if telemetry.RAM.TotalGB < 0 {
		t.Errorf("invalid RAM total: %f", telemetry.RAM.TotalGB)
	}

	if telemetry.Status != "NOMINAL" && telemetry.Status != "WARNING" && telemetry.Status != "CRITICAL" {
		t.Errorf("unexpected telemetry status: %s", telemetry.Status)
	}
}

func TestCPUTicksTotal(t *testing.T) {
	ticks := cpuTicks{
		user:    100,
		nice:    20,
		sys:     30,
		idle:    500,
		iowait:  10,
		irq:     5,
		softirq: 5,
		steal:   0,
	}
	expected := uint64(670)
	if ticks.total() != expected {
		t.Errorf("expected total %d, got %d", expected, ticks.total())
	}
}
