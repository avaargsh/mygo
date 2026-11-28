package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/plugins/xerj"
)

// The native view owns all mutable fields. The worker only receives a copy
// of the request and sends its result through a one-shot channel.
type searchSession struct {
	endpoint, index, query string
	cancel                 context.CancelFunc
	pending                <-chan searchResult
	draining               <-chan searchResult
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
	f.collectDrain()
	if f.pending != nil || f.draining != nil {
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
		hits, err := client.Search(ctx, req.query, 10)
		client.CloseIdleConnections()
		// Completion is sent only after HTTP cleanup, including cancellation.
		done <- searchResult{request: req, hits: hits, err: err}
		close(done)
		if wake != nil {
			wake()
		}
	}()
}

// collectDrain drops canceled results without publishing them. The worker
// retains its admission slot until it signals that HTTP cleanup is complete.
func (f *searchSession) collectDrain() {
	select {
	case <-f.draining:
		f.draining = nil
	default:
	}
}

func (f *searchSession) collect() {
	f.collectDrain()
	select {
	case result := <-f.pending:
		if f.cancel != nil {
			f.cancel()
		}
		f.cancel, f.pending = nil, nil
		f.result = &result
	default:
	}
}

func (f *searchSession) stop() {
	if f.cancel != nil {
		f.cancel()
	}
	if f.pending != nil {
		f.draining = f.pending
	}
	f.cancel, f.pending, f.result = nil, nil, nil
}
