package xerj

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var ErrManifestUnavailable = errors.New("index manifest unavailable")

const maxManifestBytes = 4 << 20

type manifest struct {
	Version  int               `json:"version"`
	Endpoint string            `json:"endpoint"`
	Reserved []string          `json:"reserved"`
	Bindings []manifestBinding `json:"bindings"`
}
type manifestBinding struct {
	Workspace  string     `json:"workspace"`
	Index      string     `json:"index"`
	Generation uint64     `json:"generation"`
	State      IndexState `json:"state"`
}

// CreatePersistentManager initializes a new manifest without replacing an existing
// one. The application must enforce exclusive ownership of the node and manifest
// and keep the manifest in its private state directory outside indexed trees.
func CreatePersistentManager(endpoint, path string) (*Manager, error) {
	if _, err := New(endpoint, "manifest-probe"); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("manifest path must be absolute")
	}
	m := NewManager(endpoint)
	m.manifestPath = path
	raw, err := m.manifestBytes()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	if err := writeManifest(f, raw); err != nil {
		return nil, err
	}
	return m, nil
}

// OpenPersistentManager restores reservations and generation numbers, but never
// trusts previously READY or BUILDING bindings. All require a fresh generation.
// The caller must ensure the previous owner is stopped before opening.
func OpenPersistentManager(endpoint, path string) (*Manager, error) {
	if _, err := New(endpoint, "manifest-probe"); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("manifest path must be absolute")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxManifestBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxManifestBytes {
		return nil, errors.New("manifest too large")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var saved manifest
	if err := d.Decode(&saved); err != nil {
		return nil, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, errors.New("trailing manifest data")
	}
	if saved.Version != 1 || saved.Endpoint != endpoint || saved.Reserved == nil || saved.Bindings == nil {
		return nil, errors.New("invalid manifest version, endpoint or collections")
	}
	m := NewManager(endpoint)
	m.manifestPath = path
	for _, index := range saved.Reserved {
		if !indexName.MatchString(index) {
			return nil, errors.New("invalid reserved index")
		}
		if _, ok := m.reserved[index]; ok {
			return nil, errors.New("duplicate reserved index")
		}
		m.reserved[index] = struct{}{}
	}
	used := make(map[string]bool)
	for _, b := range saved.Bindings {
		if strings.TrimSpace(b.Workspace) == "" || len(b.Workspace) > 256 || b.Generation == 0 {
			return nil, errors.New("invalid workspace binding")
		}
		if b.State != IndexReady && b.State != IndexBuilding && b.State != IndexFailed {
			return nil, errors.New("invalid binding state")
		}
		if _, ok := m.workspaces[b.Workspace]; ok {
			return nil, errors.New("duplicate workspace")
		}
		if _, ok := m.reserved[b.Index]; !ok || used[b.Index] {
			return nil, errors.New("invalid index reservation")
		}
		client, err := New(endpoint, b.Index)
		if err != nil {
			return nil, err
		}
		used[b.Index] = true
		m.workspaces[b.Workspace] = binding{status: IndexStatus{State: IndexFailed, Generation: b.Generation}, client: client}
	}
	return m, nil
}

// Caller holds the manager mutex, except during private construction.
func (m *Manager) manifestBytes() ([]byte, error) {
	saved := manifest{Version: 1, Endpoint: m.endpoint, Reserved: make([]string, 0, len(m.reserved)), Bindings: make([]manifestBinding, 0, len(m.workspaces))}
	for index := range m.reserved {
		saved.Reserved = append(saved.Reserved, index)
	}
	for workspace, b := range m.workspaces {
		saved.Bindings = append(saved.Bindings, manifestBinding{Workspace: workspace, Index: b.client.index, Generation: b.status.Generation, State: b.status.State})
	}
	sort.Strings(saved.Reserved)
	sort.Slice(saved.Bindings, func(i, j int) bool { return saved.Bindings[i].Workspace < saved.Bindings[j].Workspace })
	raw, err := json.Marshal(saved)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxManifestBytes {
		return nil, errors.New("manifest too large")
	}
	return raw, nil
}

func writeManifest(f *os.File, raw []byte) error {
	_, err := f.Write(raw)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// Replace within the same directory after syncing the temporary file. This is
// not a power-loss guarantee or a multi-process transaction protocol.
func (m *Manager) persistLocked() error {
	if m.manifestPath == "" {
		return nil
	}
	err := m.replaceManifest()
	if err != nil {
		m.manifestFault = true
		return fmt.Errorf("%w: %v", ErrManifestUnavailable, err)
	}
	return nil
}
func (m *Manager) replaceManifest() error {
	raw, err := m.manifestBytes()
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(m.manifestPath), ".xerj-manifest-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := writeManifest(f, raw); err != nil {
		return err
	}
	return os.Rename(f.Name(), m.manifestPath)
}
