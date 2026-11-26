package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

func TestWorkspaceLocalSearchLive(t *testing.T) {
	endpoint, index := os.Getenv("MYGO_XERJ_APP_LIVE_ENDPOINT"), os.Getenv("MYGO_XERJ_APP_LIVE_INDEX")
	if endpoint == "" && index == "" {
		t.Skip("requires the disposable installed-XERJ fixture")
	}
	if endpoint == "" || index == "" {
		t.Fatal("incomplete installed-XERJ fixture configuration")
	}
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
	if err := tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	f := s.searchSession()
	defer f.stop()
	f.endpoint, f.index = endpoint, index
	tt.Frame()
	if err := tt.Click("Search text"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEnter)
	if f.pending != nil || f.result != nil {
		t.Fatal("Enter submitted an empty search")
	}
	tt.Type("asterquill")
	if f.query != "asterquill" {
		t.Fatalf("query input = %q", f.query)
	}
	tt.Key(0, ui.KeyEnter)
	if f.pending == nil && f.result == nil {
		t.Fatal("Enter did not submit the search")
	}
	wait.Until(t, "live search results to arrive", func() bool {
		tt.Frame()
		return f.result != nil
	})
	if f.result.err != nil {
		t.Fatal(f.result.err)
	}
	for _, hit := range f.result.hits {
		if hit.Index == index && filepath.Base(hit.Path) == "guide.md" && strings.Contains(strings.ToLower(hit.Text), "asterquill") {
			if !tt.HasText("synthetic runbook verification marker") {
				t.Fatalf("returned passage was not rendered: %v", tt.Texts())
			}
			if !tt.HasText(searchSource(hit)) {
				t.Fatalf("source was not rendered: %v", tt.Texts())
			}
			t.Logf("verified exact index %s and source %s", index, searchSource(hit))
			return
		}
	}
	t.Fatalf("synthetic fixture passage missing: %+v", f.result.hits)
}
