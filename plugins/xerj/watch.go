package xerj

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// WatchTextChanges polls an explicitly approved, fixed file list off the UI
// thread. baseline must be a successful capture for the same trusted corpus
// and the currently accepted source revision. A detected change immediately
// invalidates the binding; notify runs after stableSamples unchanged follow-up
// samples. It receives a digest snapshot, not permission to publish or replay.
// Only one application-owned watcher per workspace is supported. The caller
// must stop and join it before changing authorization or replacing the scope.
func (m *Manager) WatchTextChanges(ctx context.Context, workspace, root string, paths []string, baseline TextSnapshot, interval time.Duration, stableSamples int, notify func(context.Context, TextSnapshot) error) error {
	if ctx == nil || notify == nil || interval < 100*time.Millisecond || interval > time.Minute || stableSamples < 2 || stableSamples > 10 || len(paths) == 0 || len(paths) > 32 || len(baseline.digests) != len(paths) {
		return errors.New("invalid text watch configuration")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	approved := append([]string(nil), paths...)
	seen := make(map[string]bool, len(approved))
	for _, p := range approved {
		if _, ok := baseline.digests[p]; !ok || seen[p] {
			return errors.New("baseline does not match approved paths")
		}
		seen[p] = true
	}
	if _, err := m.captureRevision(workspace); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	return m.watchSnapshots(ctx, workspace, baseline, stableSamples, ticker.C, func(ctx context.Context) (TextSnapshot, error) { return CaptureTextSnapshot(ctx, root, approved) }, notify)
}

func (m *Manager) watchSnapshots(ctx context.Context, workspace string, previous TextSnapshot, stableSamples int, ticks <-chan time.Time, read func(context.Context) (TextSnapshot, error), notify func(context.Context, TextSnapshot) error) error {
	dirty := false
	stable := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		next, err := read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// A missing/unreadable approved file is not an accepted deletion.
			return errors.Join(fmt.Errorf("watch capture: %w", err), m.Invalidate(workspace))
		}
		if previous.Diff(next).Changed() {
			if err := m.Invalidate(workspace); err != nil {
				return err
			}
			dirty = true
			stable = 0
		} else if dirty {
			stable++
			if stable >= stableSamples {
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := notify(ctx, next); err != nil {
					return fmt.Errorf("watch notification: %w", err)
				}
				dirty = false
				stable = 0
			}
		}
		previous = next
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticks:
		}
	}
}
