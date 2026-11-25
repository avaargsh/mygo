package xerj

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
)

// SubmitApprovedText creates a queue-owned stage from an explicitly approved
// partition of source paths by exact dataset name. The caller must exclusively
// control the roots, node and pinned binary, and keep the staged tree unchanged.
// The stage is removed at job completion (including cancellation), not on a
// cancellation request while running. No crash-orphan collection is performed.
func (q *BuildQueue) SubmitApprovedText(ctx context.Context, workspace string, config AutoindexConfig, sourceRoot, stagingRoot string, datasets map[string][]string) (*BuildJob, error) {
	return q.submitApprovedText(ctx, workspace, config, sourceRoot, stagingRoot, datasets, nil)
}

func (q *BuildQueue) submitApprovedText(ctx context.Context, workspace string, config AutoindexConfig, sourceRoot, stagingRoot string, datasets map[string][]string, observed *TextSnapshot) (job *BuildJob, err error) {
	if ctx == nil || config.Corpus != "" || !autoindexPrefix.MatchString(config.Prefix) || len(datasets) < 1 || len(datasets) > 8 {
		return nil, errors.New("invalid approved build scope")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	revision, err := q.manager.captureRevision(workspace)
	if err != nil {
		return nil, err
	}
	parent, err := resolveDirectory(stagingRoot)
	if err != nil {
		return nil, err
	}
	state, err := resolveDirectory(config.StateRoot)
	if err != nil {
		return nil, err
	}
	if containsPath(parent, state) || containsPath(state, parent) {
		return nil, errors.New("staging and CLI state roots must be disjoint")
	}
	owners := make(map[string]string)
	paths := make([]string, 0)
	for name, files := range datasets {
		if !indexName.MatchString(name) || !strings.HasPrefix(name, config.Prefix+"-") || name == config.Prefix+"-" || len(files) < 1 || len(files) > 32 {
			return nil, errors.New("invalid approved dataset")
		}
		for _, p := range files {
			if _, exists := owners[p]; exists || len(paths) >= 32 {
				return nil, errors.New("duplicate source assignment or too many files")
			}
			owners[p] = name
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	stage, err := StageTextCorpus(ctx, sourceRoot, parent, paths)
	if err != nil {
		return nil, err
	}
	cleanup := func() error { return os.RemoveAll(stage.Root()) }
	defer func() {
		if job == nil {
			if cleanupErr := cleanup(); cleanupErr != nil {
				err = errors.Join(err, cleanupErr)
			}
		}
	}()
	if observed != nil && observed.Diff(stage.snapshot).Changed() {
		return nil, errRebuildDeferred
	}
	config.Corpus = stage.Root()
	config.StateRoot = state
	expected := make(map[string]TextIndexVerifier, len(datasets))
	for name := range datasets {
		expected[name] = TextIndexVerifier{Expected: make(map[string]string)}
	}
	for p, digest := range stage.Digests() {
		expected[owners[p]].Expected[p] = digest
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return q.submitOwned(workspace, config.Prefix, func(ctx context.Context) error {
		return q.manager.buildTextSetAt(ctx, workspace, config.Prefix, expected, config.Run, revision)
	}, cleanup)
}

// CleanupError reports a terminal stage removal failure separately from the
// build outcome. Inspect after Wait/terminal status; a cleanup failure cannot
// roll back publication. It is nil for jobs without owned stages.
func (j *BuildJob) CleanupError() error {
	j.queue.mu.Lock()
	defer j.queue.mu.Unlock()
	return j.cleanupErr
}
