package main

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/ui"
)

// This test drives the real native view without a desktop window.
func TestWorkspaceShellSplitMoveSave(t *testing.T) {
	w, err := core.NewWorkspace("pane-root")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	content, err := builtInContent(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := &shell{workspace: w, content: content, theme: "system", statePath: filepath.Join(dir, "state.json")}
	s.open("home")
	tt := ui.NewTester(s.view, 1600, 900)
	for _, label := range []string{"Split right", "Runtime info", "Move tab to next pane", "Save workspace"} {
		if err := tt.Click(label); err != nil {
			t.Fatalf("click %s: %v", label, err)
		}
	}
	snapshot := w.Snapshot()
	if len(snapshot.Panes) != 2 || snapshot.ActivePane != "pane-root" || len(snapshot.Panes["pane-root"].Tabs) != 2 || s.dirty {
		t.Fatalf("unexpected shell state: %+v, dirty=%v", snapshot, s.dirty)
	}
	restored, theme, err := core.LoadState(s.statePath)
	if err != nil {
		t.Fatal(err)
	}
	if theme != "system" || !reflect.DeepEqual(snapshot, restored.Snapshot()) {
		t.Fatal("saved state differs from displayed workspace")
	}
}

func TestNotesSessionSurvivesClosingView(t *testing.T) {
	dir := t.TempDir()
	w, err := core.NewWorkspace("root")
	if err != nil {
		t.Fatal(err)
	}
	content, err := builtInContent(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := &shell{workspace: w, content: content, theme: "system", statePath: filepath.Join(dir, "state.json")}
	s.open("notes")
	n := s.note()
	if n == nil {
		t.Fatal("notes session not opened")
	}
	n.Text = "# Draft"
	snapshot := w.Snapshot()
	if err := w.Close("root", snapshot.Panes["root"].ActiveTab); err != nil {
		t.Fatal(err)
	}
	s.open("notes")
	if s.note() != n || s.note().Text != "# Draft" || !n.Dirty() {
		t.Fatal("draft lost on closing view")
	}
	s.saveNote()
	restarted, err := builtInContent(dir)
	if err != nil {
		t.Fatal(err)
	}
	session, err := restarted.Open("notes")
	if err != nil {
		t.Fatal(err)
	}
	if note := session.Value.(*core.Note); note.Text != "# Draft" || note.Dirty() {
		t.Fatal("saved draft not restored")
	}
}
