//go:build linux

package system

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/apps/workspace/core"
	"golang.org/x/sys/unix"
)

type LinuxProvider struct {
	mu         sync.Mutex
	procRoot   string
	fsRoot     string
	previousCPU *core.CPUTime
}

func New() Provider { return NewLinuxProvider("/proc", "/") }

func NewLinuxProvider(procRoot, fsRoot string) *LinuxProvider {
	if procRoot == "" {
		procRoot = "/proc"
	}
	if fsRoot == "" {
		fsRoot = "/"
	}
	return &LinuxProvider{procRoot: procRoot, fsRoot: fsRoot}
}

func (p *LinuxProvider) Snapshot(ctx context.Context) (core.SystemSnapshot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return core.SystemSnapshot{}, err
	}
	s := core.SystemSnapshot{CapturedAt: time.Now()}
	cpu, err := readCPUTime(filepath.Join(p.procRoot, "stat"))
	if err != nil {
		s.Unavailable = append(s.Unavailable, "cpu")
	} else {
		stats := &core.CPUStats{LogicalCores: runtime.NumCPU()}
		if p.previousCPU != nil {
			if value, ok := core.CPUUsagePercent(*p.previousCPU, cpu); ok {
				stats.UtilizationPercent = &value
			}
		}
		p.previousCPU = &cpu
		s.CPU = stats
	}
	if err := ctx.Err(); err != nil {
		return core.SystemSnapshot{}, err
	}
	total, available, err := readMemory(filepath.Join(p.procRoot, "meminfo"))
	if err != nil {
		s.Unavailable = append(s.Unavailable, "memory")
	} else if value, ok := core.NewMemoryStats(total, available); ok {
		s.Memory = &value
	} else {
		s.Unavailable = append(s.Unavailable, "memory")
	}
	var disk unix.Statfs_t
	if err := unix.Statfs(p.fsRoot, &disk); err != nil || disk.Bsize <= 0 {
		s.Unavailable = append(s.Unavailable, "disk")
	} else if total, ok := multiply(disk.Blocks, uint64(disk.Bsize)); !ok {
		s.Unavailable = append(s.Unavailable, "disk")
	} else if available, ok := multiply(disk.Bavail, uint64(disk.Bsize)); !ok {
		s.Unavailable = append(s.Unavailable, "disk")
	} else if value, ok := core.NewDiskStats(p.fsRoot, total, available); ok {
		s.Disk = &value
	} else {
		s.Unavailable = append(s.Unavailable, "disk")
	}
	received, sent, interfaces, err := readNetwork(filepath.Join(p.procRoot, "net", "dev"))
	if err != nil || interfaces == 0 {
		s.Unavailable = append(s.Unavailable, "network")
	} else {
		s.Network = &core.NetworkStats{ReceivedBytes: received, SentBytes: sent, Interfaces: interfaces}
	}
	if err := ctx.Err(); err != nil {
		return core.SystemSnapshot{}, err
	}
	return s, nil
}

func add(a, b uint64) (uint64, bool) {
	if b > math.MaxUint64-a {
		return 0, false
	}
	return a + b, true
}

func multiply(a, b uint64) (uint64, bool) {
	if a != 0 && b > math.MaxUint64/a {
		return 0, false
	}
	return a * b, true
}

func readCPUTime(name string) (core.CPUTime, error) {
	f, err := os.Open(name)
	if err != nil {
		return core.CPUTime{}, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return core.CPUTime{}, err
		}
		return core.CPUTime{}, errors.New("missing aggregate CPU counters")
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return core.CPUTime{}, errors.New("invalid aggregate CPU counters")
	}
	var total, idle uint64
	counters := fields[1:]
	if len(counters) > 8 {
		counters = counters[:8]
	}
	for i, field := range counters {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return core.CPUTime{}, fmt.Errorf("invalid CPU counter: %w", err)
		}
		var ok bool
		total, ok = add(total, value)
		if !ok {
			return core.CPUTime{}, errors.New("CPU counter overflow")
		}
		if i == 3 || i == 4 {
			idle, ok = add(idle, value)
			if !ok {
				return core.CPUTime{}, errors.New("CPU idle counter overflow")
			}
		}
	}
	if total == 0 || idle > total {
		return core.CPUTime{}, errors.New("invalid CPU counter range")
	}
	return core.CPUTime{Total: total, Idle: idle}, nil
}

func readMemory(name string) (uint64, uint64, error) {
	f, err := os.Open(name)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	var total, available uint64
	var haveTotal, haveAvailable bool
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 || fields[2] != "kB" {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid memory counter: %w", err)
		}
		bytes, ok := multiply(value, 1024)
		if !ok {
			return 0, 0, errors.New("memory counter overflow")
		}
		switch strings.TrimSuffix(fields[0], ":") {
		case "MemTotal":
			total, haveTotal = bytes, true
		case "MemAvailable":
			available, haveAvailable = bytes, true
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	if !haveTotal || !haveAvailable || total == 0 || available > total {
		return 0, 0, errors.New("MemTotal or MemAvailable unavailable")
	}
	return total, available, nil
}

func readNetwork(name string) (uint64, uint64, int, error) {
	f, err := os.Open(name)
	if err != nil {
		return 0, 0, 0, err
	}
	defer f.Close()
	var received, sent uint64
	interfaces := 0
	var ok bool
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		iface := strings.TrimSpace(line[:colon])
		if iface == "" || iface == "lo" {
			continue
		}
		fields := strings.Fields(line[colon+1:])
		if len(fields) < 16 {
			return 0, 0, 0, fmt.Errorf("invalid network counters for %s", iface)
		}
		rx, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid receive counter for %s: %w", iface, err)
		}
		tx, err := strconv.ParseUint(fields[8], 10, 64)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid transmit counter for %s: %w", iface, err)
		}
		received, ok = add(received, rx)
		if !ok {
			return 0, 0, 0, errors.New("network receive counter overflow")
		}
		sent, ok = add(sent, tx)
		if !ok {
			return 0, 0, 0, errors.New("network transmit counter overflow")
		}
		interfaces++
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, 0, err
	}
	return received, sent, interfaces, nil
}
