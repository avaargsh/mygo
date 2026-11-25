package main

import (
	"os"
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

func TestBrowserSessionStartsWithoutNativeResource(t *testing.T) {
	w, err := core.NewWorkspace("root")
	if err != nil {
		t.Fatal(err)
	}
	content, err := builtInContent(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &shell{workspace: w, content: content}
	s.open("browser")
	session := content.Session("browser")
	b, ok := session.Value.(*browserSession)
	if !ok || b.view != nil || b.address != "" {
		t.Fatal("browser opened a page without an explicit address")
	}
	b.address = "file:///tmp/unsafe"
	s.navigateBrowser(b)
	if b.view != nil || s.status == "" {
		t.Fatal("invalid navigation created a native resource")
	}
	b.address = "https://example.org/"
	s.navigateBrowser(b)
	if b.view != nil || s.status != "desktop window not ready" {
		t.Fatal("headless session tried to start a native WebView")
	}
	snap := w.Snapshot()
	if err := w.Close("root", snap.Panes["root"].ActiveTab); err != nil {
		t.Fatal(err)
	}
	s.open("browser")
	if content.Session("browser") != session {
		t.Fatal("closing the browser tab evicted its session")
	}
	s.releaseBrowser(b)
	if b.view != nil {
		t.Fatal("release did not leave an empty browser")
	}
}

func TestTerminalRestoredTabDoesNotStartShell(t *testing.T) {
	w, err := core.NewWorkspace("root")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	content, err := builtInContent(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := &shell{workspace: w, content: content}
	s.open("terminal")
	session := s.terminalSession()
	if session == nil || session.State() != core.TerminalIdle {
		t.Fatal("opening terminal tab started a process")
	}
	s.startTerminal(session)
	if session.State() != core.TerminalIdle || s.status != "desktop window not ready" {
		t.Fatal("headless terminal started a process")
	}
	snapshot := w.Snapshot()
	if err := w.Close("root", snapshot.Panes["root"].ActiveTab); err != nil {
		t.Fatal(err)
	}
	s.open("terminal")
	if s.terminalSession() != session {
		t.Fatal("closing view evicted terminal session")
	}
	if err := core.SaveState(filepath.Join(dir, "state.json"), w, "system"); err != nil {
		t.Fatal(err)
	}
	restored, _, err := core.LoadState(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	newContent, err := builtInContent(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, pane := range restored.Snapshot().Panes {
		for _, tab := range pane.Tabs {
			if _, err := newContent.Open(tab.SessionID); err != nil {
				t.Fatal(err)
			}
		}
	}
	restarted := &shell{workspace: restored, content: newContent}
	if session := restarted.terminalSession(); session == nil || session.State() != core.TerminalIdle {
		t.Fatal("restored tab not idle")
	}
}

func TestFilesRootGrantAndRevoke(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	browser, err := core.NewFileBrowser(root.FS())
	if err != nil {
		root.Close()
		t.Fatal(err)
	}
	f := &filesSession{root: root, browser: browser, label: dir}
	defer f.close()
	if err := browser.Enter("docs"); err != nil {
		t.Fatal(err)
	}
	if err := browser.Up(); err != nil {
		t.Fatal(err)
	}
	if err := f.close(); err != nil {
		t.Fatal(err)
	}
	if f.root != nil || f.browser != nil || f.label != "" {
		t.Fatal("revoked grant retained")
	}
	if err := browser.Refresh(); err == nil {
		t.Fatal("closed root still accessible")
	}
}

func TestFormatSystemBytes(t *testing.T) {
	cases := []struct {
		value uint64
		want  string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1024 * 1024, "1.0 MiB"},
	}
	for _, tc := range cases {
		if got := formatSystemBytes(tc.value); got != tc.want {
			t.Fatalf("formatSystemBytes(%d) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestSystemInfoRefreshesThroughContentView(t *testing.T) {
	w, err := core.NewWorkspace("root")
	if err != nil {
		t.Fatal(err)
	}
	content, err := builtInContent(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &shell{workspace: w, content: content}
	s.open("system")
	tt := ui.NewTester(s.view, 1200, 800)
	if err := tt.Click("Refresh metrics"); err != nil {
		t.Fatal(err)
	}
	session := content.Session("system")
	system, ok := session.Value.(*systemSession)
	if !ok || system.snapshot == nil || system.snapshot.CapturedAt.IsZero() || system.err != "" {
		t.Fatalf("system snapshot not refreshed: %+v", system)
	}
}

func TestFilesViewPreviewsAndClosesTextFile(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "README.md")
	if err := os.WriteFile(name, []byte("# MyGo Workspace"), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	browser, err := core.NewFileBrowser(root.FS())
	if err != nil {
		root.Close()
		t.Fatal(err)
	}
	w, err := core.NewWorkspace("root")
	if err != nil {
		root.Close()
		t.Fatal(err)
	}
	content, err := builtInContent(t.TempDir())
	if err != nil {
		root.Close()
		t.Fatal(err)
	}
	s := &shell{workspace: w, content: content}
	s.open("files")
	session := s.filesSession()
	session.root, session.browser, session.label = root, browser, dir
	tt := ui.NewTester(s.view, 1200, 800)
	if err := tt.Click("Preview: README.md"); err != nil {
		t.Fatal(err)
	}
	if session.preview != "# MyGo Workspace" || session.previewErr != "" {
		t.Fatalf("unexpected preview: %+v", session)
	}
	if err := tt.Click("Back to files"); err != nil {
		t.Fatal(err)
	}
	if session.previewName != "" || session.preview != "" {
		t.Fatal("preview state was not cleared")
	}
	if err := session.close(); err != nil {
		t.Fatal(err)
	}
}

func TestFilesViewRejectsBinaryPreview(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "binary.bin"), []byte{0xff, 0xfe, 0xfd}, 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	browser, err := core.NewFileBrowser(root.FS())
	if err != nil {
		root.Close()
		t.Fatal(err)
	}
	f := &filesSession{root: root, browser: browser}
	s := &shell{}
	s.previewFile(f, "binary.bin")
	if f.preview != "" || f.previewErr != "preview supports UTF-8 text files only" {
		t.Fatalf("binary preview was not rejected: %+v", f)
	}
	if err := f.close(); err != nil {
		t.Fatal(err)
	}
}

func TestCommandPaletteOpensNotesThroughSharedDispatch(t *testing.T) {
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
	tt := ui.NewTester(s.view, 1400, 900)
	if err := tt.Click("Commands"); err != nil {
		t.Fatal(err)
	}
	if !s.paletteOpen {
		t.Fatal("command palette did not open")
	}
	// Select a unique search result without depending on scrolling order.
	s.paletteQuery = "markdown"
	tt = ui.NewTester(s.view, 1400, 900)
	if err := tt.Click("Run: Open Notes"); err != nil {
		t.Fatal(err)
	}
	if s.paletteOpen || s.paletteQuery != "" {
		t.Fatal("command palette did not close and clear search")
	}
	snap := w.Snapshot()
	pane := snap.Panes[snap.ActivePane]
	for _, tab := range pane.Tabs {
		if tab.ID == pane.ActiveTab && tab.SessionID == "notes" {
			return
		}
	}
	t.Fatal("Notes command did not activate a notes tab")
}

func TestWorkspaceCommandRejectsDisabledLayoutOperation(t *testing.T) {
	w, err := core.NewWorkspace("root")
	if err != nil {
		t.Fatal(err)
	}
	s := &shell{workspace: w}
	before := w.Snapshot()
	s.runCommand("layout.remove-pane")
	if s.status == "" || !reflect.DeepEqual(before, w.Snapshot()) || s.dirty {
		t.Fatal("disabled command changed the workspace")
	}
}
