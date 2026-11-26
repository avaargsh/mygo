package main

import (
	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/ui"
)

func (s *shell) commandRegistry() *core.CommandRegistry {
	if s.commands != nil {
		return s.commands
	}
	r := core.NewCommandRegistry()
	register := func(c core.Command) {
		if err := r.Register(c); err != nil {
			panic(err) // A duplicate built-in ID is a programming error.
		}
	}
	register(core.Command{ID: "workspace.save", Title: "Save workspace", Keywords: "保存", Run: s.save})
	for _, mode := range []string{"system", "light", "dark"} {
		register(core.Command{ID: "theme." + mode, Title: "Theme: " + mode, Keywords: "appearance 主题", Run: func() { s.setTheme(mode) }})
	}
	for _, item := range []struct{ id, title, keywords string }{
		{"home", "Home", "首页"},
		{"runtime", "Runtime info", "运行环境"},
		{"system", "System Info", "cpu memory disk network 系统"},
		{"notes", "Notes", "markdown document 笔记"},
		{"browser", "Browser", "web url 浏览器"},
		{"terminal", "Terminal", "shell 终端"},
		{"search", "Search", "xerj passages files 搜索 检索"},
		{"files", "Files", "folder 文件"},
		{"chat", "Chat", "llm model 对话 聊天"},
		{"runs", "Run History", "chat generation status 运行 记录"},
	} {
		register(core.Command{ID: "content." + item.id, Title: "Open " + item.title, Keywords: item.keywords, Run: func() { s.open(item.id) }})
	}
	register(core.Command{ID: "content.new-chat", Title: "New chat", Keywords: "conversation 新对话", Run: s.newChat})
	register(core.Command{ID: "layout.split-right", Title: "Split right", Keywords: "horizontal pane 分屏", Run: func() { s.split(core.Horizontal) }})
	register(core.Command{ID: "layout.split-down", Title: "Split down", Keywords: "vertical pane 分屏", Run: func() { s.split(core.Vertical) }})
	register(core.Command{ID: "layout.move-next", Title: "Move tab to next pane", Keywords: "移动 标签", Enabled: func() bool {
		snap := s.workspace.Snapshot()
		return len(snap.Panes) > 1 && snap.Panes[snap.ActivePane].ActiveTab != ""
	}, Run: s.moveNext})
	register(core.Command{ID: "layout.remove-pane", Title: "Close empty pane", Keywords: "关闭 空 分屏", Enabled: func() bool {
		snap := s.workspace.Snapshot()
		return len(snap.Panes) > 1 && len(snap.Panes[snap.ActivePane].Tabs) == 0
	}, Run: func() { s.apply(s.workspace.RemovePane(s.workspace.Snapshot().ActivePane)) }})
	s.commands = r
	return r
}

func (s *shell) runCommand(id string) {
	if err := s.commandRegistry().Execute(id); err != nil {
		s.status = err.Error()
		return
	}
	s.paletteOpen, s.paletteQuery = false, ""
}

func (s *shell) commandButton(c *ui.Context, key, id, label string) {
	ui.Button(c.Key(key), label).Disabled(!s.commandRegistry().Available(id)).OnClick(func() { s.runCommand(id) })
}

func (s *shell) renderCommandPalette(c *ui.Context) {
	ui.Column(c.Key("command-palette")).Padding(spaceControl).Gap(spaceControl).Children(func() {
		ui.Row(c).Gap(spaceControl).Children(func() {
			ui.TextInput(c.Key("command-query"), &s.paletteQuery).Label("Search commands").Placeholder("Search workspace commands").Grow(1)
			ui.Button(c, "Close commands").OnClick(func() { s.paletteOpen, s.paletteQuery = false, "" })
		})
		matches := s.commandRegistry().Search(s.paletteQuery)
		if len(matches) == 0 {
			ui.Text(c, "No matching commands")
			return
		}
		ui.Scroll(c).Height(180).Children(func() {
			ui.Column(c).Gap(4).Children(func() {
				for _, match := range matches {
					ui.Button(c.Key(match.ID), "Run: "+match.Title).Disabled(!match.Enabled).OnClick(func() { s.runCommand(match.ID) })
				}
			})
		})
	})
}
