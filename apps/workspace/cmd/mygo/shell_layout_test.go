package main

import (
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
