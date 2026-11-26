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
	workspace       *core.Workspace
	window          *mygo.Window
	content         *core.ContentManager
	sequence        int
	status          string
	statePath       string
	theme           string
	dirty           bool
	commands        *core.CommandRegistry
	paletteOpen     bool
	paletteQuery    string
	closePromptOpen bool
	sessionEnding   bool
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
	ui.Themed(c, workspaceTheme(c)).Fill().Padding(spacePage).Gap(spaceSection).Children(func() {
		ui.Row(c).Gap(12).Children(func() {
			ui.Text(c, "MyGo Workspace").FontSize(fontTitle).Bold()
			ui.Spacer(c)
			ui.Text(c, "Local workspace").TextColor(c.Theme().TextMuted)
			ui.Button(c.Key("open-commands"), "Commands").OnClick(func() { s.paletteOpen = !s.paletteOpen; s.paletteQuery = "" })
		})
		ui.Row(c).Gap(spaceControl).Wrap().Children(func() {
			ui.PrimaryButton(c.Key("save"), "Save workspace").OnClick(func() { s.runCommand("workspace.save") })
			for _, mode := range []string{"system", "light", "dark"} {
				label := mode
				if mode == s.theme {
					label += " (selected)"
				}
				s.commandButton(c, "theme-"+mode, "theme."+mode, label)
			}
			s.commandButton(c, "open-home", "content.home", "Home")
			s.commandButton(c, "open-runtime", "content.runtime", "Runtime info")
			s.commandButton(c, "open-system", "content.system", "System Info")
			s.commandButton(c, "open-notes", "content.notes", "Notes")
			s.commandButton(c, "open-browser", "content.browser", "Browser")
			s.commandButton(c, "open-terminal", "content.terminal", "Terminal")
			s.commandButton(c, "open-files", "content.files", "Files")
			s.commandButton(c, "open-search", "content.search", "Search")
			s.commandButton(c, "open-chat", "content.chat", "Chat")
			s.commandButton(c, "new-chat", "content.new-chat", "New chat")
			s.commandButton(c, "split-right", "layout.split-right", "Split right")
			s.commandButton(c, "split-down", "layout.split-down", "Split down")
			s.commandButton(c, "move-next", "layout.move-next", "Move tab to next pane")
			s.commandButton(c, "remove-pane", "layout.remove-pane", "Close empty pane")
		})
		if s.paletteOpen {
			s.renderCommandPalette(c)
		}
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
		if session := s.content.Session("browser"); session != nil {
			if browser, ok := session.Value.(*browserSession); ok && browser.view != nil && core.SessionOwner(snapshot, "browser") == "" {
				ui.Button(c.Key("release-hidden-browser"), "Close hidden browser page").OnClick(func() { s.releaseBrowser(browser) })
			}
		}
		if t := s.terminalSession(); t != nil && core.SessionOwner(snapshot, "terminal") == "" {
			if t.Resource() != nil {
				ui.Button(c.Key("close-hidden-terminal"), "Close hidden terminal").OnClick(func() { s.stopTerminal(t) })
			}
			if t.State() == core.TerminalStopping {
				ui.Text(c, "Hidden terminal stopping…").FontSize(fontSmall)
			}
		}
		if f := s.filesSession(); f != nil && f.browser != nil && core.SessionOwner(snapshot, "files") == "" {
			ui.Button(c.Key("revoke-hidden-files"), "Revoke hidden folder access").OnClick(func() { s.fileResult(f.close()) })
		}
		if search := s.searchSession(); search != nil && core.SessionOwner(snapshot, "search") == "" {
			_, state := search.owner.snapshot()
			if state == "Connecting" || state == "Connected" {
				ui.Button(c.Key("disconnect-hidden-search"), "Disconnect hidden search").OnClick(func() {
					search.owner.stop()
					search.query = ""
				})
			}
		}
		s.renderChatActivity(c, snapshot)
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
				s.renderContent(c, snapshot, id, tab.SessionID)
				return
			}
		}
		ui.Text(c, "Empty pane. Focus it and open a content view.")
	})
}

