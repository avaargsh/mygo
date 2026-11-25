package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/ui"
)

type shellPanel uint8

const (
	panelExplorer shellPanel = iota
	panelSessions
	panelHidden
)

const activityWidth = 76
const sidebarWidth = 160

func (s *shell) view(c *ui.Context) {
	snapshot := s.workspace.Snapshot()
	ui.Themed(c, workspaceTheme(c)).Fill().Padding(spaceControl).Gap(spaceControl).Children(func() {
		ui.Row(c.Key("workspace-header")).Gap(spaceControl).Wrap().Children(func() {
			ui.Text(c, "MyGo Workspace").FontSize(fontTitle).Bold()
			ui.Spacer(c)
			ui.Button(c.Key("open-commands"), "Commands").OnClick(func() {
				s.paletteOpen = !s.paletteOpen
				s.paletteQuery = ""
			})
			ui.PrimaryButton(c.Key("save"), "Save workspace").OnClick(func() { s.runCommand("workspace.save") })
		})
		if s.paletteOpen {
			s.renderCommandPalette(c)
		}
		ui.Row(c.Key("workspace-body")).Grow(1).MinHeight(0).MinWidth(0).Gap(spaceControl).Children(func() {
			s.renderActivityBar(c)
			switch s.sidebar {
			case panelExplorer:
				s.renderWorkspaceExplorer(c)
			case panelSessions:
				s.renderSessionManager(c, snapshot)
			}
			ui.Column(c.Key("content-area")).Role(ui.RoleMain).Label("Content area").
				Grow(1).MinHeight(0).MinWidth(0).Gap(spaceControl).Children(func() {
				s.renderContentToolbar(c)
				s.renderNode(c, snapshot, snapshot.Layout)
			})
		})
		s.renderStatusBar(c, snapshot)
	})
}

func (s *shell) renderActivityBar(c *ui.Context) {
	ui.Column(c.Key("activity-bar")).Role(ui.RoleNavigation).Label("Activity bar").
		Width(activityWidth).Shrink(0).Padding(4).Gap(4).
		Background(c.Theme().Surface).Radius(radiusPane).Children(func() {
		s.activityButton(c, panelExplorer, "Explorer")
		s.activityButton(c, panelSessions, "Sessions")
		if s.sidebar != panelHidden {
			ui.Button(c.Key("hide-sidebar"), "Hide").Label("Hide sidebar").OnClick(func() { s.sidebar = panelHidden })
		}
	})
}

func (s *shell) activityButton(c *ui.Context, panel shellPanel, name string) {
	activate := func() { s.sidebar = panel }
	if s.sidebar == panel {
		ui.PrimaryButton(c.Key(name), name).OnClick(activate)
		return
	}
	ui.Button(c.Key(name), name).OnClick(activate)
}

func (s *shell) renderWorkspaceExplorer(c *ui.Context) {
	ui.Column(c.Key("workspace-explorer")).Role(ui.RoleRegion).Label("Workspace Explorer").
		Width(sidebarWidth).Shrink(0).MinHeight(0).Padding(spaceControl).Gap(4).
		Background(c.Theme().Surface).Radius(radiusPane).Children(func() {
		ui.Text(c, "Workspace Explorer").Bold()
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Gap(4).Children(func() {
				// Derive launchers from the command registry so new providers
				// need no separate shell registration.
				for _, cmd := range s.commandRegistry().Search("") {
					if strings.HasPrefix(cmd.ID, "content.") {
						ui.Button(c.Key(cmd.ID), strings.TrimPrefix(cmd.Title, "Open ")).
							Disabled(!cmd.Enabled).OnClick(func() { s.runCommand(cmd.ID) })
					}
				}
				ui.Divider(c)
				ui.Text(c, "Appearance").FontSize(fontSmall).Bold()
				for _, mode := range []string{"system", "light", "dark"} {
					label := mode
					if mode == s.theme {
						label += " (selected)"
					}
					s.commandButton(c, "theme-"+mode, "theme."+mode, label)
				}
			})
		})
	})
}

