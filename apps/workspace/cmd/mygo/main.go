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
	pane core.Pane
}

func (s *shell) open(id, title string) {
	if s.pane.Activate(id) {
		return
	}
	if err := s.pane.Open(core.Tab{ID: id, SessionID: id, Title: title}); err != nil {
		log.Print(err)
	}
}

func (s *shell) view(c *ui.Context) {
	ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
		ui.Row(c).Gap(12).Children(func() {
			ui.Text(c, "MyGo Workspace").FontSize(24).Bold()
			ui.Spacer(c)
			ui.Text(c, "Local workspace").TextColor(c.Theme().TextMuted)
		})
		ui.Row(c).Gap(8).Children(func() {
			ui.Button(c.Key("open-home"), "Home").OnClick(func() { s.open("home", "Home") })
			ui.Button(c.Key("open-runtime"), "Runtime info").OnClick(func() { s.open("runtime", "Runtime info") })
		})
		ui.Row(c).Gap(8).Children(func() {
			for _, tab := range s.pane.Tabs() {
				ui.Row(c.Key(tab.ID)).Gap(4).Children(func() {
					label := tab.Title
					if tab.ID == s.pane.ActiveID() {
						label += " (active)"
					}
					ui.Button(c, label).OnClick(func() { s.pane.Activate(tab.ID) })
					ui.Button(c, "×").Label("Close " + tab.Title).OnClick(func() { s.pane.Close(tab.ID) })
				})
			}
		})
		ui.Column(c).Grow(1).Gap(8).Children(func() {
			switch s.pane.ActiveID() {
			case "home":
				ui.Text(c, "Your Personal AI Workspace").FontSize(28).Bold()
				ui.Text(c, "Open Runtime info to inspect this running application.")
			case "runtime":
				ui.Text(c, "Runtime info").FontSize(20).Bold()
				ui.Text(c, fmt.Sprintf("Platform: %s/%s", runtime.GOOS, runtime.GOARCH))
				ui.Text(c, fmt.Sprintf("Logical CPUs available at startup: %d", runtime.NumCPU()))
				ui.Text(c, "Go runtime: "+runtime.Version())
			default:
				ui.Text(c, "No open tabs. Choose Home or Runtime info.")
			}
		})
		ui.Text(c, "Development build · M0 foundation").FontSize(12).TextColor(c.Theme().TextMuted)
	})
}

func main() {
	s := &shell{}
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