func (s *shell) renderContent(c *ui.Context, snapshot core.WorkspaceSnapshot, paneID, sessionID string) {
	session := s.content.Session(sessionID)
	if session == nil {
		ui.Text(c, "Content session unavailable: "+sessionID)
		ui.Button(c.Key("retry-content"), "Retry content").OnClick(func() { s.retryContent(sessionID) })
		return
	}
	switch session.ProviderID {
	case "home":
		ui.Text(c, "Your Personal AI Workspace").FontSize(24).Bold()
		ui.Text(c, "Open Runtime info to inspect this running application.")
	case "system":
		if system, ok := session.Value.(*systemSession); ok && system != nil {
			s.renderSystem(c, system)
		} else {
			ui.Text(c, "System metrics session unavailable")
		}
	case "runtime":
		ui.Text(c, "Runtime info").FontSize(20).Bold()
		ui.Text(c, fmt.Sprintf("Platform: %s/%s", runtime.GOOS, runtime.GOARCH))
		ui.Text(c, fmt.Sprintf("Logical CPUs available at startup: %d", runtime.NumCPU()))
		ui.Text(c, "Go runtime: "+runtime.Version())
	case "browser":
		if browser, ok := session.Value.(*browserSession); ok && browser != nil {
			s.renderBrowser(c, snapshot, paneID, browser)
		} else {
			ui.Text(c, "Browser session unavailable")
		}
	case "terminal":
		if t, ok := session.Value.(*core.TerminalSession); ok && t != nil {
			s.renderTerminal(c, snapshot, paneID, t)
		} else {
			ui.Text(c, "Terminal session unavailable")
		}
	case "files":
		if files, ok := session.Value.(*filesSession); ok && files != nil {
			s.renderFiles(c, files)
		} else {
			ui.Text(c, "Files session unavailable")
		}
	case "search":
		if search, ok := session.Value.(*searchSession); ok && search != nil {
			s.renderSearch(c, search)
		} else {
			ui.Text(c, "Search session unavailable")
		}
	case "chat":
		if ch, ok := session.Value.(*chatViewSession); ok && ch != nil {
			s.renderChat(c, snapshot, paneID, ch)
		} else {
			ui.Text(c, "Chat session unavailable")
		}
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
			if _, err := restoreContentSession(content, tab.SessionID); err != nil {
				s.status = "Restore content failed: " + err.Error()
				log.Print(s.status)
			}
		}
	}
	applyTheme(theme)
	if fresh {
		s.open("home")
		s.dirty = false // The default Home tab is recreated on an untouched first run.
	}
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		if e.SessionEnding {
			s.sessionEnding = true
		}
	})
	mygo.App.WhenReady(func() {
		s.window = mygo.NewWindow(mygo.WindowOptions{
			Title:   "MyGo Workspace",
			Width:   1000,
			Height:  680,
			Content: ui.View(s.view),
		})
		s.window.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
			{Role: mygo.RoleAppMenu},
			{Label: "File", Submenu: []*mygo.MenuItem{
				{Label: "Save workspace", Accelerator: "CmdOrCtrl+S", Click: func(*mygo.MenuItem, *mygo.Window) {
					s.runCommand("workspace.save")
					s.window.Invalidate()
				}},
				mygo.Separator(),
				{Label: "Close window", Accelerator: "CmdOrCtrl+W", Click: func(*mygo.MenuItem, *mygo.Window) {
					s.window.Close()
				}},
			}},
			{Role: mygo.RoleEditMenu},
			{Label: "View", Submenu: []*mygo.MenuItem{
				{Label: "Split right", Accelerator: "CmdOrCtrl+Shift+E", Click: func(*mygo.MenuItem, *mygo.Window) {
					s.runCommand("layout.split-right")
					s.window.Invalidate()
				}},
			}},
			{Label: "Window", Submenu: []*mygo.MenuItem{{Role: mygo.RoleMinimize}}},
		}))
		s.window.OnClose(func(e *mygo.CloseEvent) {
			if !s.confirmClose(mygo.Dialog.Message) {
				e.PreventDefault()
				s.window.Invalidate()
			}
		})
		s.window.OnClosed(func() {
			if search := s.searchSession(); search != nil {
				search.owner.stop()
			}
			s.closeChats()
			if session := s.content.Session("browser"); session != nil {
				if browser, ok := session.Value.(*browserSession); ok && browser != nil {
					s.releaseBrowser(browser)
				}
			}
			if f := s.filesSession(); f != nil {
				if err := f.close(); err != nil {
					log.Print("files shutdown: ", err)
				}
			}
			if t := s.terminalSession(); t != nil {
				if err := t.Stop(); err != nil {
					log.Print("terminal shutdown: ", err)
				}
			}
		})
	})
	err = mygo.App.Run()
	if search := s.searchSession(); search != nil {
		search.owner.stop()
		search.owner.wait()
	}
	if err != nil {
		log.Fatal(err)
	}
}