func (s *shell) renderContentToolbar(c *ui.Context) {
	ui.Row(c.Key("content-tools")).Role(ui.RoleToolbar).Label("Pane controls").Gap(4).Wrap().Children(func() {
		s.commandButton(c, "split-right", "layout.split-right", "Split right")
		s.commandButton(c, "split-down", "layout.split-down", "Split down")
		s.commandButton(c, "move-next", "layout.move-next", "Move tab to next pane")
		s.commandButton(c, "remove-pane", "layout.remove-pane", "Close empty pane")
	})
}

func (s *shell) renderSessionManager(c *ui.Context, snapshot core.WorkspaceSnapshot) {
	ui.Column(c.Key("session-manager")).Role(ui.RoleRegion).Label("Session Manager").
		Width(sidebarWidth).Shrink(0).MinHeight(0).Padding(spaceControl).Gap(4).
		Background(c.Theme().Surface).Radius(radiusPane).Children(func() {
		ui.Text(c, "Session Manager").Bold()
		ui.Text(c, "Closing a tab does not stop its session.").FontSize(fontSmall)
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Gap(4).Children(func() {
				if session := s.content.Session("browser"); session != nil {
					if browser, ok := session.Value.(*browserSession); ok && browser.view != nil && core.SessionOwner(snapshot, "browser") == "" {
						ui.Button(c.Key("release-hidden-browser"), "Close hidden browser page").OnClick(func() { s.releaseBrowser(browser) })
					}
				}
				if t := s.terminalSession(); t != nil && core.SessionOwner(snapshot, "terminal") == "" {
					if t.CanStop() {
						ui.Button(c.Key("close-hidden-terminal"), "Close hidden terminal").OnClick(func() { s.stopTerminal(t) })
					}
					if t.State() == core.TerminalStopping {
						ui.Text(c, "Hidden terminal stopping…").FontSize(fontSmall)
					}
				}
				if f := s.filesSession(); f != nil && f.browser != nil && core.SessionOwner(snapshot, "files") == "" {
					ui.Button(c.Key("revoke-hidden-files"), "Revoke hidden folder access").OnClick(func() { s.fileResult(f.close()) })
				}
				if n := s.note(); n != nil && n.Dirty() {
					ui.Button(c.Key("save-dirty-note"), "Save note changes").OnClick(s.saveNote)
				}
				s.renderChatActivity(c, snapshot)
			})
		})
	})
}

func (s *shell) renderStatusBar(c *ui.Context, snapshot core.WorkspaceSnapshot) {
	saved := "Saved"
	if s.dirty {
		saved = "Unsaved changes"
	}
	tabs := 0
	for _, pane := range snapshot.Panes {
		tabs += len(pane.Tabs)
	}
	message := fmt.Sprintf("Local workspace · %d panes · %d tabs · %s", len(snapshot.Panes), tabs, saved)
	if n := s.note(); n != nil && n.Dirty() {
		message += " · Note unsaved"
	}
	activeChats := 0
	for _, session := range s.content.SessionsForProvider("chat") {
		if ch, ok := session.Value.(*chatViewSession); ok && ch != nil && ch.chat.Snapshot().Active {
			activeChats++
		}
	}
	if activeChats != 0 {
		c.After(100 * time.Millisecond)
		message += fmt.Sprintf(" · %d chats running", activeChats)
	}
	ui.Row(c.Key("status-bar")).Role(ui.RoleStatus).Label("Status bar").Gap(spaceControl).Padding(4).
		Background(c.Theme().Surface).Radius(radiusPane).Children(func() {
			ui.Text(c, message).FontSize(fontSmall).TextColor(c.Theme().TextMuted)
			if s.status != "" {
				ui.Text(c, s.status).FontSize(fontSmall)
			}
		})
}
