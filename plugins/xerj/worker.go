package xerj

import (
	"context"
	"errors"
	"strings"
	"sync"
)

var (
	ErrBuildDuplicate  = errors.New("workspace or prefix already queued or running")
	ErrBuildQueueFull  = errors.New("index build queue full")
	ErrBuildWorkerBusy = errors.New("index build worker busy")
)

type BuildState string

const (
	BuildQueued    BuildState = "QUEUED"
	BuildRunning   BuildState = "RUNNING"
	BuildCanceling BuildState = "CANCELING"
	BuildSucceeded BuildState = "SUCCEEDED"
	BuildFailed    BuildState = "FAILED"
	BuildCanceled  BuildState = "CANCELED"
)

// BuildQueue is an application-owned, process-local FIFO with one executing
// job at a time. Capacity counts queued and active jobs. Do not copy it.
// It does not own a goroutine; the application drives RunNext off the UI thread.
type BuildQueue struct {
	mu       sync.Mutex
	manager  *Manager
	capacity int
	pending  []*BuildJob
	active   *BuildJob
}

// BuildJob is an opaque handle. Terminal jobs are not retained by the queue.
// Callers may retain handles to observe completion. Do not copy it.
type BuildJob struct {
	queue             *BuildQueue
	workspace, prefix string
	run               func(context.Context) error
	state             BuildState
	cancel            context.CancelFunc
	done              chan struct{}
	err               error
}

func NewBuildQueue(manager *Manager, capacity int) (*BuildQueue, error) {
	if manager == nil || capacity < 1 || capacity > 64 {
		return nil, errors.New("invalid build queue configuration")
	}
	return &BuildQueue{manager: manager, capacity: capacity}, nil
}

// Submit copies the bounded expected digest map. The trusted application must
// keep the staged corpus and pinned executable immutable until completion.
// A queued job does not invalidate the current ready binding until it runs.
func (q *BuildQueue) Submit(workspace string, config AutoindexConfig, expected map[string]TextIndexVerifier) (*BuildJob, error) {
	if len(expected) < 1 || len(expected) > 8 {
		return nil, errors.New("invalid expected dataset count")
	}
	snapshot := make(map[string]TextIndexVerifier, len(expected))
	for name, v := range expected {
		if !indexName.MatchString(name) || !strings.HasPrefix(name, config.Prefix+"-") || len(v.Expected) < 1 || len(v.Expected) > 32 {
			return nil, errors.New("invalid expected dataset")
		}
		digests := make(map[string]string, len(v.Expected))
		for path, digest := range v.Expected {
			if len(path) > 4096 || len(digest) != 64 {
				return nil, errors.New("invalid source digest")
			}
			digests[path] = digest
		}
		snapshot[name] = TextIndexVerifier{Expected: digests}
	}
	return q.submit(workspace, config.Prefix, func(ctx context.Context) error { return q.manager.BuildTextSet(ctx, workspace, config, snapshot) })
}

func (q *BuildQueue) submit(workspace, prefix string, run func(context.Context) error) (*BuildJob, error) {
	if strings.TrimSpace(workspace) == "" || len(workspace) > 256 || !autoindexPrefix.MatchString(prefix) || run == nil {
		return nil, errors.New("invalid build request")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	duplicate := func(j *BuildJob) bool { return j.workspace == workspace || j.prefix == prefix }
	n := len(q.pending)
	if q.active != nil {
		n++
		if duplicate(q.active) {
			return nil, ErrBuildDuplicate
		}
	}
	for _, j := range q.pending {
		if duplicate(j) {
			return nil, ErrBuildDuplicate
		}
	}
	if n >= q.capacity {
		return nil, ErrBuildQueueFull
	}
	j := &BuildJob{queue: q, workspace: workspace, prefix: prefix, run: run, state: BuildQueued, done: make(chan struct{})}
	q.pending = append(q.pending, j)
	return j, nil
}

// RunNext processes one queued job synchronously. A canceled caller leaves
// queued work untouched. Running cancellation is cooperative; success may
// already have been committed before cancellation is observed.
func (q *BuildQueue) RunNext(ctx context.Context) (bool, error) {
	if ctx == nil {
		return false, errors.New("context required")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	q.mu.Lock()
	if q.active != nil {
		q.mu.Unlock()
		return false, ErrBuildWorkerBusy
	}
	if len(q.pending) == 0 {
		q.mu.Unlock()
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		q.mu.Unlock()
		return false, err
	}
	j := q.pending[0]
	q.pending[0] = nil
	q.pending = q.pending[1:]
	jobctx, cancel := context.WithCancel(ctx)
	j.cancel = cancel
	j.state = BuildRunning
	q.active = j
	q.mu.Unlock()
	err := j.run(jobctx)
	cancel()
	q.mu.Lock()
	defer q.mu.Unlock()
	state := BuildSucceeded
	if err != nil {
		state = BuildFailed
		if errors.Is(err, context.Canceled) && !errors.Is(err, ErrManifestUnavailable) {
			state = BuildCanceled
		}
	}
	j.finishLocked(state, err)
	q.active = nil
	return true, err
}

func (j *BuildJob) finishLocked(state BuildState, err error) {
	j.state = state
	j.err = err
	j.run = nil
	j.cancel = nil
	close(j.done)
}
func (j *BuildJob) Status() BuildState { j.queue.mu.Lock(); defer j.queue.mu.Unlock(); return j.state }

// Cancel removes a queued job or requests cancellation of a running job.
// A true result acknowledges the request, not rollback of published state.
func (j *BuildJob) Cancel() bool {
	q := j.queue
	q.mu.Lock()
	defer q.mu.Unlock()
	switch j.state {
	case BuildQueued:
		for i, p := range q.pending {
			if p == j {
				copy(q.pending[i:], q.pending[i+1:])
				q.pending[len(q.pending)-1] = nil
				q.pending = q.pending[:len(q.pending)-1]
				j.finishLocked(BuildCanceled, context.Canceled)
				return true
			}
		}
	case BuildRunning:
		j.state = BuildCanceling
		j.cancel()
		return true
	}
	return false
}

// Wait observes the build outcome without canceling the job if the wait expires.
func (j *BuildJob) Wait(ctx context.Context) error {
	if ctx == nil {
		return errors.New("context required")
	}
	select {
	case <-j.done:
		return j.err
	default:
	}
	select {
	case <-j.done:
		return j.err
	case <-ctx.Done():
		return ctx.Err()
	}
}
