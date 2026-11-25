// MyGo Workspace is a desktop application layered over the MyGo framework.
package main

import (
	"fmt"
	"log"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/ui"
)

type shell struct {
	workspace *core.Workspace
	sequence  int
	status    string
}

func (s *shell) nextID(prefix string) string {
	s.sequence++
	return fmt.Sprintf("%s-%d", prefix, s.sequence)
}

func (s *shell) apply(err error) {
	s.status = ""
	if err != nil {
		s.status = err.Error()
		log.Print(err)
	}
}

func (s *shell) open(sessionID, title string) {
	snapshot := s.workspace.Snapshot()
	paneID := snapshot.ActivePane
	for _, tab := range snapshot.Panes[paneID].Tabs {
		if tab.SessionID == sessionID {
			s.apply(s.workspace.Activate(paneID, tab.ID))
			return
		}
	}
	s.apply(s.workspace.Open(paneID, core.Tab{ID: s.nextID("tab"), SessionID: sessionID, Title: title}))
}

func (s *shell) split(direction core.Direction) {
	paneID := s.workspace.Snapshot().ActivePane
	s.apply(s.workspace.Split(paneID, s.nextID("split"), s.nextID("pane"), direction, .5))
}

func paneIDs(n core.LayoutNode, ids *[]string) {
	if len(n.Children) == 0 {
		*ids = append(*ids, n.ID)
		return
	}
	for _, child := range n.Children {
		paneIDs(child, ids)
	}
}

func (s *shell) moveNext() {
	snapshot := s.workspace.Snapshot()
	var ids []string
	paneIDs(snapshot.Layout, &ids)
	if len(ids) < 2 {
		return
	}
	for i, id := range ids {
		if id == snapshot.ActivePane {
			s.apply(s.workspace.Move(id, ids[(i+1)%len(ids)], snapshot.Panes[id].ActiveTab))
			return
		}
	}
}

func (s *shell) view(c *ui.Context) {
	snapshot := s.workspace.Snapshot()
	active := snapshot.Panes[snapshot.ActivePane]
	ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
		ui.Row(c).Gap(12).Children(func() {
			ui.Text(c, "MyGo Workspace").FontSize(24).Bold()
			ui.Spacer(c)
			ui.Text(c, "Local workspace").TextColor(c.Theme().TextMuted)
		})
		ui.Row(c).Gap(8).Wrap().Children(func() {
			ui.Button(c.Key("open-home"), "Home").OnClick(func() { s.open("home", "Home") })
			ui.Button(c.Key("open-runtime"), "Runtime info").OnClick(func() { s.open("runtime", "Runtime info") })
			ui.Button(c.Key("split-right"), "Split right").OnClick(func() { s.split(core.Horizontal) })
			ui.Button(c.Key("split-down"), "Split down").OnClick(func() { s.split(core.Vertical) })
			ui.Button(c.Key("move-next"), "Move tab to next pane").Disabled(len(snapshot.Panes) < 2 || active.ActiveTab == "").OnClick(s.moveNext)
			ui.Button(c.Key("remove-pane"), "Close empty pane").Disabled(len(snapshot.Panes) < 2 || len(active.Tabs) != 0).OnClick(func() {
				s.apply(s.workspace.RemovePane(snapshot.ActivePane))
			})
		})
		s.renderNode(c, snapshot, snapshot.Layout, 1)
		ui.Text(c, fmt.Sprintf("Development build · %d panes", len(snapshot.Panes))).FontSize(12).TextColor(c.Theme().TextMuted)
		if s.status != "" {
			ui.Text(c, s.status)
		}
	})
}

func (s *shell) renderNode(c *ui.Context, snapshot core.WorkspaceSnapshot, n core.LayoutNode, weight float32) {
	ui.Column(c.Key(n.ID)).Grow(weight).Basis(0).MinWidth(0).MinHeight(0).Gap(4).Children(func() {
		if len(n.Children) == 0 {
			s.renderPane(c, snapshot, n.ID)
			return
		}
		ui.Row(c).Gap(4).Children(func() {
			ui.Text(c, fmt.Sprintf("%s · %.0f%%", n.Direction, n.Ratio*100)).FontSize(12)
			ui.Button(c.Key("less"), "−").Disabled(n.Ratio <= .2).OnClick(func() {
				s.apply(s.workspace.Resize(n.ID, max(.2, n.Ratio-.1)))
			})
			ui.Button(c.Key("more"), "+").Disabled(n.Ratio >= .8).OnClick(func() {
				s.apply(s.workspace.Resize(n.ID, min(.8, n.Ratio+.1)))
			})
		})
		children := func() {
			s.renderNode(c, snapshot, n.Children[0], float32(n.Ratio))
			s.renderNode(c, snapshot, n.Children[1], float32(1-n.Ratio))
		}
		if n.Direction == core.Horizontal {
			ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Gap(8).Children(children)
		} else {
			ui.Column(c).Grow(1).MinHeight(0).Gap(8).Children(children)
		}
	})
}

func (s *shell) renderPane(c *ui.Context, snapshot core.WorkspaceSnapshot, id string) {
	pane := snapshot.Panes[id]
	ui.Column(c).Grow(1).MinHeight(0).Padding(8).Gap(8).Children(func() {
		label := id
		if id == snapshot.ActivePane {
			label += " (focused)"
		}
		ui.Button(c.Key("focus"), label).OnClick(func() { s.apply(s.workspace.Focus(id)) })
		ui.Row(c).Gap(4).Wrap().Children(func() {
			for _, tab := range pane.Tabs {
				ui.Row(c.Key(tab.ID)).Gap(4).Children(func() {
					label := tab.Title
					if tab.ID == pane.ActiveTab {
						label += " (active)"
					}
					ui.Button(c, label).OnClick(func() { s.apply(s.workspace.Activate(id, tab.ID)) })
					ui.Button(c, "×").Label("Close " + tab.Title).OnClick(func() { s.apply(s.workspace.Close(id, tab.ID)) })
				})
			}
		})
		ui.Divider(c)
		for _, tab := range pane.Tabs {
			if tab.ID == pane.ActiveTab {
				s.renderContent(c, tab.SessionID)
				return
			}
		}
		ui.Text(c, "Empty pane. Focus it and open a content view.")
	})
}

func (s *shell) renderContent(c *ui.Context, sessionID string) {
	switch sessionID {
	case "home":
		ui.Text(c, "Your Personal AI Workspace").FontSize(24).Bold()
		ui.Text(c, "Open Runtime info to inspect this running application.")
	case "runtime":
		ui.Text(c, "Runtime info").FontSize(20).Bold()
		ui.Text(c, fmt.Sprintf("Platform: %s/%s", runtime.GOOS, runtime.GOARCH))
		ui.Text(c, fmt.Sprintf("Logical CPUs available at startup: %d", runtime.NumCPU()))
		ui.Text(c, "Go runtime: "+runtime.Version())
	default:
		ui.Text(c, "Content provider unavailable: "+sessionID)
	}
}

func main() {
	w, err := core.NewWorkspace("pane-root")
	if err != nil {
		log.Fatal(err)
	}
	s := &shell{workspace: w}
	s.open("home", "Home")
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:   "MyGo Workspace",
			Width:   1000,
			Height:  680,
			Content: ui.View(s.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
