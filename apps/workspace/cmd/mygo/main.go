// MyGo Workspace is a desktop application layered over the MyGo framework.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/ui"
)

type shell struct {
	workspace *core.Workspace
	content   *core.ContentManager
	sequence  int
	status    string
	statePath string
	theme     string
	dirty     bool
}

func (s *shell) nextID(prefix string) string {
	snapshot := s.workspace.Snapshot()
	for {
		s.sequence++
		id := fmt.Sprintf("%s-%d", prefix, s.sequence)
		if layoutHasID(snapshot.Layout, id) {
			continue
		}
		used := false
		for _, pane := range snapshot.Panes {
			for _, tab := range pane.Tabs {
				if tab.ID == id {
					used = true
				}
			}
		}
		if !used {
			return id
		}
	}
}

func layoutHasID(n core.LayoutNode, id string) bool {
	if n.ID == id {
		return true
	}
	for _, child := range n.Children {
		if layoutHasID(child, id) {
			return true
		}
	}
	return false
}

func (s *shell) save() {
	if err := core.SaveState(s.statePath, s.workspace, s.theme); err != nil {
		s.status = "Save failed: " + err.Error()
		log.Print(s.status)
		return
	}
	s.dirty = false
	s.status = "Workspace saved"
}

func (s *shell) setTheme(mode string) {
	s.theme = mode
	applyTheme(mode)
	s.dirty = true
	s.status = ""
}

func (s *shell) apply(err error) {
	s.status = ""
	if err != nil {
		s.status = err.Error()
		log.Print(err)
		return
	}
	s.dirty = true
}

func (s *shell) open(sessionID string) {
	session, err := s.content.Open(sessionID)
	if err != nil {
		s.apply(err)
		return
	}
	snapshot := s.workspace.Snapshot()
	paneID := snapshot.ActivePane
	for _, tab := range snapshot.Panes[paneID].Tabs {
		if tab.SessionID == sessionID {
			s.apply(s.workspace.Activate(paneID, tab.ID))
			return
		}
	}
	s.apply(s.workspace.Open(paneID, core.Tab{ID: s.nextID("tab"), SessionID: session.ID, Title: session.Title}))
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
	ui.Themed(c, workspaceTheme(c)).Fill().Padding(spacePage).Gap(spaceSection).Children(func() {
		ui.Row(c).Gap(12).Children(func() {
			ui.Text(c, "MyGo Workspace").FontSize(fontTitle).Bold()
			ui.Spacer(c)
			ui.Text(c, "Local workspace").TextColor(c.Theme().TextMuted)
		})
		ui.Row(c).Gap(spaceControl).Wrap().Children(func() {
			ui.PrimaryButton(c.Key("save"), "Save workspace").OnClick(s.save)
			for _, mode := range []string{"system", "light", "dark"} {
				label := mode
				if mode == s.theme {
					label += " (selected)"
				}
				ui.Button(c.Key("theme-"+mode), label).OnClick(func() { s.setTheme(mode) })
			}
			ui.Button(c.Key("open-home"), "Home").OnClick(func() { s.open("home") })
			ui.Button(c.Key("open-runtime"), "Runtime info").OnClick(func() { s.open("runtime") })
			ui.Button(c.Key("open-notes"), "Notes").OnClick(func() { s.open("notes") })
			ui.Button(c.Key("split-right"), "Split right").OnClick(func() { s.split(core.Horizontal) })
			ui.Button(c.Key("split-down"), "Split down").OnClick(func() { s.split(core.Vertical) })
			ui.Button(c.Key("move-next"), "Move tab to next pane").Disabled(len(snapshot.Panes) < 2 || active.ActiveTab == "").OnClick(s.moveNext)
			ui.Button(c.Key("remove-pane"), "Close empty pane").Disabled(len(snapshot.Panes) < 2 || len(active.Tabs) != 0).OnClick(func() {
				s.apply(s.workspace.RemovePane(snapshot.ActivePane))
			})
		})
		s.renderNode(c, snapshot, snapshot.Layout)
		saved := "Saved"
		if s.dirty {
			saved = "Unsaved changes"
		}
		ui.Text(c, fmt.Sprintf("Development build · %d panes · %s", len(snapshot.Panes), saved)).FontSize(fontSmall).TextColor(c.Theme().TextMuted)
		if n := s.note(); n != nil && n.Dirty() {
			ui.Text(c, "Notes have unsaved changes").FontSize(fontSmall)
			ui.Button(c.Key("save-dirty-note"), "Save note changes").OnClick(s.saveNote)
		}
		if s.status != "" {
			ui.Text(c, s.status)
		}
	})
}

