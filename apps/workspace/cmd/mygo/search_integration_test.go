package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/apps/workspace/core"
)

func TestWorkspaceSearchProviderRestoresDisconnected(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	t.Setenv("MYGO_XERJ_PANEL", "1")
	t.Setenv("MYGO_XERJ_ENDPOINT", server.URL)
	t.Setenv("MYGO_XERJ_INDEX", "fixture-text")
	dir := t.TempDir()
	w, err := core.NewWorkspace("root")
	if err != nil {
		t.Fatal(err)
	}
	content, err := builtInContent(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := &shell{workspace: w, content: content}
	s.runCommand("content.search")
	search := s.searchSession()
	if search == nil {
		t.Fatal("search command did not open session")
	}
	if runner, state := search.owner.snapshot(); runner != nil || state != "Disconnected" {
		t.Fatalf("opening search: %v %s", runner, state)
	}
	search.query = "ephemeral query"
	if err := w.Close("root", w.Snapshot().Panes["root"].ActiveTab); err != nil {
		t.Fatal(err)
	}
	s.open("search")
	if s.searchSession() != search || search.query != "ephemeral query" {
		t.Fatal("tab closure evicted session")
	}
	path := filepath.Join(dir, "state.json")
	if err := core.SaveState(path, w, "system"); err != nil {
		t.Fatal(err)
	}
	restored, _, err := core.LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := builtInContent(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, pane := range restored.Snapshot().Panes {
		for _, tab := range pane.Tabs {
			if _, err := restoreContentSession(fresh, tab.SessionID); err != nil {
				t.Fatal(err)
			}
		}
	}
	next := (&shell{workspace: restored, content: fresh}).searchSession()
	if next == nil || next == search || next.query != "" {
		t.Fatal("query or connection session persisted across restart")
	}
	if runner, state := next.owner.snapshot(); runner != nil || state != "Disconnected" {
		t.Fatalf("restored search: %v %s", runner, state)
	}
	if requests.Load() != 0 {
		t.Fatal("opening or restoring a search tab contacted the engine")
	}
}
