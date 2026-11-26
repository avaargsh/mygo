package xerj

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestWorkspaceSearchStates(t *testing.T) {
	fixture := Hit{Index: "approved-code", Path: "src/a.go", Line: 3, Text: "needle"}
	for _, tt := range []struct {
		name string
		hits []Hit
		err  error
		want SearchState
	}{
		{"matches", []Hit{fixture}, nil, SearchMatches},
		{"empty", nil, nil, SearchEmpty},
		{"not ready", nil, ErrIndexNotReady, SearchNotReady},
		{"stale", []Hit{fixture}, ErrStaleGeneration, SearchStale},
		{"manifest fault", nil, ErrManifestUnavailable, SearchUnavailable},
		{"backend fault", []Hit{fixture}, errors.New("private diagnostic"), SearchUnavailable},
		{"canceled", nil, context.Canceled, SearchCanceled},
		{"deadline", nil, context.DeadlineExceeded, SearchTimeout},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &WorkspaceSearchService{workspace: "approved", search: func(_ context.Context, workspace, query string, limit int) ([]Hit, error) {
				if workspace != "approved" || query != "needle" || limit != 5 {
					t.Fatal("scope or request changed")
				}
				return tt.hits, tt.err
			}}
			got := s.Search(context.Background(), "needle", 5)
			if got.State != tt.want || (len(got.Hits) == 1) != (tt.want == SearchMatches) || got.Hits == nil {
				t.Fatalf("reply %+v", got)
			}
			if tt.want == SearchMatches && got.Hits[0] != fixture {
				t.Fatal(got.Hits)
			}
			body, err := json.Marshal(got)
			if err != nil || strings.Contains(string(body), "private diagnostic") || strings.Contains(string(body), `"hits":null`) {
				t.Fatalf("wire %s %v", body, err)
			}
		})
	}
}

func TestWorkspaceSearchDisabledAndInvalid(t *testing.T) {
	disabled, err := NewWorkspaceSearchService(nil, "approved")
	if err != nil || disabled.Search(context.Background(), "needle", 5).State != SearchDisabled {
		t.Fatalf("disabled: %v %v", disabled, err)
	}
	if _, err := NewWorkspaceSearchService(nil, " "); err == nil {
		t.Fatal("blank workspace accepted")
	}
	s := &WorkspaceSearchService{workspace: "approved", search: func(context.Context, string, string, int) ([]Hit, error) {
		t.Fatal("invalid input dispatched")
		return nil, nil
	}}
	for _, request := range []struct {
		query string
		limit int
	}{{" ", 5}, {strings.Repeat("x", 4097), 5}, {"needle", 0}, {"needle", 21}} {
		if got := s.Search(context.Background(), request.query, request.limit); got.State != SearchInvalid {
			t.Fatal(got)
		}
	}
	if got := s.Search(nil, "needle", 5); got.State != SearchInvalid {
		t.Fatal(got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := s.Search(ctx, "needle", 5); got.State != SearchCanceled {
		t.Fatal(got)
	}
}

func TestWorkspaceSearchDropsCanceledReply(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &WorkspaceSearchService{workspace: "approved", search: func(context.Context, string, string, int) ([]Hit, error) {
		cancel()
		return []Hit{{Path: "a", Text: "needle"}}, nil
	}}
	if got := s.Search(ctx, "needle", 5); got.State != SearchCanceled || len(got.Hits) != 0 {
		t.Fatal(got)
	}
}
