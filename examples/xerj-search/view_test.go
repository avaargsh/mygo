package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/xerj"
	"github.com/egoist/mygo/ui"
)

// Render the actual example view without opening a native window or contacting
// an engine. No worker is started: queue and cancel transitions are deterministic.
func TestNativeSearchPanelSubmitCancel(t *testing.T) {
	manager := xerj.NewManager("http://127.0.0.1:1")
	service, err := xerj.NewWorkspaceSearchService(manager, "panel-fixture")
	if err != nil {
		t.Fatal(err)
	}
	controller := xerj.NewSearchController(service)
	defer controller.Close()
	p := &panel{controller: controller, notice: "Synthetic view test"}
	tt := ui.NewTester(p.view, 720, 520)
	if !tt.HasText("Enter a query to search the fixture.") {
		t.Fatalf("idle view: %v", tt.Texts())
	}
	if err := tt.Click("Search query"); err != nil {
		t.Fatal(err)
	}
	tt.Type("needle")
	if p.query != "needle" {
		t.Fatalf("input did not update: %q", p.query)
	}
	if err := tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	if p.submitted != "needle" || controller.Snapshot().State != xerj.SearchLoading {
		t.Fatalf("submit did not enter loading: %q %+v", p.submitted, controller.Snapshot())
	}
	if !tt.HasText("Searching…") {
		t.Fatalf("loading view: %v", tt.Texts())
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if got := controller.Snapshot(); got.State != xerj.SearchCanceled || len(got.Hits) != 0 {
		t.Fatalf("cancel: %+v", got)
	}
	if !tt.HasText("Search canceled.") {
		t.Fatalf("canceled view: %v", tt.Texts())
	}
}

func TestNativeSearchPanelDisabled(t *testing.T) {
	controller := xerj.NewSearchController(nil)
	defer controller.Close()
	p := &panel{controller: controller, query: "needle", notice: "Disabled fixture"}
	tt := ui.NewTester(p.view, 720, 520)
	if !tt.HasText("Search is disabled.") {
		t.Fatalf("disabled view: %v", tt.Texts())
	}
	// Tester may reject a disabled control, or deliver a no-op click.
	// In either case, neither query submission nor a result may appear.
	_ = tt.Click("Search")
	if got := controller.Snapshot(); p.submitted != "" || got.State != xerj.SearchDisabled || len(got.Hits) != 0 {
		t.Fatalf("disabled panel accepted query: %q %+v", p.submitted, got)
	}
}

func TestNativeSearchPanelRendersResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/fixture-text/_search" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"hits":{"hits":[{"_index":"fixture-text","_source":{"ax_path":"mygo-xerj-fixture.txt","start_line":1},"fields":{"_passage":{"text":"mygoxerjfixturemarker","start_offset":0}}}]}}`)
	}))
	defer server.Close()
	m := xerj.NewManager(server.URL)
	ticket, err := m.Begin("panel-fixture", "fixture-text")
	if err != nil {
		t.Fatal(err)
	}
	// This is an HTTP fixture, not an installed-engine verifier test.
	if err := m.Publish(ticket); err != nil {
		t.Fatal(err)
	}
	service, err := xerj.NewWorkspaceSearchService(m, "panel-fixture")
	if err != nil {
		t.Fatal(err)
	}
	controller := xerj.NewSearchController(service)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	done, redraw := make(chan error, 1), make(chan struct{}, 1)
	go func() { done <- controller.Serve(ctx, func() {
		select {
		case redraw <- struct{}{}:
		default:
		}
	}) }()
	defer func() {
		controller.Close()
		cancel()
		select {
		case <-done:
		case <-time.After(6 * time.Second):
			t.Error("search worker did not exit")
		}
	}()
	p := &panel{controller: controller, query: "mygoxerjfixturemarker", notice: "Synthetic view test"}
	tt := ui.NewTester(p.view, 720, 520)
	if err := tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-redraw:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// Render a new frame from the controller's snapshot, not a callback payload.
	tt = ui.NewTester(p.view, 720, 520)
	for _, want := range []string{"1 match", "mygo-xerj-fixture.txt:1", "mygoxerjfixturemarker"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %v", want, tt.Texts())
		}
	}
	controller.Close()
	tt = ui.NewTester(p.view, 720, 520)
	if !tt.HasText("Search is disabled.") || tt.HasText("mygo-xerj-fixture.txt:1") {
		t.Fatalf("closed view retains results: %v", tt.Texts())
	}
}
