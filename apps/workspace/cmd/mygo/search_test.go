package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

func TestWorkspaceLocalSearch(t *testing.T) {
	queries := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/notes/_search" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var req struct {
			Size  int
			Query struct{ Match map[string]string }
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Size != 10 {
			t.Errorf("unexpected request contract: %+v %v", req, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		query := req.Query.Match["body"]
		queries <- query
		if query == "empty" {
			fmt.Fprint(w, `{"hits":{"hits":[]}}`)
			return
		}
		if query == "unavailable" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `{"hits":{"hits":[{"_index":"notes","_source":{"path":"guide.md","line":7},"fields":{"_passage":[{"text":"Local only","start_offset":0}]}}]}}`)
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
	tt := ui.NewTester(s.view, 1600, 900)
	if err := tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	f := s.searchSession()
	if f == nil || f.pending != nil || f.result != nil || f.index != "" {
		t.Fatal("opening Search did not create an idle, unconfigured tab")
	}
	defer f.stop()
	f.endpoint, f.index = server.URL, "notes"
	for _, tc := range []struct {
		query string
		want  int
		fail  bool
	}{{"guide", 1, false}, {"empty", 0, false}, {"unavailable", 0, true}} {
		f.query = tc.query
		tt = ui.NewTester(s.view, 1600, 900)
		if err := tt.Click("Find passages"); err != nil {
			t.Fatal(err)
		}
		if q := wait.For(t, "search request to reach the local index", queries); q != tc.query {
			t.Fatalf("unexpected query %q", q)
		}
		wait.Until(t, "search results to arrive", func() bool {
			f.collect()
			return f.result != nil
		})
		if r := f.result; r == nil || (r.err != nil) != tc.fail || len(r.hits) != tc.want {
			t.Fatalf("%s: unexpected result %+v", tc.query, r)
		}
		if tc.want == 1 && (searchSource(f.result.hits[0]) != `"guide.md":7` || f.result.hits[0].Text != "Local only") {
			t.Fatalf("lost source or passage: %+v", f.result.hits[0])
		}
		tt.Frame()
	}
	f.endpoint = "http://example.invalid:9200"
	f.start(nil)
	if f.pending != nil || f.result == nil || f.result.err == nil {
		t.Fatal("non-loopback configuration was not rejected before any request")
	}
}

func TestWorkspaceSearchCancellationDropsLateResponse(t *testing.T) {
	received := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(received)
		<-r.Context().Done()
	}))
	defer server.Close()
	f := &searchSession{endpoint: server.URL, index: "notes", query: "first"}
	redraw := make(chan struct{}, 1)
	f.start(func() {
		select {
		case redraw <- struct{}{}:
		default:
		}
	})
	defer f.stop()
	wait.For(t, "request to start", received)
	old := f.pending
	f.stop()
	if f.draining != old || f.pending != nil || f.result != nil {
		t.Fatal("cancel did not retain only the cleanup fence")
	}
	wait.For(t, "canceled request to notify cleanup", redraw)
	f.collect()
	if f.draining != nil || f.pending != nil || f.result != nil {
		t.Fatal("canceled request did not drain or repopulated the view")
	}
}

func TestWorkspaceSessionManagerCanCancelHiddenSearch(t *testing.T) {
	received, canceled := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(received)
		<-r.Context().Done()
		close(canceled)
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
	tt := ui.NewTester(s.view, 1600, 900)
	if err := tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	f := s.searchSession()
	defer f.stop()
	f.endpoint, f.index, f.query = server.URL, "notes", "report"
	tt.Frame()
	if err := tt.Click("Find passages"); err != nil {
		t.Fatal(err)
	}
	wait.For(t, "request to reach the local index", received)
	pending := f.pending
	s.open("home")
	tt.Frame()
	if f.pending != pending || core.SessionOwner(w.Snapshot(), "search") != "" {
		t.Fatal("switching tabs canceled the request or left Search visible")
	}
	if err := tt.Click("Sessions"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Cancel hidden search"); err != nil {
		t.Fatal(err)
	}
	wait.For(t, "server context cancellation", canceled)
	wait.For(t, "canceled worker to finish", pending)
	if f.pending != nil || f.result != nil {
		t.Fatal("hidden search remained active or restored a canceled result")
	}
}
