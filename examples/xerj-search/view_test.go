package main

import (
	"testing"

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
