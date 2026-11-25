package xerj

import (
	"context"
	"errors"
	"strings"
)

// SearchState is a stable presentation state, not an engine error message.
type SearchState string

const (
	SearchDisabled    SearchState = "DISABLED"
	SearchInvalid     SearchState = "INVALID_REQUEST"
	SearchNotReady    SearchState = "NOT_READY"
	SearchStale       SearchState = "STALE"
	SearchUnavailable SearchState = "UNAVAILABLE"
	SearchCanceled    SearchState = "CANCELED"
	SearchTimeout     SearchState = "TIMEOUT"
	SearchMatches     SearchState = "MATCHES"
	SearchEmpty       SearchState = "EMPTY"
)

// WorkspaceSearchResult is a presentation DTO. Hits are untrusted metadata
// and plain text, not file-open authorization or executable instructions.
type WorkspaceSearchResult struct {
	State SearchState `json:"state"`
	Hits  []Hit       `json:"hits"`
}

// WorkspaceSearchService binds read-only search to one application-approved
// workspace. It does not authorize callers, own engine state, or schedule
// work. Create and discard it with the trusted workspace lifecycle. Run Search
// off the UI thread. The caller must discard responses from superseded UI
// requests and recheck authorization before displaying or opening a source.
type WorkspaceSearchService struct {
	workspace string
	search    func(context.Context, string, string, int) ([]Hit, error)
}

// NewWorkspaceSearchService takes only a trusted workspace identity. A nil
// manager explicitly disables the feature; there is no fallback provider.
func NewWorkspaceSearchService(manager *Manager, workspace string) (*WorkspaceSearchService, error) {
	if strings.TrimSpace(workspace) == "" || len(workspace) > 256 {
		return nil, errors.New("invalid workspace identity")
	}
	s := &WorkspaceSearchService{workspace: workspace}
	if manager != nil {
		s.search = manager.Search
	}
	return s, nil
}

// Search accepts only query text and a bounded result count. It returns stable
// UI states without exposing raw engine errors. EMPTY is successful retrieval
// with no hits; failures and canceled/superseded reads never return partial hits.
func (s *WorkspaceSearchService) Search(ctx context.Context, query string, limit int) WorkspaceSearchResult {
	reply := WorkspaceSearchResult{State: SearchDisabled, Hits: []Hit{}}
	if s == nil || s.search == nil {
		return reply
	}
	if ctx == nil || strings.TrimSpace(query) == "" || len(query) > 4096 || limit < 1 || limit > 20 {
		reply.State = SearchInvalid
		return reply
	}
	if err := ctx.Err(); err != nil {
		reply.State = searchErrorState(err)
		return reply
	}
	hits, err := s.search(ctx, s.workspace, query, limit)
	if canceled := ctx.Err(); canceled != nil {
		reply.State = searchErrorState(canceled)
		return reply
	}
	if err != nil {
		reply.State = searchErrorState(err)
		return reply
	}
	if len(hits) > limit {
		reply.State = SearchUnavailable
		return reply
	}
	if len(hits) == 0 {
		reply.State = SearchEmpty
		return reply
	}
	reply.State = SearchMatches
	reply.Hits = append(reply.Hits, hits...)
	return reply
}

func searchErrorState(err error) SearchState {
	switch {
	case errors.Is(err, context.Canceled):
		return SearchCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return SearchTimeout
	case errors.Is(err, ErrManifestUnavailable):
		return SearchUnavailable
	case errors.Is(err, ErrStaleGeneration):
		return SearchStale
	case errors.Is(err, ErrIndexNotReady):
		return SearchNotReady
	default:
		return SearchUnavailable
	}
}
