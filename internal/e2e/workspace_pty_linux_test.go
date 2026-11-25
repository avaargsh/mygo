//go:build linux && !android

package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

func TestWorkspaceNativePTY(t *testing.T) {
	var s core.TerminalSession
	var term *terminal.Terminal
	err := s.Start(func() (core.TerminalResource, error) {
		var err error
		term, err = terminal.New(terminal.Options{
			Command: []string{"/bin/sh", "-c", "test -t 0 && test -t 1 || exit 9; echo ready; read line; echo reply:$line; exit 7"},
			Dir: t.TempDir(),
			NoClipboardWrite: true,
			ClipboardRead: terminal.ClipboardDeny,
		})
		if err != nil {
			return nil, err
		}
		return term, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Stop() })
	newWindow(t, mygo.WindowOptions{Title: "PTY smoke", Width: 800, Height: 500, Content: ui.View(func(c *ui.Context) {
		terminal.View(c, term).Fill()
	})})
	eventually(t, "PTY ready", func() bool { return strings.Contains(term.Text(), "ready") })
	term.Send([]byte("workspace-smoke\n"))
	eventually(t, "PTY output", func() bool { return strings.Contains(term.Text(), "reply:workspace-smoke") })
	select {
	case <-term.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("PTY did not exit")
	}
	if code, ok := s.ExitCode(); !ok || code != 7 {
		t.Fatalf("PTY exit = %d, available=%v", code, ok)
	}
}
