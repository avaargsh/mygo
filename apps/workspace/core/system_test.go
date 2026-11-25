package core

import (
	"math"
	"testing"
)

func TestCPUUsagePercentUsesCounterDelta(t *testing.T) {
	previous := CPUTime{Total: 1000, Idle: 900}
	current := CPUTime{Total: 1100, Idle: 950}
	got, ok := CPUUsagePercent(previous, current)
	if !ok || math.Abs(got-50) > 0.001 {
		t.Fatalf("usage = %v, available = %v; want 50%%", got, ok)
	}
}

func TestCPUUsagePercentRejectsFirstAndInvalidSamples(t *testing.T) {
	cases := []struct {
		name string
		old  CPUTime
		next CPUTime
	}{
		{"first sample", CPUTime{}, CPUTime{Total: 100, Idle: 90}},
		{"no elapsed ticks", CPUTime{Total: 100, Idle: 90}, CPUTime{Total: 100, Idle: 90}},
		{"counter reset", CPUTime{Total: 100, Idle: 90}, CPUTime{Total: 50, Idle: 40}},
		{"idle exceeds total", CPUTime{Total: 100, Idle: 90}, CPUTime{Total: 120, Idle: 130}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := CPUUsagePercent(tc.old, tc.next); ok {
				t.Fatal("invalid sample reported as available")
			}
		})
	}
}

func TestNewMemoryStatsRejectsUnavailableValues(t *testing.T) {
	if _, ok := NewMemoryStats(0, 0); ok {
		t.Fatal("zero total memory accepted")
	}
	if _, ok := NewMemoryStats(100, 101); ok {
		t.Fatal("available memory above total accepted")
	}
	m, ok := NewMemoryStats(1000, 250)
	if !ok || m.UsedBytes != 750 || math.Abs(m.UsedPercent-75) > 0.001 {
		t.Fatalf("unexpected memory stats: %+v, %v", m, ok)
	}
}

func TestNewDiskStatsRejectsUnavailableValues(t *testing.T) {
	if _, ok := NewDiskStats("/", 0, 0); ok {
		t.Fatal("zero disk capacity accepted")
	}
	if _, ok := NewDiskStats("/", 100, 101); ok {
		t.Fatal("available space above total accepted")
	}
	d, ok := NewDiskStats("/", 2000, 500)
	if !ok || d.UsedBytes != 1500 || math.Abs(d.UsedPercent-75) > 0.001 {
		t.Fatalf("unexpected disk stats: %+v, %v", d, ok)
	}
}
