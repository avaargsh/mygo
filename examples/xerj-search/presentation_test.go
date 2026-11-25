package main

import "testing"

func TestSearchStatusText(t *testing.T) {
	for _, tc := range []struct {
		state string
		count int
		want  string
	}{
		{"IDLE", 0, "Enter a query to search the fixture."},
		{"LOADING", 0, "Searching…"},
		{"DISABLED", 0, "Search is disabled."},
		{"INVALID_REQUEST", 0, "Enter a non-empty query of at most 4096 bytes."},
		{"NOT_READY", 0, "Index is not ready."},
		{"STALE", 0, "Index changed. Search again."},
		{"UNAVAILABLE", 0, "Search service is unavailable."},
		{"CANCELED", 0, "Search canceled."},
		{"TIMEOUT", 0, "Search timed out."},
		{"EMPTY", 0, "No matching results."},
		{"MATCHES", 1, "1 match"},
		{"MATCHES", 2, "2 matches"},
		{"MATCHES", 0, "Search service is unavailable."},
		{"unknown", 0, "Search service is unavailable."},
	} {
		if got := searchStatusText(tc.state, tc.count); got != tc.want {
			t.Errorf("%s/%d: got %q, want %q", tc.state, tc.count, got, tc.want)
		}
	}
}

func TestSourceLabel(t *testing.T) {
	for _, tc := range []struct {
		path string
		line int
		want string
	}{
		{"src/a.go", 3, "src/a.go:3"},
		{"src/a.go", 0, "src/a.go"},
		{"a\nb\u202e", 0, "a�b�"},
		{"", 1, "Unknown source:1"},
	} {
		if got := sourceLabel(tc.path, tc.line); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}
