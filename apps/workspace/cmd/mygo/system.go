package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/apps/workspace/adapters/system"
	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/ui"
)

type systemSession struct {
	provider system.Provider
	snapshot *core.SystemSnapshot
	err      string
}

func newSystemSession() *systemSession {
	return &systemSession{provider: system.New()}
}

func (s *shell) refreshSystem(session *systemSession) {
	snapshot, err := session.provider.Snapshot(context.Background())
	if err != nil {
		session.err = err.Error()
		s.status = "System metrics refresh failed: " + err.Error()
		return
	}
	session.snapshot = &snapshot
	session.err = ""
	s.status = ""
}

func formatSystemBytes(value uint64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	div, exp := uint64(unit), 0
	for n := value / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(div), "KMGTPE"[exp])
}

func (s *shell) renderSystem(c *ui.Context, session *systemSession) {
	ui.Row(c).Gap(spaceControl).Wrap().Children(func() {
		ui.Button(c, "Refresh metrics").OnClick(func() { s.refreshSystem(session) })
	})
	if session.err != "" {
		ui.Text(c, "Last refresh failed: "+session.err)
	}
	if session.snapshot == nil {
		ui.Text(c, "System metrics have not been collected. Refresh to read this host.")
		return
	}
	snapshot := session.snapshot
	ui.Text(c, "Captured: "+snapshot.CapturedAt.Format("15:04:05"))
	ui.Divider(c)
	ui.Text(c, "CPU").Bold()
	if snapshot.CPU == nil {
		ui.Text(c, "Unavailable")
	} else if snapshot.CPU.UtilizationPercent == nil {
		ui.Text(c, fmt.Sprintf("%d logical cores · Sampling; refresh again for utilization", snapshot.CPU.LogicalCores))
	} else {
		ui.Text(c, fmt.Sprintf("%d logical cores · %.1f%% utilization", snapshot.CPU.LogicalCores, *snapshot.CPU.UtilizationPercent))
	}
	ui.Divider(c)
	ui.Text(c, "Memory").Bold()
	if snapshot.Memory == nil {
		ui.Text(c, "Unavailable")
	} else {
		m := snapshot.Memory
		ui.Text(c, fmt.Sprintf("%s used / %s total · %s available (%.1f%% used)", formatSystemBytes(m.UsedBytes), formatSystemBytes(m.TotalBytes), formatSystemBytes(m.AvailableBytes), m.UsedPercent))
	}
	ui.Divider(c)
	ui.Text(c, "Disk").Bold()
	if snapshot.Disk == nil {
		ui.Text(c, "Unavailable")
	} else {
		d := snapshot.Disk
		ui.Text(c, fmt.Sprintf("%s · %s used / %s total · %s available (%.1f%% used)", d.Path, formatSystemBytes(d.UsedBytes), formatSystemBytes(d.TotalBytes), formatSystemBytes(d.AvailableBytes), d.UsedPercent))
	}
	ui.Divider(c)
	ui.Text(c, "Network · cumulative counters").Bold()
	if snapshot.Network == nil {
		ui.Text(c, "Unavailable")
	} else {
		n := snapshot.Network
		ui.Text(c, fmt.Sprintf("%d interfaces · Received %s · Sent %s", n.Interfaces, formatSystemBytes(n.ReceivedBytes), formatSystemBytes(n.SentBytes)))
	}
	if len(snapshot.Unavailable) > 0 {
		ui.Text(c, "Unavailable metrics: "+strings.Join(snapshot.Unavailable, ", "))
	}
}
