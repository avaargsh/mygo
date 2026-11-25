package core

import (
	"math"
	"time"
)

// CPUTime is a cumulative CPU counter sample. Values are platform-neutral ticks.
type CPUTime struct {
	Total uint64
	Idle  uint64
}

type CPUStats struct {
	LogicalCores      int
	UtilizationPercent *float64
}

type MemoryStats struct {
	TotalBytes     uint64
	AvailableBytes uint64
	UsedBytes      uint64
	UsedPercent    float64
}

type DiskStats struct {
	Path           string
	TotalBytes     uint64
	AvailableBytes uint64
	UsedBytes      uint64
	UsedPercent    float64
}

type NetworkStats struct {
	ReceivedBytes uint64
	SentBytes     uint64
	Interfaces    int
}

// SystemSnapshot contains only successfully measured values. A nil metric is
// unavailable; it is never interpreted as a measured zero.
type SystemSnapshot struct {
	CapturedAt time.Time
	CPU        *CPUStats
	Memory     *MemoryStats
	Disk       *DiskStats
	Network    *NetworkStats
	Unavailable []string
}

func CPUUsagePercent(previous, current CPUTime) (float64, bool) {
	if current.Total <= previous.Total || current.Idle < previous.Idle || current.Idle > current.Total {
		return 0, false
	}
	totalDelta := current.Total - previous.Total
	idleDelta := current.Idle - previous.Idle
	if idleDelta > totalDelta {
		return 0, false
	}
	usage := 100 * float64(totalDelta-idleDelta) / float64(totalDelta)
	if math.IsNaN(usage) || math.IsInf(usage, 0) || usage < 0 || usage > 100 {
		return 0, false
	}
	return usage, true
}

func NewMemoryStats(total, available uint64) (MemoryStats, bool) {
	if total == 0 || available > total {
		return MemoryStats{}, false
	}
	used := total - available
	return MemoryStats{TotalBytes: total, AvailableBytes: available, UsedBytes: used, UsedPercent: 100 * float64(used) / float64(total)}, true
}

func NewDiskStats(path string, total, available uint64) (DiskStats, bool) {
	if path == "" || total == 0 || available > total {
		return DiskStats{}, false
	}
	used := total - available
	return DiskStats{Path: path, TotalBytes: total, AvailableBytes: available, UsedBytes: used, UsedPercent: 100 * float64(used) / float64(total)}, true
}
