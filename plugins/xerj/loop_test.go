package xerj

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServeWakesAndContinuesAfterFailure(t *testing.T) {
	q, _ := NewBuildQueue(NewManager("http://127.0.0.1:9200"), 2)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- q.Serve(ctx, time.Second) }()
	boom := errors.New("build failed")
	a, err := q.submit("a", "one", func(context.Context) error { return boom })
	if err != nil {
		t.Fatal(err)
	}
	b, err := q.submit("b", "two", func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Wait(ctx); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if err := b.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestServeExclusiveAndStopPreservesPending(t *testing.T) {
	q, _ := NewBuildQueue(NewManager("http://127.0.0.1:9200"), 2)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	entered := make(chan struct{})
	a, _ := q.submit("a", "one", func(ctx context.Context) error { close(entered); <-ctx.Done(); return ctx.Err() })
	b, _ := q.submit("b", "two", func(context.Context) error { return nil })
	done := make(chan error, 1)
	go func() { done <- q.Serve(ctx, 3*time.Second) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := q.Serve(ctx, time.Second); !errors.Is(err, ErrBuildWorkerBusy) {
		t.Fatal(err)
	}
	if ok, err := q.RunNext(ctx); ok || !errors.Is(err, ErrBuildWorkerBusy) {
		t.Fatalf("%v %v", ok, err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if a.Status() != BuildCanceled || b.Status() != BuildQueued {
		t.Fatalf("%v %v", a.Status(), b.Status())
	}
	if ok, err := q.RunNext(context.Background()); !ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
}

func TestServeJobDeadlineAndInvalidConfiguration(t *testing.T) {
	q, _ := NewBuildQueue(NewManager("http://127.0.0.1:9200"), 2)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	a, _ := q.submit("a", "one", func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })
	b, _ := q.submit("b", "two", func(context.Context) error { return nil })
	if err := q.Serve(ctx, 0); err == nil || a.Status() != BuildQueued {
		t.Fatalf("invalid timeout: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- q.Serve(ctx, 50*time.Millisecond) }()
	if err := a.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if err := b.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	<-done
}

func TestServeStopsOnJournalFailure(t *testing.T) {
	dir := t.TempDir()
	m, err := CreatePersistentManager("http://127.0.0.1:9200", filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(dir, "state")
	if err = os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	q, err := CreateJournaledBuildQueue(m, 1, filepath.Join(state, "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(state, state+"-moved"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- q.Serve(ctx, time.Second) }()
	if _, err = q.submit("a", "one", func(context.Context) error { t.Error("unexpected run"); return nil }); !errors.Is(err, ErrBuildJournalUnavailable) {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, ErrBuildJournalUnavailable) {
		t.Fatal(err)
	}
}
