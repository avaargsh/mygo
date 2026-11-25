package xerj

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestWorkspaceSearchServiceManager(t *testing.T) {
	var mode atomic.Int32
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/approved-code/_search" {
			t.Errorf("unexpected index request: %s", r.URL.Path)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch mode.Load() {
		case 1:
			fmt.Fprint(w, `{"hits":{"hits":[]}}`)
		case 2:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			fmt.Fprint(w, `{"hits":{"hits":[{"_index":"approved-code","_source":{"ax_path":"src/a.go","line":3},"fields":{"_passage":{"text":"needle","start_offset":0}}}]}}`)
		}
	}))
	defer server.Close()
	m := NewManager(server.URL)
	s, err := NewWorkspaceSearchService(m, "approved")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	check := func(want SearchState, n int) {
		t.Helper()
		got := s.Search(ctx, "needle", 5)
		if got.State != want || len(got.Hits) != n {
			t.Fatalf("want %s/%d, got %+v", want, n, got)
		}
	}
	check(SearchNotReady, 0)
	ticket, err := m.Begin("approved", "approved-code")
	if err != nil {
		t.Fatal(err)
	}
	check(SearchNotReady, 0)
	if err := m.Publish(ticket); err != nil {
		t.Fatal(err)
	}
	other, err := m.Begin("other", "other-code")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Publish(other); err != nil {
		t.Fatal(err)
	}
	check(SearchMatches, 1)
	mode.Store(1)
	check(SearchEmpty, 0)
	mode.Store(2)
	check(SearchUnavailable, 0)
	if err := m.Invalidate("approved"); err != nil {
		t.Fatal(err)
	}
	check(SearchNotReady, 0)
	if calls.Load() != 3 {
		t.Fatalf("unexpected search request count %d", calls.Load())
	}
}
