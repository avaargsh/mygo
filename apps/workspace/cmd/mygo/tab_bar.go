package main

import (
	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/ui"
)

func (s *shell) renderTabBar(c *ui.Context, paneID string, pane core.PaneSnapshot) {
	ui.Row(c.Key("tab-bar")).Role(ui.RoleGroup).Label("Tab bar").Gap(4).Wrap().Children(func() {
		for _, tab := range pane.Tabs {
			ui.Row(c.Key(tab.ID)).Gap(4).Children(func() {
				label := tab.Title
				if tab.ID == pane.ActiveTab {
					label += " (active)"
				}
				ui.Button(c, label).OnClick(func() { s.apply(s.workspace.Activate(paneID, tab.ID)) })
				ui.Button(c, "×").Label("Close " + tab.Title).OnClick(func() { s.apply(s.workspace.Close(paneID, tab.ID)) })
			})
		}
	})
}
