package core

import "errors"

// Workspace owns the layout, pane state and global tab identity. Its owner serializes commands.
type Workspace struct {
	layout *Layout
	panes  map[string]*Pane
	active string
}

type PaneSnapshot struct {
	Tabs      []Tab  `json:"tabs"`
	ActiveTab string `json:"activeTab"`
}

type WorkspaceSnapshot struct {
	Version    int                     `json:"version"`
	Layout     LayoutNode              `json:"layout"`
	Panes      map[string]PaneSnapshot `json:"panes"`
	ActivePane string                  `json:"activePane"`
}

func NewWorkspace(paneID string) (*Workspace, error) {
	l, err := NewLayout(paneID)
	if err != nil {
		return nil, err
	}
	return &Workspace{layout: l, panes: map[string]*Pane{paneID: {}}, active: paneID}, nil
}

func leafIDs(n LayoutNode, ids *[]string) {
	if len(n.Children) == 0 {
		*ids = append(*ids, n.ID)
		return
	}
	for _, child := range n.Children {
		leafIDs(child, ids)
	}
}

func (w *Workspace) Snapshot() WorkspaceSnapshot {
	s := WorkspaceSnapshot{Version: 1, Layout: w.layout.Snapshot(), Panes: make(map[string]PaneSnapshot, len(w.panes)), ActivePane: w.active}
	for id, p := range w.panes {
		s.Panes[id] = PaneSnapshot{Tabs: p.Tabs(), ActiveTab: p.ActiveID()}
	}
	return s
}

func RestoreWorkspace(s WorkspaceSnapshot) (*Workspace, error) {
	if s.Version != 1 {
		return nil, errors.New("unsupported workspace snapshot version")
	}
	l, err := RestoreLayout(s.Layout)
	if err != nil {
		return nil, err
	}
	var ids []string
	leafIDs(l.root, &ids)
	if len(ids) != len(s.Panes) {
		return nil, errors.New("layout and panes disagree")
	}
	w := &Workspace{layout: l, panes: make(map[string]*Pane, len(ids)), active: s.ActivePane}
	for _, id := range ids {
		state, ok := s.Panes[id]
		if !ok {
			return nil, errors.New("missing pane")
		}
		w.panes[id] = &Pane{}
		for _, tab := range state.Tabs {
			if err := w.Open(id, tab); err != nil {
				return nil, err
			}
		}
		p := w.panes[id]
		if len(state.Tabs) == 0 {
			if state.ActiveTab != "" {
				return nil, errors.New("empty pane has active tab")
			}
		} else if !p.Activate(state.ActiveTab) {
			return nil, errors.New("invalid active tab")
		}
	}
	if err := w.Focus(s.ActivePane); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Workspace) Focus(id string) error {
	if w.panes[id] == nil {
		return errors.New("pane not found")
	}
	w.active = id
	return nil
}

func (w *Workspace) Open(id string, tab Tab) error {
	p := w.panes[id]
	if p == nil {
		return errors.New("pane not found")
	}
	for _, other := range w.panes {
		if other.index(tab.ID) >= 0 {
			return errors.New("duplicate workspace tab ID")
		}
	}
	if err := p.Open(tab); err != nil {
		return err
	}
	w.active = id
	return nil
}

func (w *Workspace) Activate(id, tabID string) error {
	p := w.panes[id]
	if p == nil || !p.Activate(tabID) {
		return errors.New("tab not found in pane")
	}
	w.active = id
	return nil
}

func (w *Workspace) Close(id, tabID string) error {
	p := w.panes[id]
	if p == nil || !p.Close(tabID) {
		return errors.New("tab not found in pane")
	}
	return nil
}

func (w *Workspace) Split(id, splitID, newID string, d Direction, ratio float64) error {
	if err := w.layout.Split(id, splitID, newID, d, ratio); err != nil {
		return err
	}
	w.panes[newID] = &Pane{}
	w.active = newID
	return nil
}

func (w *Workspace) Resize(id string, ratio float64) error { return w.layout.Resize(id, ratio) }

func (w *Workspace) Move(from, to, tabID string) error {
	src, dst := w.panes[from], w.panes[to]
	if src == nil || dst == nil {
		return errors.New("pane not found")
	}
	i := src.index(tabID)
	if i < 0 {
		return errors.New("tab not found")
	}
	if from == to {
		return w.Activate(from, tabID)
	}
	// Open first: if rejected the source has not been modified.
	if err := dst.Open(src.tabs[i]); err != nil {
		return err
	}
	src.Close(tabID)
	w.active = to
	return nil
}

func (w *Workspace) RemovePane(id string) error {
	p := w.panes[id]
	if p == nil {
		return errors.New("pane not found")
	}
	if len(p.tabs) != 0 {
		return errors.New("pane still contains tabs")
	}
	if err := w.layout.RemovePane(id); err != nil {
		return err
	}
	delete(w.panes, id)
	if w.active == id {
		var ids []string
		leafIDs(w.layout.root, &ids)
		w.active = ids[0]
	}
	return nil
}
