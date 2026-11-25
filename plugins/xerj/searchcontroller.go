package xerj

import (
	"context"
	"errors"
	"strings"
	"sync"
)

const (
	SearchIdle    SearchState = "IDLE"
	SearchLoading SearchState = "LOADING"
)

var ErrSearchWorkerBusy = errors.New("search worker already running")

type searchRequest struct {
	query string
	limit int
}

// SearchController is a single-workspace presentation controller. It retains
// at most one active and one latest pending request. Submit, Cancel, Close and
// Snapshot are safe to call on the UI thread; Serve performs search off it.
// It is not an authorization boundary. Do not copy after use.
type SearchController struct {
	mu               sync.Mutex
	service          *WorkspaceSearchService
	wake             chan struct{}
	serving, closed  bool
	current, pending *searchRequest
	cancel           context.CancelFunc
	result           WorkspaceSearchResult
}

func NewSearchController(service *WorkspaceSearchService) *SearchController {
	state := SearchIdle
	if service == nil || service.search == nil {
		state = SearchDisabled
	}
	return &SearchController{service: service, wake: make(chan struct{}, 1), result: WorkspaceSearchResult{State: state, Hits: []Hit{}}}
}

func (c *SearchController) signalLocked() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}
func (c *SearchController) clearLocked(state SearchState) {
	c.current, c.pending = nil, nil
	if c.cancel != nil {
		c.cancel()
	}
	c.result = WorkspaceSearchResult{State: state, Hits: []Hit{}}
}

// Submit replaces any previous pending request and cancels active work
// cooperatively. False means disabled or closed. Invalid input is presented
// immediately without dispatch. The caller requests a redraw after Submit.
func (c *SearchController) Submit(query string, limit int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.service == nil || c.service.search == nil {
		return false
	}
	if strings.TrimSpace(query) == "" || len(query) > 4096 || limit < 1 || limit > 20 {
		c.clearLocked(SearchInvalid)
		return true
	}
	c.clearLocked(SearchLoading)
	req := &searchRequest{query: query, limit: limit}
	c.current, c.pending = req, req
	c.signalLocked()
	return true
}

// Cancel discards queued and active results immediately; it does not wait for
// the HTTP call to return. The caller requests a redraw.
func (c *SearchController) Cancel() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current != nil {
		c.clearLocked(SearchCanceled)
	}
}

// Close permanently disables the controller and discards displayed hits.
// The owner must join Serve off the UI thread before releasing its resources.
func (c *SearchController) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.clearLocked(SearchDisabled)
	c.signalLocked()
}

// Snapshot returns a copy. Read it at render time, not from a queued callback
// carrying an old payload. The app must recheck workspace/source authorization.
func (c *SearchController) Snapshot() WorkspaceSearchResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	return WorkspaceSearchResult{State: c.result.State, Hits: append([]Hit{}, c.result.Hits...)}
}

// Serve runs one worker until Close or lifetime context cancellation. notify
// is an optional short, thread-safe redraw signal after accepted completion;
// it carries no result and executes outside the lock. Do not mutate UI state
// from it. A newer Submit/Close may occur before that redraw is handled.
// Context cancellation permanently closes the controller. No automatic retries.
func (c *SearchController) Serve(ctx context.Context, notify func()) error {
	if ctx == nil {
		return errors.New("context required")
	}
	c.mu.Lock()
	if c.serving {
		c.mu.Unlock()
		return ErrSearchWorkerBusy
	}
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.serving = true
	c.mu.Unlock()
	defer func() { c.Close(); c.mu.Lock(); c.serving = false; c.mu.Unlock() }()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return nil
		}
		req := c.pending
		if req == nil {
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-c.wake:
			}
			continue
		}
		c.pending = nil
		jobctx, cancel := context.WithCancel(ctx)
		c.cancel = cancel
		c.mu.Unlock()
		result := c.service.Search(jobctx, req.query, req.limit)
		cancel()
		c.mu.Lock()
		c.cancel = nil
		accepted := !c.closed && c.current == req && ctx.Err() == nil
		if accepted {
			c.result = result
			c.current = nil
		}
		c.mu.Unlock()
		if accepted && notify != nil {
			notify()
		}
	}
}
