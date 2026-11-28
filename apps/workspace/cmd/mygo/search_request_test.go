package main

import (
	"context"
	"testing"

	"github.com/egoist/mygo/plugins/xerj"
)

func TestSearchCancelWaitsForWorkerCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	old := make(chan searchResult, 1)
	f := &searchSession{pending: old, cancel: cancel}
	f.stop()
	if ctx.Err() != context.Canceled || f.result != nil || f.pending != nil {
		t.Fatal("cancel did not clear visible state")
	}
	// A canceled worker has not yet sent completion. Even invalid new work
	// must not be admitted until cleanup finishes.
	f.start(nil)
	if f.result != nil || f.pending != nil {
		t.Fatal("replacement admitted before canceled worker completed")
	}
	f.stop() // Repeated cancel must not forget the outstanding worker.
	f.start(nil)
	if f.result != nil || f.pending != nil {
		t.Fatal("repeated cancel lost the drain fence")
	}
	old <- searchResult{hits: []xerj.Hit{{Text: "obsolete"}}}
	f.collect()
	if f.result != nil || f.pending != nil {
		t.Fatal("late result became visible")
	}
	f.start(nil) // Empty query now reaches normal validation, no engine I/O.
	if f.result == nil || f.result.err == nil {
		t.Fatal("new work remained blocked after cleanup")
	}
}

func TestSearchCollectCompletedRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan searchResult, 1)
	f := &searchSession{pending: done, cancel: cancel}
	done <- searchResult{hits: []xerj.Hit{{Text: "current"}}}
	f.collect()
	if f.pending != nil || f.result == nil || len(f.result.hits) != 1 || f.result.hits[0].Text != "current" || ctx.Err() != context.Canceled {
		t.Fatalf("completed result not collected: %+v", f.result)
	}
}
