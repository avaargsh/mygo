package xerj

import (
	"context"
	"errors"
	"time"
)

// signalLocked coalesces wakeups. The receiver always checks actual queue state.
func (q *BuildQueue) signalLocked() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// Serve consumes queued jobs until ctx is canceled or a persistence fault
// occurs. The application owns this blocking loop and its lifetime. Only one
// Serve or manual RunNext can own dispatch at a time, including while idle.
// Ordinary build errors are retained by their handles; no automatic retries
// occur. jobTimeout bounds each pipeline cooperatively, not filesystem I/O.
// Stopping cancels active work and leaves pending jobs queued in memory.
func (q *BuildQueue) Serve(ctx context.Context, jobTimeout time.Duration) error {
	if ctx == nil || jobTimeout <= 0 || jobTimeout > 30*time.Minute {
		return errors.New("context and job timeout in (0, 30m] required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	q.mu.Lock()
	if q.serving || q.active != nil {
		q.mu.Unlock()
		return ErrBuildWorkerBusy
	}
	q.serving = true
	q.mu.Unlock()
	defer func() { q.mu.Lock(); q.serving = false; q.mu.Unlock() }()
	for {
		processed, err := q.runNext(ctx, true, jobTimeout)
		if errors.Is(err, ErrBuildJournalUnavailable) || errors.Is(err, ErrManifestUnavailable) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !processed && err != nil {
			return err
		}
		if processed {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-q.wake:
		}
	}
}
