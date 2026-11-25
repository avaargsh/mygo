package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Workspace-only tokens: the generic framework theme remains unchanged.
const (
	spacePage    = 16
	spaceSection = 12
	spaceControl = 8
	radiusPane   = 8
	fontTitle    = 24
	fontSmall    = 12
)

func applyTheme(mode string) {
	switch mode {
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}

func workspaceTheme(c *ui.Context) *ui.Theme {
	base := ui.LightTheme()
	if mygo.Theme.IsDark() {
		base = ui.DarkTheme()
	}
	// Preserve platform text scale and contrast preferences. The workspace
	// uses its own accent only inside the product's themed subtree.
	t := base.WithPreferences(c.Preferences()).WithAccent(ui.Hex("#10a37f"))
	t.Radius = radiusPane
	t.Spacing = 4
	return t
}
