package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/plugins/xerj"
	"github.com/egoist/mygo/ui"
)

// The native view owns all mutable fields. The worker only receives a copy
// of the request and sends its result through a one-shot channel.
type searchSession struct {
	endpoint, index, query string
	cancel                 context.CancelFunc
	pending                <-chan searchResult
	result                 *searchResult
}

type searchRequest struct {
	endpoint, index, query string
}

type searchResult struct {
	request searchRequest
	hits    []xerj.Hit
	err     error
}

func (f *searchSession) request() searchRequest {
	return searchRequest{strings.TrimSpace(f.endpoint), strings.TrimSpace(f.index), strings.TrimSpace(f.query)}
}

func (f *searchSession) start(wake func()) {
	if f.pending != nil {
		return
	}
	req := f.request()
	f.result = nil
	if req.query == "" || len(req.query) > 4096 {
		f.result = &searchResult{request: req, err: fmt.Errorf("enter a query of at most 4096 bytes")}
		return
	}
	client, err := xerj.New(req.endpoint, req.index)
	if err != nil {
		f.result = &searchResult{request: req, err: err}
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan searchResult, 1)
	f.cancel, f.pending = cancel, done
	go func() {
		defer client.CloseIdleConnections()
		hits, err := client.Search(ctx, req.query, 10)
		done <- searchResult{request: req, hits: hits, err: err}
		if ctx.Err() == nil && wake != nil {
			wake()
		}
	}()
}

func (f *searchSession) collect() {
	select {
	case result := <-f.pending:
		f.stop()
		f.result = &result
	default:
	}
}

func (f *searchSession) stop() {
	if f.cancel != nil {
		f.cancel()
	}
	f.cancel, f.pending, f.result = nil, nil, nil
}

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
	if f.pending != nil {
		ui.Text(c, "Searching…")
		ui.Button(c, "Cancel search").OnClick(f.stop)
		return
	}
	ui.TextInput(c.Key("search-endpoint"), &f.endpoint).Label("Local XERJ endpoint").MaxLength(256)
	ui.TextInput(c.Key("search-index"), &f.index).Label("Exact index name").Placeholder("Enter the index slug").MaxLength(128)
	ui.Row(c).Gap(spaceControl).Children(func() {
		query := ui.TextInput(c.Key("search-query"), &f.query).Label("Search text").Placeholder("Enter keywords").MaxLength(4096).Grow(1)
		req := f.request()
		disabled := req.query == "" || req.index == ""
		clicked := ui.Button(c, "Find passages").Disabled(disabled).Clicked()
		if disabled || !(query.Submitted() || clicked) {
			return
		}
		var wake func()
		if window := s.window; window != nil {
			wake = window.Invalidate
		}
		f.start(wake)
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