func (s *shell) renderNode(c *ui.Context, snapshot core.WorkspaceSnapshot, n core.LayoutNode) {
	ui.Column(c.Key(n.ID)).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		if len(n.Children) == 0 {
			s.renderPane(c, snapshot, n.ID)
			return
		}
		percent := float32(n.Ratio * 100)
		first := func() { s.renderNode(c, snapshot, n.Children[0]) }
		second := func() { s.renderNode(c, snapshot, n.Children[1]) }
		var splitter ui.Element
		if n.Direction == core.Horizontal {
			splitter = ui.SplitPercent(c, &percent, first, second)
		} else {
			splitter = ui.SplitVerticalPercent(c, &percent, first, second)
		}
		splitter.Fill().MinWidth(0).MinHeight(0).SplitGap(spaceControl).SplitReset(50)
		if splitter.Changed() {
			// A small window can drive the widget to 0 or 100; the domain
			// always requires a strict interior ratio.
			s.apply(s.workspace.Resize(n.ID, float64(min(99, max(1, percent)))/100))
		}
	})
}

func (s *shell) renderPane(c *ui.Context, snapshot core.WorkspaceSnapshot, id string) {
	pane := snapshot.Panes[id]
	border := c.Theme().Border
	if id == snapshot.ActivePane {
		border = c.Theme().Accent
	}
	ui.Column(c).Grow(1).MinHeight(0).MinWidth(0).Padding(spaceControl).Gap(spaceControl).
		Background(c.Theme().Surface).Border(1, border).Radius(radiusPane).Clip().Children(func() {
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
	session := s.content.Session(sessionID)
	if session == nil {
		ui.Text(c, "Content session unavailable: "+sessionID)
		ui.Button(c.Key("retry-content"), "Retry content").OnClick(func() { s.open(sessionID) })
		return
	}
	switch session.ID {
	case "home":
		ui.Text(c, "Your Personal AI Workspace").FontSize(24).Bold()
		ui.Text(c, "Open Runtime info to inspect this running application.")
	case "runtime":
		ui.Text(c, "Runtime info").FontSize(20).Bold()
		ui.Text(c, fmt.Sprintf("Platform: %s/%s", runtime.GOOS, runtime.GOARCH))
		ui.Text(c, fmt.Sprintf("Logical CPUs available at startup: %d", runtime.NumCPU()))
		ui.Text(c, "Go runtime: "+runtime.Version())
	case "notes":
		n, ok := session.Value.(*core.Note)
		if !ok || n == nil {
			ui.Text(c, "Notes session unavailable")
			return
		}
		ui.Text(c, "Markdown note").FontSize(fontTitle).Bold()
		ui.TextArea(c, &n.Text).Label("Markdown source").Placeholder("Write Markdown…").Grow(1).MaxLength(1 << 18)
		ui.Button(c, "Save note").OnClick(s.saveNote)
	default:
		ui.Text(c, "Content provider unavailable: "+sessionID)
	}
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--webview-smoke" {
		runWebViewSmoke()
		return
	}
	config, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	dir := filepath.Join(config, "MyGoWorkspace")
	if err := os.MkdirAll(dir, 0700); err != nil {
		log.Fatal(err)
	}
	path := filepath.Join(dir, "state.json")
	w, theme, err := core.LoadState(path)
	fresh := errors.Is(err, os.ErrNotExist)
	if fresh {
		w, err = core.NewWorkspace("pane-root")
		theme = "system"
	}
	if err != nil {
		log.Fatalf("Cannot restore workspace %s (original file preserved): %v", path, err)
	}
	content, err := builtInContent(dir)
	if err != nil {
		log.Fatal(err)
	}
	s := &shell{workspace: w, content: content, statePath: path, theme: theme}
	for _, pane := range w.Snapshot().Panes {
		for _, tab := range pane.Tabs {
			if _, err := content.Open(tab.SessionID); err != nil {
				s.status = "Restore content failed: " + err.Error()
				log.Print(s.status)
			}
		}
	}
	applyTheme(theme)
	if fresh {
		s.open("home")
	}
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
