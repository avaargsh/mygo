// XERJ-search is an opt-in native UI fixture search example.
// Run: go run ./examples/xerj-search
// The default is disabled and makes no engine requests.
package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/xerj"
	"github.com/egoist/mygo/ui"
)

type panel struct {
	controller *xerj.SearchController
	query      string
	submitted  string
	notice     string
	scroll     ui.ScrollState
}

func (p *panel) view(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(20).Gap(12).Children(func() {
		ui.Text(c, "Workspace search").FontSize(24).Bold()
		ui.Text(c, p.notice).FontSize(12).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			state := p.controller.Snapshot().State
			disabled := state == xerj.SearchDisabled
			input := ui.TextInput(c, &p.query).Grow(1).Placeholder("Search fixture text").Label("Search query").Disabled(disabled)
			clicked := ui.PrimaryButton(c, "Search").Disabled(disabled).Clicked()
			if !disabled && (input.Submitted() || clicked) {
				p.submitted = p.query
				p.scroll.Y = 0
				p.controller.Submit(p.query, 10)
			}
			if ui.Button(c, "Cancel").Disabled(p.controller.Snapshot().State != xerj.SearchLoading).Clicked() {
				p.controller.Cancel()
			}
		})
		reply := p.controller.Snapshot()
		ui.Text(c, searchStatusText(string(reply.State), len(reply.Hits))).TextColor(t.TextMuted)
		if p.submitted != "" && p.submitted != p.query {
			ui.Text(c, "Results refer to the last submitted query. Search to apply your edits.").FontSize(12).TextColor(t.TextMuted)
		}
		ui.Divider(c)
		ui.Scroll(c).Grow(1).TrackScroll(&p.scroll).Children(func() {
			ui.Column(c).Gap(12).Children(func() {
				for i, hit := range reply.Hits {
					ui.Column(c.Key(i)).Gap(4).Padding(10).Background(t.Surface).Radius(8).Children(func() {
						ui.Text(c, sourceLabel(hit.Path, hit.Line)).Bold().SingleLine()
						ui.Text(c, hit.Text)
					})
				}
			})
		})
	})
}

// fixtureService accepts only a disposable synthetic one-file corpus.
// The operator must exclusively own the pinned node and index throughout use.
// No arbitrary directory, file-open action, indexing or server launch occurs.
func fixtureService() (*xerj.WorkspaceSearchService, string) {
	const workspace = "native-search-fixture"
	disabled, err := xerj.NewWorkspaceSearchService(nil, workspace)
	if err != nil {
		panic(err) // fixed workspace identifier
	}
	if os.Getenv("MYGO_XERJ_PANEL") != "1" {
		return disabled, "Demo is disabled. See the example README to enable the synthetic fixture."
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		return disabled, "This fixture requires a desktop OS."
	}
	endpoint, index := os.Getenv("MYGO_XERJ_ENDPOINT"), os.Getenv("MYGO_XERJ_INDEX")
	m := xerj.NewManager(endpoint)
	ticket, err := m.Begin(workspace, index)
	if err != nil {
		return disabled, "Fixture configuration invalid. Search remains disabled."
	}
	v := xerj.TextIndexVerifier{Expected: map[string]string{
		"mygo-xerj-fixture.txt": fmt.Sprintf("%x", sha256.Sum256([]byte("mygoxerjfixturemarker"))),
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := v.Verify(ctx, xerj.BuildTarget{Workspace: workspace, Index: index, Endpoint: endpoint, Generation: 1}); err != nil {
		_ = m.Fail(ticket)
		return disabled, "Fixture verification failed. Search remains disabled."
	}
	if err := m.Publish(ticket); err != nil {
		return disabled, "Fixture publication failed. Search remains disabled."
	}
	s, err := xerj.NewWorkspaceSearchService(m, workspace)
	if err != nil {
		return disabled, "Fixture service unavailable."
	}
	return s, "Read-only synthetic fixture. Source labels are not file-open links."
}

func main() {
	// Bounded fixture preflight occurs before the GUI event loop, never in a view.
	service, notice := fixtureService()
	p := &panel{controller: xerj.NewSearchController(service), notice: notice}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	started := false
	mygo.App.WhenReady(func() {
		w := mygo.NewWindow(mygo.WindowOptions{
			Title:   "XERJ Search Fixture",
			Width:   720,
			Height:  520,
			Content: ui.View(p.view),
		})
		w.OnClosed(func() { p.controller.Close(); cancel() })
		started = true
		go func() { done <- p.controller.Serve(ctx, w.Invalidate) }()
	})
	err := mygo.App.Run()
	p.controller.Close()
	cancel()
	// Join after App.Run returns, not from a main-thread close handler.
	if started {
		<-done
	}
	if err != nil {
		log.Fatal(err)
	}
}
