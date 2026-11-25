package core

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestWorkspaceMoveAndCollapse(t *testing.T) {
	w, err := NewWorkspace("a")
	if err != nil {
		t.Fatal(err)
	}
	tab := Tab{ID: "t", SessionID: "session", Title: "Note"}
	if err := w.Open("a", tab); err != nil {
		t.Fatal(err)
	}
	if err := w.Split("a", "s", "b", Horizontal, .5); err != nil {
		t.Fatal(err)
	}
	if err := w.RemovePane("a"); err == nil {
		t.Fatal("removed nonempty pane")
	}
	if err := w.Move("a", "b", "t"); err != nil {
		t.Fatal(err)
	}
	if err := w.RemovePane("a"); err != nil {
		t.Fatal(err)
	}
	s := w.Snapshot()
	if s.Layout.ID != "b" || s.ActivePane != "b" || !reflect.DeepEqual(s.Panes["b"].Tabs, []Tab{tab}) {
		t.Fatalf("unexpected state: %+v", s)
	}
}

func TestWorkspaceRejectedCommandsAreAtomic(t *testing.T) {
	w, _ := NewWorkspace("a")
	_ = w.Open("a", Tab{ID: "t", SessionID: "s"})
	_ = w.Split("a", "split", "b", Vertical, .5)
	before := w.Snapshot()
	for _, err := range []error{w.Open("b", Tab{ID: "t", SessionID: "other"}), w.Move("a", "missing", "t"), w.Move("a", "b", "missing"), w.Split("a", "other", "b", Horizontal, .5), w.Focus("missing"), w.Close("b", "t"), w.Activate("b", "t")} {
		if err == nil {
			t.Fatal("invalid command accepted")
		}
	}
	if !reflect.DeepEqual(before, w.Snapshot()) {
		t.Fatal("invalid command mutated state")
	}
}

func TestWorkspaceSnapshotRoundTrip(t *testing.T) {
	w, _ := NewWorkspace("a")
	_ = w.Open("a", Tab{ID: "t", SessionID: "s"})
	_ = w.Split("a", "split", "b", Horizontal, .6)
	_ = w.Open("b", Tab{ID: "u", SessionID: "s"})
	_ = w.Focus("a")
	data, err := json.Marshal(w.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var s WorkspaceSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreWorkspace(s)
	if err != nil {
		t.Fatal(err)
	}
	s.Panes["a"].Tabs[0].ID = "changed"
	if !reflect.DeepEqual(w.Snapshot(), restored.Snapshot()) {
		t.Fatal("restore lost state or aliases input")
	}
}

func TestWorkspaceRejectsInconsistentSnapshot(t *testing.T) {
	w, _ := NewWorkspace("a")
	_ = w.Open("a", Tab{ID: "t", SessionID: "s"})
	_ = w.Split("a", "split", "b", Horizontal, .5)
	for _, mutate := range []func(*WorkspaceSnapshot){
		func(s *WorkspaceSnapshot) { delete(s.Panes, "a") },
		func(s *WorkspaceSnapshot) { s.Panes["orphan"] = PaneSnapshot{} },
		func(s *WorkspaceSnapshot) { s.ActivePane = "missing" },
		func(s *WorkspaceSnapshot) {
			s.Panes["b"] = PaneSnapshot{Tabs: []Tab{{ID: "t", SessionID: "s"}}, ActiveTab: "t"}
		},
		func(s *WorkspaceSnapshot) {
			s.Panes["a"] = PaneSnapshot{Tabs: []Tab{{ID: "t", SessionID: "s"}}, ActiveTab: "missing"}
		},
		func(s *WorkspaceSnapshot) { s.Version = 99 },
	} {
		s := w.Snapshot()
		mutate(&s)
		if _, err := RestoreWorkspace(s); err == nil {
			t.Fatal("accepted inconsistent state")
		}
	}
}

func TestWorkspaceCloseAndActivate(t *testing.T) {
	w, _ := NewWorkspace("a")
	_ = w.Open("a", Tab{ID: "one", SessionID: "shared"})
	_ = w.Open("a", Tab{ID: "two", SessionID: "shared"})
	if err := w.Activate("a", "one"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close("a", "one"); err != nil {
		t.Fatal(err)
	}
	p := w.Snapshot().Panes["a"]
	if p.ActiveTab != "two" || len(p.Tabs) != 1 || p.Tabs[0].SessionID != "shared" {
		t.Fatalf("unexpected pane %+v", p)
	}
}
