package xerj

import (
	"context"
	"errors"
	"strings"
	"sync"
)

var (
	ErrIndexNotReady   = errors.New("workspace index not ready")
	ErrStaleGeneration = errors.New("stale workspace index generation")
)

type IndexState string

const (
	IndexUnbound  IndexState = "UNBOUND"
	IndexBuilding IndexState = "BUILDING"
	IndexReady    IndexState = "READY"
	IndexFailed   IndexState = "FAILED"
)

type IndexStatus struct {
	State      IndexState
	Generation uint64
}

// Generation is an opaque, manager-local ticket for trusted index builders.
type Generation struct {
	manager   *Manager
	workspace string
	number    uint64
}

type binding struct {
	status IndexStatus
	client *Client
}

// Manager coordinates application-owned bindings on one local node. It is
// in-memory by default, does not create indices, and is not an authorization service.
// Only trusted application code may call its methods. Do not copy after use.
type Manager struct {
	mu            sync.Mutex
	endpoint      string
	workspaces    map[string]binding
	reserved      map[string]struct{}
	manifestPath  string
	manifestFault bool
}

func NewManager(endpoint string) *Manager {
	return &Manager{endpoint: endpoint, workspaces: make(map[string]binding), reserved: make(map[string]struct{})}
}

// Begin fences the old binding immediately. Each attempt requires a fresh,
// dedicated exact index, never reused during this manager's lifetime.
func (m *Manager) Begin(workspace, index string) (Generation, error) {
	if strings.TrimSpace(workspace) == "" || len(workspace) > 256 {
		return Generation{}, errors.New("invalid workspace identity")
	}
	client, err := New(m.endpoint, index)
	if err != nil {
		return Generation{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.manifestFault {
		return Generation{}, ErrManifestUnavailable
	}
	if _, ok := m.reserved[index]; ok {
		return Generation{}, errors.New("index already reserved")
	}
	old := m.workspaces[workspace]
	if old.status.Generation == ^uint64(0) {
		return Generation{}, errors.New("generation exhausted")
	}
	number := old.status.Generation + 1
	m.reserved[index] = struct{}{}
	m.workspaces[workspace] = binding{status: IndexStatus{State: IndexBuilding, Generation: number}, client: client}
	if err := m.persistLocked(); err != nil {
		return Generation{}, err
	}
	return Generation{manager: m, workspace: workspace, number: number}, nil
}

// Publish is a trusted caller's assertion that its dedicated corpus was built
// and verified. It does not independently verify engine identity or content.
func (m *Manager) Publish(ticket Generation) error { return m.finish(ticket, IndexReady) }
func (m *Manager) Fail(ticket Generation) error    { return m.finish(ticket, IndexFailed) }
func (m *Manager) finish(ticket Generation, state IndexState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.manifestFault {
		return ErrManifestUnavailable
	}
	b, ok := m.workspaces[ticket.workspace]
	if ticket.manager != m || !ok || b.status.Generation != ticket.number || b.status.State != IndexBuilding {
		return ErrStaleGeneration
	}
	b.status.State = state
	m.workspaces[ticket.workspace] = b
	return m.persistLocked()
}

func (m *Manager) Status(workspace string) IndexStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.workspaces[workspace]; ok {
		if m.manifestFault {
			return IndexStatus{State: IndexFailed, Generation: b.status.Generation}
		}
		return b.status
	}
	return IndexStatus{State: IndexUnbound}
}

// Search uses only the current ready binding. A result superseded while the
// request was in flight is discarded; no fallback to older indices occurs.
// Its final generation check is the linearization point for successful reads.
func (m *Manager) Search(ctx context.Context, workspace, query string, limit int) ([]Hit, error) {
	m.mu.Lock()
	if m.manifestFault {
		m.mu.Unlock()
		return nil, ErrManifestUnavailable
	}
	b, ok := m.workspaces[workspace]
	m.mu.Unlock()
	if !ok || b.status.State != IndexReady {
		return nil, ErrIndexNotReady
	}
	hits, err := b.client.Search(ctx, query, limit)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.manifestFault {
		return nil, ErrManifestUnavailable
	}
	if m.workspaces[workspace].status != b.status {
		return nil, ErrStaleGeneration
	}
	if err != nil {
		return nil, err
	}
	return hits, nil
}
