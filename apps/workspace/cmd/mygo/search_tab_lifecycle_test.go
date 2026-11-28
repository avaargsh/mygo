package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

func TestWorkspaceSearchOutlivesLastTabAtDefaultSize(t *testing.T) {
	requests := make(chan string, 2)
	release := make(chan struct{})
	canceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Query struct{ Match map[string]string }
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || r.Method != "POST" || r.URL.Path != "/notes/_search" {
			t.Errorf("unexpected search request: %s %s (%v)", r.Method, r.URL.Path, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		query := request.Query.Match["body"]
		requests <- query
		if query == "retain" {
			select {
			case <-release:
				fmt.Fprint(w, `{"hits":{"hits":[{"_index":"notes","_source":{"path":"guide.md","line":7},"fields":{"_passage":[{"text":"Retained exact passage","start_offset":0}]}}]}}`)
			case <-r.Context().Done():
			}
		} else if query == "cancel" {
			<-r.Context().Done()
			close(canceled)
		}
	}))
	defer server.Close()
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
	tt := ui.NewTester(s.view, 1000, 680)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	assertHidden := func() {
		t.Helper()
		if core.SessionOwner(w.Snapshot(), "search") != "" || tt.HasText("Search local documents") {
			t.Fatal("closing last Search tab did not hide Search")
		}
		for _, pane := range w.Snapshot().Panes {
			for _, tab := range pane.Tabs {
				if tab.SessionID == "search" {
					t.Fatal("Search tab was only defocused, not closed")
				}
			}
		}
	}
	click("Search")
	f := s.searchSession()
	defer f.stop()
	f.endpoint, f.index, f.query = server.URL, "notes", "retain"
	tt.Frame()
	click("Find passages")
	if got := wait.For(t, "first request", requests); got != "retain" {
		t.Fatalf("first query = %q", got)
	}
	first := f.pending
	click("Close Search")
	assertHidden()
	if f.pending != first {
		t.Fatal("closing Search changed the in-flight request")
	}
	click("Sessions")
	if !tt.HasText("Cancel hidden search") {
		t.Fatalf("first query is not visible to the Session Manager: %v", tt.Texts())
	}
	close(release)
	wait.Until(t, "completion to be rendered by Sessions", func() bool {
		tt.Frame()
		return !tt.HasText("Cancel hidden search")
	})
	click("Explorer")
	click("Search")
	if !tt.HasText("Retained exact passage") || !tt.HasText(`"guide.md":7`) {
		t.Fatal("reopening Search lost the completed passage or its citation")
	}
	f.query = "cancel"
	tt.Frame()
	click("Find passages")
	if got := wait.For(t, "second request", requests); got != "cancel" {
		t.Fatalf("second query = %q", got)
	}
	last := f.pending
	click("Close Search")
	assertHidden()
	click("Sessions")
	click("Cancel hidden search")
	wait.For(t, "server receives cancellation", canceled)
	wait.For(t, "canceled worker completes", last)
	click("Explorer")
	click("Search")
	if tt.HasText("Retained exact passage") || tt.HasText("Searching…") || tt.HasText("Search failed:") ||
		f.result != nil || f.pending != nil {
		t.Fatal("reopened Search shows canceled request or earlier results")
	}
}
