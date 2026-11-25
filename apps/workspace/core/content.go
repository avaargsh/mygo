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

// ContentProvider describes one built-in session. Multiple instances and
// external providers are deferred until their lifecycle contracts are needed.
type ContentProvider struct {
	ID    string
	Title string
	Open  func() (any, error)
}

type ContentSession struct {
	ID    string
	Title string
	Value any
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
	s := &ContentSession{ID: p.ID, Title: p.Title, Value: value}
	m.sessions[id] = s
	return s, nil
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
