//go:build linux

package system

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeFixture(t *testing.T, root, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLinuxProviderSamplesRealisticProcfsAndCPUDelta(t *testing.T) {
	proc := t.TempDir()
	fsRoot := t.TempDir()
	writeFixture(t, proc, "stat", "cpu 100 0 100 800 0 0 0 0\n")
	writeFixture(t, proc, "meminfo", "MemTotal: 1024000 kB\nMemAvailable: 256000 kB\n")
	writeFixture(t, proc, "net", "")
	if err := os.Mkdir(filepath.Join(proc, "net"), 0700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(proc, "net"), "dev", "Inter-| Receive | Transmit\n face |bytes packets errs drop fifo frame compressed multicast|bytes packets errs drop fifo colls carrier compressed\n  lo: 100 1 0 0 0 0 0 0 200 1 0 0 0 0 0 0\n eth0: 1000 10 0 0 0 0 0 0 2000 20 0 0 0 0 0 0\n")
	p := NewLinuxProvider(proc, fsRoot)
	first, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.CPU == nil || first.CPU.LogicalCores < 1 || first.CPU.UtilizationPercent != nil {
		t.Fatalf("first CPU sample should not invent utilization: %+v", first.CPU)
	}
	if first.Memory == nil || first.Memory.TotalBytes != 1024000*1024 || first.Memory.AvailableBytes != 256000*1024 {
		t.Fatalf("bad memory sample: %+v", first.Memory)
	}
	if first.Disk == nil || first.Disk.TotalBytes == 0 {
		t.Fatalf("missing disk sample: %+v", first.Disk)
	}
	if first.Network == nil || first.Network.ReceivedBytes != 1000 || first.Network.SentBytes != 2000 || first.Network.Interfaces != 1 {
		t.Fatalf("loopback was not excluded: %+v", first.Network)
	}
	writeFixture(t, proc, "stat", "cpu 150 0 100 850 0 0 0 0\n")
	second, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.CPU == nil || second.CPU.UtilizationPercent == nil || *second.CPU.UtilizationPercent < 49.99 || *second.CPU.UtilizationPercent > 50.01 {
		t.Fatalf("expected 50%% CPU delta: %+v", second.CPU)
	}
}

func TestLinuxProviderReportsUnavailableInsteadOfZero(t *testing.T) {
	p := NewLinuxProvider(t.TempDir(), t.TempDir())
	snapshot, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Memory != nil || snapshot.Network != nil || snapshot.Disk == nil {
		t.Fatalf("unexpected partial sample: %+v", snapshot)
	}
	for _, want := range []string{"cpu", "memory", "network"} {
		found := false
		for _, item := range snapshot.Unavailable {
			if item == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing unavailable marker %q: %v", want, snapshot.Unavailable)
		}
	}
}

func TestLinuxProviderHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := NewLinuxProvider(t.TempDir(), t.TempDir())
	if _, err := p.Snapshot(ctx); err == nil {
		t.Fatal("canceled context was ignored")
	}
}
