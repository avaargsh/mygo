package core

import (
	"errors"
	"fmt"
)

type Direction string

const (
	Horizontal     Direction = "horizontal"
	Vertical       Direction = "vertical"
	maxLayoutDepth           = 32
)

// LayoutNode is a portable snapshot: a leaf identifies a pane; a split has exactly two children.
type LayoutNode struct {
	ID        string       `json:"id"`
	Direction Direction    `json:"direction,omitempty"`
	Ratio     float64      `json:"ratio,omitempty"`
	Children  []LayoutNode `json:"children,omitempty"`
}

// Layout owns the split tree, not pane contents. Mutations are serialized by the caller.
type Layout struct{ root LayoutNode }

func NewLayout(paneID string) (*Layout, error) { return RestoreLayout(LayoutNode{ID: paneID}) }

func RestoreLayout(root LayoutNode) (*Layout, error) {
	if err := validateLayout(root, make(map[string]bool), 0); err != nil {
		return nil, err
	}
	return &Layout{root: cloneNode(root)}, nil
}

func validateLayout(n LayoutNode, seen map[string]bool, depth int) error {
	if depth > maxLayoutDepth {
		return errors.New("layout nesting limit exceeded")
	}
	if n.ID == "" || seen[n.ID] {
		return fmt.Errorf("missing or duplicate layout ID: %q", n.ID)
	}
	seen[n.ID] = true
	if len(n.Children) == 0 {
		if n.Direction != "" || n.Ratio != 0 {
			return errors.New("pane has split attributes")
		}
		return nil
	}
	if len(n.Children) != 2 || !validSplit(n.Direction, n.Ratio) {
		return errors.New("invalid split")
	}
	for _, child := range n.Children {
		if err := validateLayout(child, seen, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func validSplit(d Direction, r float64) bool {
	return (d == Horizontal || d == Vertical) && r > 0 && r < 1
}

func cloneNode(n LayoutNode) LayoutNode {
	if len(n.Children) == 0 {
		n.Children = nil
		return n
	}
	children := make([]LayoutNode, len(n.Children))
	for i, child := range n.Children {
		children[i] = cloneNode(child)
	}
	n.Children = children
	return n
}

func (l *Layout) Snapshot() LayoutNode { return cloneNode(l.root) }

func findNode(n *LayoutNode, id string) *LayoutNode {
	if n.ID == id {
		return n
	}
	for i := range n.Children {
		if found := findNode(&n.Children[i], id); found != nil {
			return found
		}
	}
	return nil
}

func (l *Layout) Split(paneID, splitID, newPaneID string, d Direction, ratio float64) error {
	next := l.Snapshot()
	target := findNode(&next, paneID)
	if target == nil || len(target.Children) != 0 {
		return errors.New("target must be a pane")
	}
	*target = LayoutNode{ID: splitID, Direction: d, Ratio: ratio, Children: []LayoutNode{*target, {ID: newPaneID}}}
	if err := validateLayout(next, make(map[string]bool), 0); err != nil {
		return err
	}
	l.root = next
	return nil
}

func (l *Layout) Resize(splitID string, ratio float64) error {
	target := findNode(&l.root, splitID)
	if target == nil || len(target.Children) != 2 || !validSplit(target.Direction, ratio) {
		return errors.New("invalid split or ratio")
	}
	target.Ratio = ratio
	return nil
}

// RemovePane collapses the parent split; the caller must first relocate or close its tab views.
func (l *Layout) RemovePane(paneID string) error {
	if !collapsePane(&l.root, paneID) {
		return errors.New("pane not found or is the last pane")
	}
	return nil
}

func collapsePane(n *LayoutNode, id string) bool {
	if len(n.Children) != 2 {
		return false
	}
	for i := range n.Children {
		if n.Children[i].ID == id && len(n.Children[i].Children) == 0 {
			*n = n.Children[1-i]
			return true
		}
	}
	for i := range n.Children {
		if collapsePane(&n.Children[i], id) {
			return true
		}
	}
	return false
}
