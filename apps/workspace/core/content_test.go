package core

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContentRegistryAndSessionReuse(t *testing.T) {
	m := NewContentManager()
	calls := 0
	p := ContentProvider{ID: "home", Title: "Home", Open: func() (any, error) { calls++; return &struct{}{}, nil }}
	if err := m.Register(p); err != nil {
		t.Fatal(err)
	}
	if m.Register(p) == nil || m.Register(ContentProvider{}) == nil {
		t.Fatal("invalid registration accepted")
	}
	if _, err := m.Open("unknown"); err == nil {
		t.Fatal("unknown provider accepted")
	}
	a, err := m.Open("home")
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.Open("home")
	if err != nil || a != b || calls != 1 {
		t.Fatal("session was reopened")
	}
	w, _ := NewWorkspace("root")
	if err := w.Open("root", Tab{ID: "view", SessionID: a.ID}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close("root", "view"); err != nil {
		t.Fatal(err)
	}
	c, err := m.Open("home")
	if err != nil || c != a {
		t.Fatal("closing a view destroyed the session")
	}
}

func TestContentOpenFailureCanRetry(t *testing.T) {
	m := NewContentManager()
	calls := 0
	if err := m.Register(ContentProvider{ID: "notes", Title: "Notes", Open: func() (any, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("unavailable")
		}
		return "ready", nil
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Open("notes"); err == nil {
		t.Fatal("failure hidden")
	}
	if s, err := m.Open("notes"); err != nil || s.Value != "ready" || calls != 2 {
		t.Fatal("failed open was cached")
	}
}

func TestNoteSaveRestoreAndDirty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.md")
	n, err := OpenNote(path)
	if err != nil {
		t.Fatal(err)
	}
	if n.Dirty() {
		t.Fatal("new note dirty")
	}
	n.Text = "# Markdown\n中文\n"
	if !n.Dirty() {
		t.Fatal("edit not tracked")
	}
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	restored, err := OpenNote(path)
	if err != nil || restored.Text != n.Text || restored.Dirty() || n.Dirty() {
		t.Fatal("note did not round trip")
	}
	n.Text = ""
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) != 0 {
		t.Fatal("empty edit not saved")
	}
}

func TestNoteRejectsOversizeAndInvalidUTF8(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.md")
	n, err := OpenNote(path)
	if err != nil {
		t.Fatal(err)
	}
	n.Text = "safe"
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{strings.Repeat("x", maxNoteBytes+1), string([]byte{0xff})} {
		n.Text = text
		if n.Save() == nil || !n.Dirty() {
			t.Fatal("bad edit accepted")
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "safe" {
			t.Fatal("prior save lost")
		}
	}
	if err := os.WriteFile(path, []byte{0xff}, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenNote(path); err == nil {
		t.Fatal("invalid disk content accepted")
	}
}
