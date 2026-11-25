package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContentViewsDetachWithoutEndingSession(t *testing.T) {
	w, err := NewWorkspace("left")
	if err != nil {
		t.Fatal(err)
	}
	m := NewContentManager()
	opens := 0
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(m.Register(ContentProvider{ID: "browser", Title: "Browser", Open: func() (any, error) {
		opens++
		return new(int), nil
	}}))
	session, err := m.Open("browser")
	must(err)
	must(w.Split("left", "root", "right", Horizontal, .5))
	must(w.Open("left", Tab{ID: "left-view", SessionID: session.ID, Title: "Browser"}))
	must(w.Open("right", Tab{ID: "right-view", SessionID: session.ID, Title: "Browser"}))
	expect := func(want ContentViewState) {
		t.Helper()
		snapshot := w.Snapshot()
		if got := ResolveContentViews(snapshot, "browser"); got != want {
			t.Fatalf("view projection %+v, want %+v", got, want)
		}
		if got := SessionOwner(snapshot, "browser"); got != want.MountPane {
			t.Fatalf("mount %q, want %q", got, want.MountPane)
		}
	}
	must(w.Focus("right"))
	expect(ContentViewState{TabReferences: 2, ActiveViews: 2, MountPane: "right"})
	must(w.Focus("left"))
	expect(ContentViewState{TabReferences: 2, ActiveViews: 2, MountPane: "left"})
	must(w.Focus("right"))
	must(w.Open("right", Tab{ID: "right-note", SessionID: "notes", Title: "Notes"}))
	expect(ContentViewState{TabReferences: 2, ActiveViews: 1, MountPane: "left"})
	must(w.Close("left", "left-view"))
	expect(ContentViewState{TabReferences: 1})
	must(w.Close("right", "right-view"))
	expect(ContentViewState{})
	if got := m.Session(session.ID); got != session || opens != 1 {
		t.Fatalf("closing last view changed runtime session: %v, opens=%d", got, opens)
	}
	reopened, err := m.Open("browser")
	must(err)
	if reopened != session || opens != 1 {
		t.Fatal("reopening a view recreated its live session")
	}
}

func TestContentLayoutRestoreKeepsReferenceNotLiveState(t *testing.T) {
	const instanceID = "chat-demo"
	const draft = "unpersisted-runtime-draft"
	w, err := NewWorkspace("root")
	if err != nil {
		t.Fatal(err)
	}
	newManager := func() *ContentManager {
		t.Helper()
		m := NewContentManager()
		if err := m.Register(ContentProvider{
			ID: "chat", Title: "Chat",
			Open:        func() (any, error) { return new(string), nil },
			NewInstance: func(string) (any, error) { return new(string), nil },
		}); err != nil {
			t.Fatal(err)
		}
		return m
	}
	m := newManager()
	live, err := m.OpenInstance("chat", instanceID)
	if err != nil {
		t.Fatal(err)
	}
	*live.Value.(*string) = draft
	if err := w.Open("root", Tab{ID: "tab", SessionID: live.ID, Title: "Chat"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "state.json")
	if err := SaveState(path, w, "system"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(raw), instanceID) || strings.Contains(string(raw), draft) {
		t.Fatalf("layout did not store only the instance reference: %s (%v)", raw, err)
	}
	restored, _, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	fresh := newManager()
	if fresh.Session(instanceID) != nil {
		t.Fatal("loading the layout recreated a live session")
	}
	tab := restored.Snapshot().Panes["root"].Tabs[0]
	idle, err := fresh.OpenInstance("chat", tab.SessionID)
	if err != nil || idle == live || *idle.Value.(*string) != "" {
		t.Fatalf("restored instance was not an independent idle binding: %v, %v", idle, err)
	}
}
