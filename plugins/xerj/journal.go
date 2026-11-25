package xerj

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrBuildJournalUnavailable = errors.New("build journal unavailable")

const maxBuildHistory = 1024

// BuildRecord is status evidence, not a replayable job or index readiness proof.
type BuildRecord struct {
	Workspace string     `json:"workspace"`
	Prefix    string     `json:"prefix"`
	State     BuildState `json:"state"`
}
type buildJournal struct {
	Version  int           `json:"version"`
	Endpoint string        `json:"endpoint"`
	Manifest string        `json:"manifest"`
	Records  []BuildRecord `json:"records"`
}

// CreateJournaledBuildQueue requires an exclusively owned persistent Manager
// and a separate private state file. It never overwrites an existing journal.
func CreateJournaledBuildQueue(m *Manager, capacity int, path string) (*BuildQueue, error) {
	q, err := journalQueue(m, capacity, path)
	if err != nil {
		return nil, err
	}
	raw, err := q.journalBytes()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(q.journalPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	if err = writeManifest(f, raw); err != nil {
		return nil, err
	}
	return q, nil
}

// OpenJournaledBuildQueue recovers status history only. After the old owner
// stops, unfinished tasks become INTERRUPTED and must not be replayed.
// Open the persistent Manager separately; its recovery fences old bindings.
func OpenJournaledBuildQueue(m *Manager, capacity int, path string) (*BuildQueue, error) {
	q, err := journalQueue(m, capacity, path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(q.journalPath)
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxManifestBytes+1))
	closeErr := f.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(raw) > maxManifestBytes {
		return nil, errors.New("journal too large")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var saved buildJournal
	if err = d.Decode(&saved); err != nil {
		return nil, err
	}
	if err = d.Decode(new(any)); err != io.EOF {
		return nil, errors.New("trailing journal data")
	}
	if saved.Version != 1 || saved.Endpoint != m.endpoint || saved.Manifest != filepath.Clean(m.manifestPath) || saved.Records == nil || len(saved.Records) > maxBuildHistory {
		return nil, errors.New("invalid journal identity or size")
	}
	seen := map[string]bool{}
	for i, r := range saved.Records {
		if strings.TrimSpace(r.Workspace) == "" || len(r.Workspace) > 256 || !autoindexPrefix.MatchString(r.Prefix) || seen[r.Prefix] {
			return nil, errors.New("invalid journal record")
		}
		seen[r.Prefix] = true
		switch r.State {
		case BuildQueued, BuildRunning, BuildCanceling:
			saved.Records[i].State = BuildInterrupted
		case BuildSucceeded, BuildFailed, BuildCanceled, BuildInterrupted:
		default:
			return nil, errors.New("invalid journal state")
		}
	}
	q.records = saved.Records
	if err = q.persistJournalLocked(); err != nil {
		return nil, err
	}
	return q, nil
}

func journalQueue(m *Manager, capacity int, path string) (*BuildQueue, error) {
	q, err := NewBuildQueue(m, capacity)
	if err != nil {
		return nil, err
	}
	if m.manifestPath == "" || !filepath.IsAbs(path) || filepath.Clean(path) == filepath.Clean(m.manifestPath) {
		return nil, errors.New("persistent manager and separate absolute journal path required")
	}
	q.journalPath = filepath.Clean(path)
	q.records = make([]BuildRecord, 0)
	return q, nil
}
func (q *BuildQueue) journalBytes() ([]byte, error) {
	raw, err := json.Marshal(buildJournal{Version: 1, Endpoint: q.manager.endpoint, Manifest: filepath.Clean(q.manager.manifestPath), Records: q.records})
	if err != nil {
		return nil, err
	}
	if len(raw) > maxManifestBytes {
		return nil, errors.New("journal too large")
	}
	return raw, nil
}
func (q *BuildQueue) persistJournalLocked() error {
	if q.journalPath == "" {
		return nil
	}
	raw, err := q.journalBytes()
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(q.journalPath), ".xerj-jobs-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = writeManifest(f, raw); err != nil {
		return err
	}
	return os.Rename(f.Name(), q.journalPath)
}

// recordLocked writes the next state before exposing it. A failed write
// freezes dispatch and resolves pending handles as interrupted in memory.
// A terminal write failure cannot roll back an already published index.
func (q *BuildQueue) recordLocked(j *BuildJob, state BuildState) error {
	if q.journalPath == "" {
		return nil
	}
	if q.journalFault {
		return ErrBuildJournalUnavailable
	}
	old := append([]BuildRecord{}, q.records...)
	if state == BuildQueued {
		q.records = append(q.records, BuildRecord{j.workspace, j.prefix, state})
	} else {
		for i := range q.records {
			if q.records[i].Prefix == j.prefix {
				q.records[i].State = state
				break
			}
		}
	}
	if err := q.persistJournalLocked(); err != nil {
		q.records = old
		q.journalFault = true
		failure := fmt.Errorf("%w: %v", ErrBuildJournalUnavailable, err)
		for _, p := range q.pending {
			p.finishLocked(BuildInterrupted, failure)
		}
		q.pending = nil
		return failure
	}
	return nil
}

// History returns the last persisted status snapshot (no executable inputs).
// A journal fault may leave it behind the in-memory job handle state.
func (q *BuildQueue) History() []BuildRecord {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]BuildRecord{}, q.records...)
}
