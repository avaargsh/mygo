// Package core holds workspace state independent of the desktop framework.
package core

import "errors"

// Tab references a content session; closing the view never closes the session.
type Tab struct {
	ID        string
	SessionID string
	Title     string
}

// Pane owns tab ordering and selection. Callers serialize mutations.
type Pane struct {
	tabs   []Tab
	active string
}

func (p *Pane) Tabs() []Tab      { return append([]Tab(nil), p.tabs...) }
func (p *Pane) ActiveID() string { return p.active }

func (p *Pane) index(id string) int {
	for i, tab := range p.tabs {
		if tab.ID == id {
			return i
		}
	}
	return -1
}

func (p *Pane) Open(tab Tab) error {
	if tab.ID == "" || tab.SessionID == "" {
		return errors.New("tab and session IDs are required")
	}
	if p.index(tab.ID) >= 0 {
		return errors.New("duplicate tab ID")
	}
	p.tabs = append(p.tabs, tab)
	p.active = tab.ID
	return nil
}

func (p *Pane) Activate(id string) bool {
	if p.index(id) < 0 {
		return false
	}
	p.active = id
	return true
}

func (p *Pane) Close(id string) bool {
	i := p.index(id)
	if i < 0 {
		return false
	}
	p.tabs = append(p.tabs[:i], p.tabs[i+1:]...)
	if p.active == id {
		p.active = ""
		if len(p.tabs) > 0 {
			if i == len(p.tabs) {
				i--
			}
			p.active = p.tabs[i].ID
		}
	}
	return true
}
