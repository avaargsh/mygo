package xerj

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSearchControllerLatestWins(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	started := make(chan string, 3)
	release := make(chan struct{}, 1)
	defer close(release)
	s := &WorkspaceSearchService{workspace: "a", search: func(ctx context.Context, _, query string, _ int) ([]Hit, error) {
		started <- query
		if query == "first" {
			<-ctx.Done()
			<-release
			return []Hit{{Text: "obsolete"}}, nil
		}
		return []Hit{{Text: query}}, nil
	}}
	c := NewSearchController(s)
	wake := make(chan struct{}, 4)
	done := make(chan error, 1)
	go func() { done <- c.Serve(ctx, func() { wake <- struct{}{} }) }()
	if !c.Submit("first", 5) {
		t.Fatal("not accepted")
	}
	select {
	case q := <-started:
		if q != "first" {
			t.Fatal(q)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if !c.Submit("second", 5) || !c.Submit("latest", 5) {
		t.Fatal("not accepted")
	}
	if got := c.Snapshot(); got.State != SearchLoading || len(got.Hits) != 0 {
		t.Fatal(got)
	}
	// Release the old callback only after the replacement is installed.
	// A separate signal avoids relying on scheduling to verify coalescing.
	release <- struct{}{}
	select {
	case q := <-started:
		if q != "latest" {
			t.Fatal(q)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case <-wake:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	got := c.Snapshot()
	if got.State != SearchMatches || len(got.Hits) != 1 || got.Hits[0].Text != "latest" {
		t.Fatal(got)
	}
	got.Hits[0].Text = "mutated"
	if c.Snapshot().Hits[0].Text != "latest" {
		t.Fatal("snapshot alias")
	}
	c.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestSearchControllerCancelAndClose(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	started := make(chan struct{})
	stopped := make(chan struct{})
	s := &WorkspaceSearchService{workspace: "a", search: func(ctx context.Context, _, _ string, _ int) ([]Hit, error) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return nil, ctx.Err()
	}}
	c := NewSearchController(s)
	done := make(chan error, 1)
	go func() { done <- c.Serve(ctx, nil) }()
	c.Submit("needle", 5)
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := c.Serve(ctx, nil); !errors.Is(err, ErrSearchWorkerBusy) {
		t.Fatal(err)
	}
	c.Cancel()
	if got := c.Snapshot(); got.State != SearchCanceled || len(got.Hits) != 0 {
		t.Fatal(got)
	}
	select {
	case <-stopped:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	c.Close()
	if c.Submit("again", 5) || c.Snapshot().State != SearchDisabled {
		t.Fatal("closed controller accepted work")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestSearchControllerInvalidAndLifetime(t *testing.T) {
	s := &WorkspaceSearchService{workspace: "a", search: func(context.Context, string, string, int) ([]Hit, error) {
		t.Error("unexpected search")
		return nil, nil
	}}
	c := NewSearchController(s)
	if c.Snapshot().State != SearchIdle {
		t.Fatal(c.Snapshot())
	}
	if !c.Submit(strings.Repeat("x", 4097), 5) || c.Snapshot().State != SearchInvalid {
		t.Fatal(c.Snapshot())
	}
	if !c.Submit("valid", 5) {
		t.Fatal("queue rejected")
	}
	c.Cancel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Serve(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if c.Submit("again", 5) || c.Snapshot().State != SearchDisabled {
		t.Fatal("canceled lifetime not closed")
	}
	d := NewSearchController(nil)
	if d.Submit("needle", 5) || d.Snapshot().State != SearchDisabled {
		t.Fatal("disabled accepted work")
	}
}
