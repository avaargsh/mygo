package xerj

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

var errRebuildDeferred = errors.New("rebuild admission deferred")

// WatchAndRebuildText composes the fixed approved-file watcher with queue
// admission. Dataset keys are approved suffixes, not full index names. Leave
// config.Prefix and config.Corpus empty. The caller owns both this loop and
// the worker loop, and must stop and join both before changing authorization.
// Only changes relative to the baseline trigger jobs; this is not initial
// provisioning or automatic retry of a failed build. onJob must return promptly.
// A stopped controller does not certify freshness; invalidate before relying
// on any subsequent search or changing authorization.
func (q *BuildQueue) WatchAndRebuildText(ctx context.Context, workspace string, config AutoindexConfig, sourceRoot, stagingRoot string, datasets map[string][]string, baseline TextSnapshot, interval time.Duration, stableSamples int, onJob func(*BuildJob)) error {
	if ctx == nil || config.Prefix != "" || config.Corpus != "" || onJob == nil || len(datasets) < 1 || len(datasets) > 8 {
		return errors.New("invalid rebuild configuration")
	}
	approved := make(map[string][]string, len(datasets))
	paths := make([]string, 0, 32)
	seen := make(map[string]bool)
	for suffix, files := range datasets {
		if !validDatasetSuffix(suffix) || len(files) == 0 || len(files) > 32 {
			return errors.New("invalid approved dataset")
		}
		for _, p := range files {
			if seen[p] || len(paths) >= 32 {
				return errors.New("duplicate or excessive approved paths")
			}
			seen[p] = true
			paths = append(paths, p)
		}
		approved[suffix] = append([]string(nil), files...)
	}
	return q.manager.WatchTextChanges(ctx, workspace, sourceRoot, paths, baseline, interval, stableSamples, func(ctx context.Context, s TextSnapshot) error {
		job, err := q.submitRebuildNotice(ctx, workspace, config, sourceRoot, stagingRoot, approved, s)
		if err != nil {
			return err
		}
		onJob(job)
		return nil
	})
}

func validDatasetSuffix(s string) bool {
	return s != "" && len(s) <= 90 && indexName.MatchString("mygo00000000000000000000000000000000-"+s)
}

// Precheck avoids repeated staging while an admitted job occupies the queue.
// Admission is still decided by submitOwned; other producers may race it.
func (q *BuildQueue) rebuildAdmission(workspace string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.journalFault {
		return ErrBuildJournalUnavailable
	}
	if q.journalPath != "" && len(q.records) >= maxBuildHistory {
		return ErrBuildQueueFull
	}
	n := len(q.pending)
	if q.active != nil {
		n++
		if q.active.workspace == workspace {
			return errRebuildDeferred
		}
	}
	for _, j := range q.pending {
		if j.workspace == workspace {
			return errRebuildDeferred
		}
	}
	if n >= q.capacity {
		return errRebuildDeferred
	}
	return nil
}

func (q *BuildQueue) submitRebuildNotice(ctx context.Context, workspace string, config AutoindexConfig, sourceRoot, stagingRoot string, datasets map[string][]string, observed TextSnapshot) (*BuildJob, error) {
	if err := q.rebuildAdmission(workspace); err != nil {
		return nil, err
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	config.Prefix = "mygo" + hex.EncodeToString(nonce[:])
	names := make(map[string][]string, len(datasets))
	for suffix, paths := range datasets {
		if !validDatasetSuffix(suffix) || strings.TrimSpace(suffix) != suffix {
			return nil, errors.New("invalid dataset suffix")
		}
		names[config.Prefix+"-"+suffix] = paths
	}
	job, err := q.submitApprovedText(ctx, workspace, config, sourceRoot, stagingRoot, names, &observed)
	// A racing producer may fill the queue after the precheck. Retry admission
	// on a later sample only if the queue really is temporarily busy; a full
	// history or a random prefix collision must not cause an endless retry.
	if err == ErrBuildQueueFull || err == ErrBuildDuplicate {
		if a := q.rebuildAdmission(workspace); a == errRebuildDeferred {
			return nil, a
		}
	}
	return job, err
}
