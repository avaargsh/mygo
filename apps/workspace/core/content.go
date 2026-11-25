package core

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// ContentProvider describes a default session and an optional factory for
// independent, explicitly opened instances. Factories own resource creation.
type ContentProvider struct {
	ID          string
	Title       string
	Open        func() (any, error)
	NewInstance func(sessionID string) (any, error)
}

// ContentSession is the UI owner's live binding for a content instance.
// In snapshot v1 ID also serves as the persisted instance reference. Value
// is runtime state and must not be serialized as part of a layout.
type ContentSession struct {
	ID         string
	ProviderID string
	Title      string
	Value      any
}

// ContentManager owns sessions independently of tab views. Calls are serialized
// by the workspace UI owner. Closing a tab does not evict its session.
type ContentManager struct {
	providers map[string]ContentProvider
	sessions  map[string]*ContentSession
}

func NewContentManager() *ContentManager {
	return &ContentManager{providers: make(map[string]ContentProvider), sessions: make(map[string]*ContentSession)}
}

func (m *ContentManager) Register(p ContentProvider) error {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.Title) == "" || p.Open == nil {
		return errors.New("invalid content provider")
	}
	if _, exists := m.providers[p.ID]; exists {
		return errors.New("duplicate content provider")
	}
	if m.sessions[p.ID] != nil {
		return errors.New("provider ID conflicts with existing session")
	}
	m.providers[p.ID] = p
	return nil
}

func (m *ContentManager) Open(id string) (*ContentSession, error) {
	if s := m.sessions[id]; s != nil {
		return s, nil
	}
	p, ok := m.providers[id]
	if !ok {
		return nil, fmt.Errorf("unknown content provider: %s", id)
	}
	value, err := p.Open()
	if err != nil {
		return nil, err
	}
	s := &ContentSession{ID: p.ID, ProviderID: p.ID, Title: p.Title, Value: value}
	m.sessions[id] = s
	return s, nil
}

// OpenInstance explicitly creates or reuses an independent session of an
// opted-in provider. Instance IDs are globally unique within the manager.
// Open only resolves an existing instance; it does not reconstruct one.
func (m *ContentManager) OpenInstance(providerID, sessionID string) (*ContentSession, error) {
	p, ok := m.providers[providerID]
	if !ok || p.NewInstance == nil {
		return nil, errors.New("provider does not support instances")
	}
	if !validInstanceID(sessionID) {
		return nil, errors.New("invalid instance ID")
	}
	if _, reserved := m.providers[sessionID]; reserved {
		return nil, errors.New("instance ID conflicts with provider")
	}
	if s := m.sessions[sessionID]; s != nil {
		if s.ProviderID != providerID {
			return nil, errors.New("instance belongs to another provider")
		}
		return s, nil
	}
	value, err := p.NewInstance(sessionID)
	if err != nil {
		return nil, err
	}
	s := &ContentSession{ID: sessionID, ProviderID: providerID, Title: p.Title, Value: value}
	m.sessions[sessionID] = s
	return s, nil
}

func validInstanceID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (m *ContentManager) Session(id string) *ContentSession { return m.sessions[id] }

const maxNoteBytes = 1 << 20

// Note is a local Markdown source document, not part of the layout snapshot.
// Save is explicit; an unsaved edit survives view closure but not process exit.
type Note struct {
	Text  string
	saved string
	path  string
}

func OpenNote(path string) (*Note, error) {
	n := &Note{path: path}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return n, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxNoteBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxNoteBytes || !utf8.Valid(data) {
		return nil, errors.New("invalid or oversized note")
	}
	n.Text = string(data)
	n.saved = n.Text
	return n, nil
}

func (n *Note) Dirty() bool { return n.Text != n.saved }

func (n *Note) Save() error {
	if len(n.Text) > maxNoteBytes || !utf8.ValidString(n.Text) {
		return errors.New("note must be valid UTF-8 and at most 1 MiB")
	}
	f, err := os.CreateTemp(filepath.Dir(n.path), ".note-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.WriteString(n.Text); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), n.path); err != nil {
		return err
	}
	n.saved = n.Text
	return nil
}
