package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/egoist/mygo/plugins/xerj"
	"github.com/egoist/mygo/ui"
)

// Search is an opt-in synthetic-fixture session. Restoring the tab or opening
// the provider never contacts the engine. No workspace paths are indexed.
type searchSession struct {
	owner searchOwner
	query string
}

func connectSearchFixture(ctx context.Context, endpoint, index string) (searchRunner, error) {
	if endpoint == "" || index == "" {
		return nil, errors.New("fixture endpoint and exact index required")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	const workspace = "workspace-search-fixture"
	m := xerj.NewManager(endpoint)
	ticket, err := m.Begin(workspace, index)
	if err != nil {
		return nil, err
	}
	v := xerj.TextIndexVerifier{Expected: map[string]string{
		"mygo-xerj-fixture.txt": fmt.Sprintf("%x", sha256.Sum256([]byte("mygoxerjfixturemarker"))),
	}}
	if err := v.Verify(ctx, xerj.BuildTarget{Workspace: workspace, Index: index, Endpoint: endpoint, Generation: 1}); err != nil {
		_ = m.Fail(ticket)
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		_ = m.Fail(ticket)
		return nil, err
	}
	if err := m.Publish(ticket); err != nil {
		return nil, err
	}
	service, err := xerj.NewWorkspaceSearchService(m, workspace)
	if err != nil {
		return nil, err
	}
	return xerj.NewSearchController(service), nil
}

func (s *shell) searchSession() *searchSession {
	if session := s.content.Session("search"); session != nil {
		value, _ := session.Value.(*searchSession)
		return value
	}
	return nil
}

func (s *shell) renderSearch(c *ui.Context, session *searchSession) {
	runner, state := session.owner.snapshot()
	controller, _ := runner.(*xerj.SearchController)
	enabled := os.Getenv("MYGO_XERJ_PANEL") == "1"
	ui.Text(c, "Search (synthetic fixture)").FontSize(fontTitle).Bold()
	ui.Text(c, "Only a dedicated, pre-indexed single-line test document is supported. No workspace files are accessed.")
	ui.Text(c, state)
	ui.Row(c).Gap(spaceControl).Wrap().Children(func() {
		ui.Button(c, "Connect test fixture").Disabled(!enabled || s.window == nil || (state != "Disconnected" && state != "Connection failed")).OnClick(func() {
			if s.window == nil || os.Getenv("MYGO_XERJ_PANEL") != "1" {
				return
			}
			endpoint, index := os.Getenv("MYGO_XERJ_ENDPOINT"), os.Getenv("MYGO_XERJ_INDEX")
			session.owner.start(func(ctx context.Context) (searchRunner, error) {
				return connectSearchFixture(ctx, endpoint, index)
			}, s.window.Invalidate)
		})
		ui.Button(c, "Disconnect search").Disabled(state != "Connected" && state != "Connecting").OnClick(func() {
			session.owner.stop()
			session.query = ""
		})
	})
	if !enabled {
		ui.Text(c, "Disabled by default. See the XERJ example README for fixture configuration.")
	}
	if controller == nil || !enabled {
		return
	}
	ui.Row(c).Gap(spaceControl).Children(func() {
		input := ui.TextInput(c, &session.query).Label("Fixture query").Placeholder("Search the test document").Grow(1)
		clicked := ui.Button(c, "Search fixture").Clicked()
		if input.Submitted() || clicked {
			controller.Submit(session.query, 10)
		}
		ui.Button(c, "Cancel search").Disabled(controller.Snapshot().State != xerj.SearchLoading).OnClick(controller.Cancel)
	})
	result := controller.Snapshot()
	ui.Text(c, fmt.Sprintf("%s · %d results", result.State, len(result.Hits)))
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Gap(spaceControl).Children(func() {
			for i, hit := range result.Hits {
				ui.Column(c.Key(i)).Gap(4).Children(func() {
					ui.Text(c, searchHitLocation(hit)).Bold()
					ui.Text(c, hit.Text)
				})
			}
		})
	})
}

func searchHitLocation(hit xerj.Hit) string {
	if hit.Line > 0 {
		return fmt.Sprintf("%s:%d", hit.Path, hit.Line)
	}
	return hit.Path
}
