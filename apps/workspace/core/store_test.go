package core

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStoreRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	w, _ := NewWorkspace("root")
	if err := w.Open("root", Tab{ID: "tab", SessionID: "notes"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Split("root", "split", "right", Horizontal, .6); err != nil {
		t.Fatal(err)
	}
	if err := w.Move("root", "right", "tab"); err != nil {
		t.Fatal(err)
	}
	if err := SaveState(path, w, "dark"); err != nil {
		t.Fatal(err)
	}
	got, theme, err := LoadState(path)
	if err != nil || theme != "dark" || !reflect.DeepEqual(w.Snapshot(), got.Snapshot()) {
		t.Fatalf("restore: %v %s", err, theme)
	}
	if err := w.Resize("split", .4); err != nil {
		t.Fatal(err)
	}
	if err := SaveState(path, w, "light"); err != nil {
		t.Fatal(err)
	}
	got, theme, err = LoadState(path)
	if err != nil || theme != "light" || !reflect.DeepEqual(w.Snapshot(), got.Snapshot()) {
		t.Fatalf("replace: %v %s", err, theme)
	}
}

func TestStoreRejectsInvalidAndPreservesPrevious(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	w, _ := NewWorkspace("root")
	if err := SaveState(path, w, "system"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if SaveState(path, w, "invalid") == nil {
		t.Fatal("invalid theme accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("previous save changed")
	}
}

func TestStoreMissingCorruptAndOversized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if _, _, err := LoadState(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing: %v", err)
	}
	for _, data := range []string{`{`, `{"version":2}`, `{"version":1,"theme":"dark","workspace":{}}`, strings.Repeat(" ", maxStateBytes+1)} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := LoadState(path); err == nil {
			t.Fatal("invalid state accepted")
		}
	}
}

func TestStoreWriteFailure(t *testing.T) {
	w, _ := NewWorkspace("root")
	path := filepath.Join(t.TempDir(), "directory")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if SaveState(path, w, "system") == nil {
		t.Fatal("directory replaced")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary file leaked: %v %v", entries, err)
	}
}

func TestStoreSyncsParentAfterReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	w, _ := NewWorkspace("root")
	if err := SaveState(path, w, "system"); err != nil {
		t.Fatal(err)
	}
	synced := false
	err := saveState(path, w, "dark", os.Rename, func(gotDir string) error {
		if gotDir != dir {
			t.Fatalf("wrong directory: %q", gotDir)
		}
		_, theme, err := LoadState(path)
		if err != nil || theme != "dark" {
			t.Fatalf("directory sync preceded replacement: %v %s", err, theme)
		}
		synced = true
		return nil
	})
	if err != nil || !synced {
		t.Fatalf("directory sync not completed: %v", err)
	}
}

func TestStoreReplacementFailurePreservesPrevious(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	w, _ := NewWorkspace("root")
	if err := SaveState(path, w, "system"); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("replace failed")
	err := saveState(path, w, "dark", func(_, _ string) error { return injected }, func(string) error {
		t.Fatal("directory sync after failed replacement")
		return nil
	})
	if !errors.Is(err, injected) {
		t.Fatalf("replacement error lost: %v", err)
	}
	_, theme, err := LoadState(path)
	if err != nil || theme != "system" {
		t.Fatalf("previous state not preserved: %v %s", err, theme)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary file leaked: %v %v", entries, err)
	}
}

func TestStoreDirectorySyncFailureIsReportedAfterReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	w, _ := NewWorkspace("root")
	injected := errors.New("directory sync failed")
	err := saveState(path, w, "dark", os.Rename, func(string) error { return injected })
	if !errors.Is(err, injected) {
		t.Fatalf("sync error lost: %v", err)
	}
	_, theme, err := LoadState(path)
	if err != nil || theme != "dark" {
		t.Fatalf("committed snapshot not readable: %v %s", err, theme)
	}
}
