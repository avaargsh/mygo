package xerj

import (
	"errors"
	"strings"
)

// Nonzero-sized identity objects are unique while referenced by a queued job.
// These tickets are process-local: the journal never replays queued work.
type sourceRevision struct{ marker byte }
type sourceTicket struct{ revision *sourceRevision }

func (m *Manager) captureRevision(workspace string) (sourceTicket, error) {
	if strings.TrimSpace(workspace) == "" || len(workspace) > 256 {
		return sourceTicket{}, errors.New("invalid workspace identity")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.manifestFault {
		return sourceTicket{}, ErrManifestUnavailable
	}
	return sourceTicket{revision: m.revisions[workspace]}, nil
}

// Invalidate must be called by trusted application code when the approved
// source or its authorization changes. Existing bindings become FAILED and
// cannot be published or searched; queued builds from before this call cannot
// reserve a new generation. It does not cancel processes or delete indices.
// Unbound-workspace revision fences are process-local; no jobs replay on restart.
func (m *Manager) Invalidate(workspace string) error {
	if strings.TrimSpace(workspace) == "" || len(workspace) > 256 {
		return errors.New("invalid workspace identity")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.manifestFault {
		return ErrManifestUnavailable
	}
	if m.revisions == nil {
		m.revisions = make(map[string]*sourceRevision)
	}
	m.revisions[workspace] = &sourceRevision{marker: 1}
	b, ok := m.workspaces[workspace]
	if !ok || b.status.State == IndexFailed {
		return nil
	}
	b.status.State = IndexFailed
	m.workspaces[workspace] = b
	return m.persistLocked()
}
