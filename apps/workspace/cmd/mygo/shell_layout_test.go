package main

import (
	"errors"
	"reflect"
	"testing"

	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/ui"
)

func TestWorkspaceShellKeepsStableRegionsAndSidebarSwitches(t *testing.T) {
	w, err := core.NewWorkspace("root")
	if err != nil {
		t.Fatal(err)
	}
	content, err := builtInContent(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &shell{workspace: w, content: content, theme: "system"}
	s.open("home")
	initial := w.Snapshot()
	tt := ui.NewTester(s.view, 1000, 680)
	tt.Frame()
	for _, want := range []struct {
		role ui.Role
		name string
	}{
		{ui.RoleNavigation, "Activity bar"},
		{ui.RoleRegion, "Workspace Explorer"},
		{ui.RoleMain, "Content area"},
		{ui.RoleGroup, "Tab bar"},
		{ui.RoleStatus, "Status bar"},
	} {
		if _, ok := tt.AccessByRole(want.role, want.name); !ok {
			t.Fatalf("missing shell region %q", want.name)
		}
	}
	if err := tt.Click("Sessions"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.AccessByRole(ui.RoleRegion, "Session Manager"); !ok {
		t.Fatal("Sessions did not display the Session Manager")
	}
	if _, ok := tt.AccessByRole(ui.RoleRegion, "Workspace Explorer"); ok {
		t.Fatal("Explorer and Session Manager competed for the same sidebar")
	}
	if !reflect.DeepEqual(initial, w.Snapshot()) {
		t.Fatal("switching the sidebar changed the workspace layout")
	}
	if err := tt.Click("Explorer"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Runtime info"); err != nil {
		t.Fatal(err)
	}
	pane := w.Snapshot().Panes["root"]
	if len(pane.Tabs) != 2 || pane.Tabs[1].SessionID != "runtime" || pane.Tabs[1].ID != pane.ActiveTab {
		t.Fatalf("explorer launcher did not open content: %+v", pane)
	}
}

type retryTerminalFixture struct {
	done chan struct{}
	attempts int
}

func (f *retryTerminalFixture) Done() <-chan struct{} { return f.done }
func (f *retryTerminalFixture) ExitCode() int { return 0 }
func (f *retryTerminalFixture) Close() error {
	f.attempts++
	if f.attempts == 1 {
		return errors.New("injected close failure")
	}
	return nil
}

func TestSessionManagerRetriesHiddenTerminalAfterCloseFailure(t *testing.T) {
	s := closeFixture(t)
	s.open("terminal")
	terminal := s.terminalSession()
	resource := &retryTerminalFixture{done: make(chan struct{})}
	if err := terminal.Start(func() (core.TerminalResource, error) { return resource, nil }); err != nil {
		t.Fatal(err)
	}
	if err := terminal.Stop(); err == nil {
		t.Fatal("expected injected close failure")
	}
	s.open("home")
	if terminal.Resource() != nil || terminal.State() != core.TerminalStopping {
		t.Fatal("failed teardown must keep the view hidden and await exit")
	}
	tt := ui.NewTester(s.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("Sessions"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Close hidden terminal"); err != nil {
		t.Fatal(err)
	}
	if resource.attempts != 2 || terminal.State() != core.TerminalStopping {
		t.Fatal("hidden close retry did not reach resource or incorrectly reported exit")
	}
	close(resource.done)
	if terminal.State() != core.TerminalExited {
		t.Fatal("missing exit evidence")
	}
}
