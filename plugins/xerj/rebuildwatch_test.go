package xerj

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRebuildNoticeDeferredUntilStable(t *testing.T) {
	m := NewManager("http://127.0.0.1:9200")
	old := TextSnapshot{digests: map[string]string{"a.txt": "old"}}
	next := TextSnapshot{digests: map[string]string{"a.txt": "new"}}
	ticks := make(chan time.Time, 5)
	for i := 0; i < 5; i++ {
		ticks <- time.Time{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	err := m.watchSnapshots(ctx, "a", old, 2, ticks, func(context.Context) (TextSnapshot, error) { return next, nil }, func(context.Context, TextSnapshot) error {
		calls++
		if calls == 1 {
			return errRebuildDeferred
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || calls != 2 {
		t.Fatalf("%v calls %d", err, calls)
	}
}

func TestRebuildSubmissionRejectsChangedSnapshotAndDefersBusy(t *testing.T) {
	source, stage, state := t.TempDir(), t.TempDir(), t.TempDir()
	path := filepath.Join(source, "a.txt")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	baseline, err := CaptureTextSnapshot(context.Background(), source, []string{"a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager("http://127.0.0.1:9200")
	q, _ := NewBuildQueue(m, 1)
	config := AutoindexConfig{StateRoot: state}
	datasets := map[string][]string{"code": {"a.txt"}}
	busy, err := q.submit("other", "busy", func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.submitRebuildNotice(context.Background(), "a", config, source, stage, datasets, baseline); !errors.Is(err, errRebuildDeferred) {
		t.Fatalf("busy %v", err)
	}
	busy.Cancel()
	if err = os.WriteFile(path, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = q.submitRebuildNotice(context.Background(), "a", config, source, stage, datasets, baseline); !errors.Is(err, errRebuildDeferred) {
		t.Fatalf("changed %v", err)
	}
	entries, err := os.ReadDir(stage)
	if err != nil || len(entries) != 0 {
		t.Fatalf("leaked %v %v", entries, err)
	}
	fresh, err := CaptureTextSnapshot(context.Background(), source, []string{"a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	job, err := q.submitRebuildNotice(context.Background(), "a", config, source, stage, datasets, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status() != BuildQueued || !job.Cancel() {
		t.Fatal(job.Status())
	}
	another, err := q.submitRebuildNotice(context.Background(), "a", config, source, stage, datasets, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if another.prefix == job.prefix {
		t.Fatal("prefix reused")
	}
	another.Cancel()
}

func TestWatchAndRebuildTextQueuesObservedChange(t *testing.T) {
	source, stage, state := t.TempDir(), t.TempDir(), t.TempDir()
	path := filepath.Join(source, "a.txt")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	baseline, err := CaptureTextSnapshot(context.Background(), source, []string{"a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager("http://127.0.0.1:9200")
	q, _ := NewBuildQueue(m, 1)
	ticket, err := m.Begin("a", "old-code")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Publish(ticket); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var job *BuildJob
	err = q.WatchAndRebuildText(ctx, "a", AutoindexConfig{StateRoot: state}, source, stage, map[string][]string{"code": {"a.txt"}}, baseline, 100*time.Millisecond, 2, func(j *BuildJob) { job = j; cancel() })
	if !errors.Is(err, context.Canceled) || job == nil || job.Status() != BuildQueued || m.Status("a").State != IndexFailed {
		t.Fatalf("job %v status %v err %v", job, m.Status("a"), err)
	}
	if !job.Cancel() {
		t.Fatal("could not cancel queued job")
	}
}
