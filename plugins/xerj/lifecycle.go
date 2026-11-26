package xerj

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
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
	status  IndexStatus
	clients []*Client
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
	revisions     map[string]*sourceRevision
}

func NewManager(endpoint string) *Manager {
	return &Manager{endpoint: endpoint, workspaces: make(map[string]binding), reserved: make(map[string]struct{})}
}

// Begin fences the old binding immediately. Each attempt requires a fresh,
// dedicated exact index, never reused during this manager's lifetime.
func (m *Manager) Begin(workspace, index string) (Generation, error) {
	return m.BeginSet(workspace, []string{index})
}

// BeginSet reserves all exact indices as one generation (maximum eight).
func (m *Manager) BeginSet(workspace string, indices []string) (Generation, error) {
	return m.beginSet(workspace, indices, "")
}

func (m *Manager) beginSet(workspace string, indices []string, freshPrefix string) (Generation, error) {
	return m.beginSetAt(workspace, indices, freshPrefix, nil)
}

func (m *Manager) beginSetAt(workspace string, indices []string, freshPrefix string, revision *sourceTicket) (Generation, error) {
	if strings.TrimSpace(workspace) == "" || len(workspace) > 256 {
		return Generation{}, errors.New("invalid workspace identity")
	}
	if len(indices) == 0 || len(indices) > 8 {
		return Generation{}, errors.New("invalid index set size")
	}
	names := append([]string(nil), indices...)
	sort.Strings(names)
	clients := make([]*Client, 0, len(names))
	for i, index := range names {
		if i > 0 && names[i-1] == index {
			return Generation{}, errors.New("duplicate index")
		}
		client, err := New(m.endpoint, index)
		if err != nil {
			return Generation{}, err
		}
		clients = append(clients, client)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.manifestFault {
		return Generation{}, ErrManifestUnavailable
	}
	if revision != nil && m.revisions[workspace] != revision.revision {
		return Generation{}, ErrStaleGeneration
	}
	if freshPrefix != "" {
		if err := m.checkPrefixLocked(freshPrefix); err != nil {
			return Generation{}, err
		}
	}
	for _, index := range names {
		if _, ok := m.reserved[index]; ok {
			return Generation{}, errors.New("index already reserved")
		}
	}
	old := m.workspaces[workspace]
	if old.status.Generation == ^uint64(0) {
		return Generation{}, errors.New("generation exhausted")
	}
	number := old.status.Generation + 1
	for _, index := range names {
		m.reserved[index] = struct{}{}
	}
	m.workspaces[workspace] = binding{status: IndexStatus{State: IndexBuilding, Generation: number}, clients: clients}
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
	hits, err := searchSet(ctx, b.clients, query, limit)
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

// searchSet uses a shared deadline and deterministic round-robin interleaving,
// not a claim of comparable relevance scores across datasets. Any failure
// discards all hits, even when the global limit was already satisfied.
func searchSet(ctx context.Context, clients []*Client, query string, limit int) ([]Hit, error) {
	if ctx == nil || limit < 1 || limit > 20 {
		return nil, errors.New("invalid search context or limit")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	sets := make([][]Hit, 0, len(clients))
	for _, c := range clients {
		hits, err := c.Search(ctx, query, limit)
		if err != nil {
			return nil, err
		}
		sets = append(sets, hits)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := make([]Hit, 0, limit)
	for rank := 0; rank < limit && len(out) < limit; rank++ {
		for _, hits := range sets {
			if rank < len(hits) {
				out = append(out, hits[rank])
				if len(out) == limit {
					break
				}
			}
		}
	}
	return out, nil
}
