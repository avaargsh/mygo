package main

import (
	"fmt"
	"strconv"

	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/plugins/xerj"
	"github.com/egoist/mygo/ui"
)

func (s *shell) searchSession() *searchSession {
	if c := s.content.Session("search"); c != nil {
		f, _ := c.Value.(*searchSession)
		return f
	}
	return nil
}

func searchSource(hit xerj.Hit) string {
	path := strconv.QuoteToGraphic(hit.Path)
	if hit.Line > 0 {
		return fmt.Sprintf("%s:%d", path, hit.Line)
	}
	return path + " (line unavailable)"
}

func (s *shell) renderSearch(c *ui.Context, snap core.WorkspaceSnapshot, paneID string, f *searchSession) {
	if core.SessionOwner(snap, "search") != paneID {
		ui.Text(c, "Search is displayed in another pane.")
		ui.Button(c, "Show search here").OnClick(func() { s.apply(s.workspace.Focus(paneID)) })
		return
	}
	f.collect()
	ui.Text(c, "Search local documents").FontSize(fontTitle).Bold()
	ui.Text(c, "Use an existing XERJ index on this computer. Only the search text is sent to the selected local node.")
	if f.draining != nil {
		ui.Text(c, "Canceling search…")
		return
	}
	if f.pending != nil {
		ui.Text(c, "Searching…")
		ui.Button(c, "Cancel search").OnClick(f.stop)
		return
	}
	ui.TextInput(c.Key("search-endpoint"), &f.endpoint).Label("Local XERJ endpoint").MaxLength(256)
	ui.TextInput(c.Key("search-index"), &f.index).Label("Exact index name").Placeholder("Enter the index slug").MaxLength(128)
	ui.Row(c).Gap(spaceControl).Children(func() {
		ui.TextInput(c.Key("search-query"), &f.query).Label("Search text").Placeholder("Enter keywords").MaxLength(4096).Grow(1)
		ui.Button(c, "Find passages").Disabled(f.request().query == "" || f.request().index == "").OnClick(func() {
			window := s.window
			var wake func()
			if window != nil {
				wake = window.Invalidate
			}
			f.start(wake)
		})
	})
	r := f.result
	if r == nil || r.request != f.request() {
		return
	}
	if r.err != nil {
		ui.Text(c, "Search failed: "+r.err.Error())
		return
	}
	if len(r.hits) == 0 {
		ui.Text(c, "No matching passages.")
		return
	}
	ui.Text(c, fmt.Sprintf("%d passages · %s", len(r.hits), r.request.index)).FontSize(fontSmall)
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Gap(spaceControl).Children(func() {
			for i, hit := range r.hits {
				ui.Column(c.Key(strconv.Itoa(i))).Gap(4).Children(func() {
					ui.Text(c, searchSource(hit)).Bold()
					ui.Text(c, hit.Text)
				})
			}
		})
	})
}
