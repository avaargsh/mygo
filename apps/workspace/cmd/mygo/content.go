package main

import (
	"log"
	"path/filepath"

	"github.com/egoist/mygo/apps/workspace/core"
)

func builtInContent(dir string) (*core.ContentManager, error) {
	m := core.NewContentManager()
	for _, provider := range []core.ContentProvider{
		{ID: "home", Title: "Home", Open: func() (any, error) { return nil, nil }},
		{ID: "runtime", Title: "Runtime info", Open: func() (any, error) { return nil, nil }},
		{ID: "browser", Title: "Browser", Open: func() (any, error) { return &browserSession{}, nil }},
		{ID: "notes", Title: "Notes", Open: func() (any, error) {
			return core.OpenNote(filepath.Join(dir, "notes.md"))
		}},
	} {
		if err := m.Register(provider); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func (s *shell) note() *core.Note {
	if session := s.content.Session("notes"); session != nil {
		n, _ := session.Value.(*core.Note)
		return n
	}
	return nil
}

func (s *shell) saveNote() {
	n := s.note()
	if n == nil {
		return
	}
	if err := n.Save(); err != nil {
		s.status = "Note save failed: " + err.Error()
		log.Print(s.status)
		return
	}
	s.status = "Note saved"
}

func (s *shell) retryContent(id string) {
	s.status = ""
	if _, err := s.content.Open(id); err != nil {
		s.status = "Content open failed: " + err.Error()
		log.Print(s.status)
	}
}
