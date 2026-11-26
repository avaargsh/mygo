package xerj

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchCoalescesBurstAndFencesEachChange(t *testing.T) {
	m := NewManager("http://127.0.0.1:9200")
	ticket, err := m.Begin("a", "old-code")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Publish(ticket); err != nil {
		t.Fatal(err)
	}
	baseline := TextSnapshot{digests: map[string]string{"a.txt": "old"}}
	first := TextSnapshot{digests: map[string]string{"a.txt": "first"}}
	last := TextSnapshot{digests: map[string]string{"a.txt": "last"}}
	samples := []TextSnapshot{first, first, last, last, last}
	ticks := make(chan time.Time, len(samples))
	for range samples {
		ticks <- time.Time{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reads, notifications := 0, 0
	var stale sourceTicket
	err = m.watchSnapshots(ctx, "a", baseline, 2, ticks, func(context.Context) (TextSnapshot, error) {
		if reads == 1 {
			if m.Status("a").State != IndexFailed {
				t.Fatal("first observation did not invalidate")
			}
			stale, _ = m.captureRevision("a")
		}
		if reads == 3 {
			current, _ := m.captureRevision("a")
			if stale.revision == current.revision {
				t.Fatal("second change did not fence new tasks")
			}
		}
		if reads >= len(samples) {
			t.Fatal("notification did not occur")
		}
		s := samples[reads]
		reads++
		return s, nil
	}, func(_ context.Context, s TextSnapshot) error {
		notifications++
		if reads != 5 || s.Digests()["a.txt"] != "last" {
			t.Fatalf("premature notice after %d samples: %v", reads, s.Digests())
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || notifications != 1 {
		t.Fatalf("%v notifications=%d", err, notifications)
	}
}

func TestWatchApprovedFileChange(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "a.txt")
	if err := os.WriteFile(p, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	baseline, err := CaptureTextSnapshot(context.Background(), root, []string{"a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(p, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	m := NewManager("http://127.0.0.1:9200")
	ticket, err := m.Begin("a", "old-code")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Publish(ticket); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	called := false
	err = m.WatchTextChanges(ctx, "a", root, []string{"a.txt"}, baseline, 100*time.Millisecond, 2, func(_ context.Context, s TextSnapshot) error {
		called = true
		if !baseline.Diff(s).Changed() || m.Status("a").State != IndexFailed {
			t.Fatal("missing change or fence")
		}
		cancel()
		return nil
	})
	if !called || !errors.Is(err, context.Canceled) {
		t.Fatalf("called=%t err=%v", called, err)
	}
}

func TestWatchCaptureFailureInvalidatesAndStops(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "a.txt")
	if err := os.WriteFile(p, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	baseline, err := CaptureTextSnapshot(context.Background(), root, []string{"a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(p); err != nil {
		t.Fatal(err)
	}
	m := NewManager("http://127.0.0.1:9200")
	ticket, err := m.Begin("a", "old-code")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Publish(ticket); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = m.WatchTextChanges(ctx, "a", root, []string{"a.txt"}, baseline, 100*time.Millisecond, 2, func(context.Context, TextSnapshot) error { t.Error("unexpected notice"); return nil })
	if err == nil || m.Status("a").State != IndexFailed {
		t.Fatalf("%v %v", err, m.Status("a"))
	}
}
