package core

// ContentViewState is a projection of layout, not session or execution state.
// A Tab refers to a content session. An active tab offers a view; MountPane
// selects at most one such view when a resource cannot be mounted twice.
type ContentViewState struct {
	TabReferences int
	ActiveViews   int
	MountPane     string
}

// ResolveContentViews counts references in layout order. The focused pane
// wins a mount if it shows the session; otherwise the first active view wins.
// Zero references do not imply that a session or its executions should stop.
func ResolveContentViews(snapshot WorkspaceSnapshot, sessionID string) ContentViewState {
	if sessionID == "" {
		return ContentViewState{}
	}
	var out ContentViewState
	var ids []string
	leafIDs(snapshot.Layout, &ids)
	for _, id := range ids {
		p := snapshot.Panes[id]
		for _, t := range p.Tabs {
			if t.SessionID != sessionID {
				continue
			}
			out.TabReferences++
			if t.ID == p.ActiveTab {
				out.ActiveViews++
				if out.MountPane == "" || id == snapshot.ActivePane {
					out.MountPane = id
				}
			}
		}
	}
	return out
}

// SessionOwner is the mount selector used by the existing shell. The name
// denotes view placement, not ownership of the session or of any execution.
func SessionOwner(snapshot WorkspaceSnapshot, sessionID string) string {
	return ResolveContentViews(snapshot, sessionID).MountPane
}
