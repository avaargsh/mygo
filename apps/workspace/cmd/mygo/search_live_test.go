package main

import (
	"os"
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
	tt := ui.NewTester(s.view, 1600, 900)
	if err := tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	f := s.searchSession()
	defer f.stop()
	f.endpoint, f.index, f.query = endpoint, index, "asterquill"
	if err := tt.Click("Find passages"); err != nil {
		t.Fatal(err)
	}
	wait.Until(t, "live search results to arrive", func() bool {
		f.collect()
		return f.result != nil
	})
	if f.result.err != nil {
		t.Fatal(f.result.err)
	}
	for _, hit := range f.result.hits {
		if hit.Index == index && strings.HasSuffix(hit.Path, "guide.md") && strings.Contains(strings.ToLower(hit.Text), "asterquill") {
			tt.Frame()
			t.Logf("verified exact index %s and source %s", index, searchSource(hit))
			return
		}
	}
	t.Fatalf("synthetic fixture passage missing: %+v", f.result.hits)
}
