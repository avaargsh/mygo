package main

import (
	"fmt"

	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

func (s *shell) startTerminal(session *core.TerminalSession) {
	win := s.window
	if win == nil {
		s.status = "desktop window not ready"
		return
	}
	err := session.Start(func() (core.TerminalResource, error) {
		t, err := terminal.New(terminal.Options{
			NoClipboardWrite: true,
			ClipboardRead:    terminal.ClipboardDeny,
			OnExit:           func(int) { win.Invalidate() },
		})
		if err != nil {
			return nil, err
		}
		return t, nil
	})
	if err != nil {
		s.status = "Terminal start failed: " + err.Error()
		return
	}
	s.status = ""
}

func (s *shell) stopTerminal(session *core.TerminalSession) {
	if err := session.Stop(); err != nil {
		s.status = "Terminal close failed: " + err.Error()
		return
	}
	s.status = ""
}

func (s *shell) terminalSession() *core.TerminalSession {
	if v := s.content.Session("terminal"); v != nil {
		t, _ := v.Value.(*core.TerminalSession)
		return t
	}
	return nil
}

func (s *shell) renderTerminal(c *ui.Context, snapshot core.WorkspaceSnapshot, paneID string, t *core.TerminalSession) {
	if core.SessionOwner(snapshot, "terminal") != paneID {
		ui.Text(c, "Terminal is displayed in another pane.")
		ui.Button(c, "Show terminal here").OnClick(func() { s.apply(s.workspace.Focus(paneID)) })
		return
	}
	state := t.State()
	ui.Row(c).Gap(spaceControl).Wrap().Children(func() {
		ui.Text(c, "Terminal: "+string(state))
		if code, ok := t.ExitCode(); ok {
			ui.Text(c, fmt.Sprintf("Exit code: %d", code))
		}
		ui.Button(c, "Start shell").Disabled(state == core.TerminalRunning || state == core.TerminalStopping).OnClick(func() { s.startTerminal(t) })
		ui.Button(c, "Close terminal").Disabled(!t.CanStop()).OnClick(func() { s.stopTerminal(t) })
	})
	if term, ok := t.Resource().(*terminal.Terminal); ok {
		terminal.View(c, term).Grow(1).MinHeight(0)
	} else {
		ui.Text(c, "Start shell to open a local interactive terminal.")
	}
}
