package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/apps/workspace/core"
)

func closeFixture(t *testing.T) *shell {
	t.Helper()
	dir := t.TempDir()
	w, err := core.NewWorkspace("root")
	if err != nil {
		t.Fatal(err)
	}
	content, err := builtInContent(dir)
	if err != nil {
		t.Fatal(err)
	}
	return &shell{workspace: w, content: content, statePath: filepath.Join(dir, "state.json"), theme: "system"}
}

func TestWorkspaceCloseWithoutUnsavedStateDoesNotPrompt(t *testing.T) {
	s := closeFixture(t)
	if !s.confirmClose(func(mygo.MessageOptions) (mygo.MessageResult, error) {
		t.Fatal("clean workspace prompted")
		return mygo.MessageResult{}, nil
	}) {
		t.Fatal("clean workspace did not close")
	}
}

func TestWorkspaceCloseProtectsHiddenNoteAndUnsavedLayout(t *testing.T) {
	for _, tc := range []struct {
		name      string
		result    int
		dialogErr error
		wantClose bool
		wantSave  bool
	}{
		{name: "cancel", result: 2},
		{name: "unknown response", result: -1},
		{name: "dialog failure", result: 1, dialogErr: errors.New("unavailable")},
		{name: "discard", result: 1, wantClose: true},
		{name: "save", result: 0, wantClose: true, wantSave: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := closeFixture(t)
			s.open("notes")
			note := s.note()
			note.Text = "draft"
			before := s.workspace.Snapshot()
			if err := s.workspace.Close("root", before.Panes["root"].ActiveTab); err != nil {
				t.Fatal(err)
			}
			s.split(core.Horizontal)
			got := s.confirmClose(func(opts mygo.MessageOptions) (mygo.MessageResult, error) {
				if opts.DefaultButton != 2 || opts.CancelButton != 2 || len(opts.Buttons) != 3 {
					t.Fatalf("dialog did not default to cancellation: %+v", opts)
				}
				return mygo.MessageResult{Button: tc.result}, tc.dialogErr
			})
			if got != tc.wantClose {
				t.Fatalf("close result %v; want %v", got, tc.wantClose)
			}
			notePath := filepath.Join(filepath.Dir(s.statePath), "notes.md")
			if !tc.wantSave {
				for _, path := range []string{s.statePath, notePath} {
					if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("unexpected disk change: %v", err)
					}
				}
				if !note.Dirty() || !s.dirty {
					t.Fatal("close decision lost current unsaved buffers")
				}
				return
			}
			w, theme, err := core.LoadState(s.statePath)
			if err != nil || len(w.Snapshot().Panes) != 2 || theme != "system" {
				t.Fatalf("saved workspace did not restore: %v", err)
			}
			data, err := os.ReadFile(notePath)
			if err != nil || string(data) != "draft" || s.dirty || note.Dirty() {
				t.Fatalf("saved note did not restore: %q, %v", data, err)
			}
		})
	}
}

func TestWorkspaceCloseSaveFailureKeepsWindowAndDraft(t *testing.T) {
	s := closeFixture(t)
	s.open("notes")
	s.note().Text = "draft"
	s.statePath = filepath.Join(t.TempDir(), "missing", "state.json")
	if s.confirmClose(func(mygo.MessageOptions) (mygo.MessageResult, error) {
		return mygo.MessageResult{Button: 0}, nil
	}) {
		t.Fatal("allowed close after save failure")
	}
	if !s.dirty || s.note().Text != "draft" || !strings.Contains(s.status, "Save failed:") {
		t.Fatalf("save failure did not retain workspace or explain the error: %q", s.status)
	}
}

func TestWorkspaceCloseChecksOnlyDirtyNoteAndRejectsReentry(t *testing.T) {
	s := closeFixture(t)
	s.open("notes")
	if err := core.SaveState(s.statePath, s.workspace, s.theme); err != nil {
		t.Fatal(err)
	}
	s.dirty = false
	s.note().Text = "draft"
	if !s.confirmClose(func(mygo.MessageOptions) (mygo.MessageResult, error) {
		if s.confirmClose(func(mygo.MessageOptions) (mygo.MessageResult, error) {
			t.Fatal("nested confirmation")
			return mygo.MessageResult{}, nil
		}) {
			t.Fatal("reentrant close was allowed")
		}
		return mygo.MessageResult{Button: 0}, nil
	}) || s.note().Dirty() {
		t.Fatal("note-only save did not complete")
	}
}

func TestWorkspaceEndSessionDoesNotBlockOnDialog(t *testing.T) {
	s := closeFixture(t)
	s.open("notes")
	s.note().Text = "draft"
	s.sessionEnding = true
	if !s.confirmClose(func(mygo.MessageOptions) (mygo.MessageResult, error) {
		t.Fatal("OS session ending prompted")
		return mygo.MessageResult{}, nil
	}) {
		t.Fatal("OS session ending was blocked")
	}
	if !s.note().Dirty() {
		t.Fatal("OS session ending silently saved the note")
	}
}
