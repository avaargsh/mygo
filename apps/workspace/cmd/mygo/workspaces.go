package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/ui"
)

// workspaceHost selects the view for the one native window. Each loaded shell
// retains its own layout and live sessions while another one is displayed.
type workspaceHost struct {
	catalog     *core.WorkspaceCatalog
	loaded      map[string]*shell
	current     *shell
	window      *mygo.Window
	chooserOpen bool
	newName     string
	chooserErr  string
}

func newWorkspaceHost(dir string) (*workspaceHost, error) {
	catalog, err := core.OpenWorkspaceCatalog(dir)
	if err != nil {
		return nil, err
	}
	h := &workspaceHost{catalog: catalog, loaded: make(map[string]*shell)}
	profile := catalog.Active()
	initial, err := h.loadWorkspace(profile)
	if err != nil {
		return nil, err
	}
	h.loaded[profile.ID], h.current = initial, initial
	if catalog.MissingSavedWorkspace() {
		h.current.status = "The last workspace folder is missing. Opened Default without changing the saved selection."
	}
	return h, nil
}

func (h *workspaceHost) useWorkspace(profile core.WorkspaceProfile) error {
	s := h.loaded[profile.ID]
	if s == nil {
		var err error
		s, err = h.loadWorkspace(profile)
		if err != nil {
			return err
		}
	}
	keepingDefault := profile.ID == core.DefaultWorkspaceID && h.catalog.MissingSavedWorkspace()
	if err := h.catalog.Select(profile.ID); err != nil {
		return err
	}
	if keepingDefault {
		s.status = "Default will open on startup."
	}
	h.loaded[profile.ID], h.current = s, s
	if h.window != nil {
		applyTheme(s.theme)
	}
	h.chooserOpen, h.newName, h.chooserErr = false, "", ""
	return nil
}

func (h *workspaceHost) loadWorkspace(profile core.WorkspaceProfile) (*shell, error) {
	dir, err := h.catalog.Directory(profile.ID)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "state.json")
	w, theme, err := core.LoadState(path)
	fresh := errors.Is(err, os.ErrNotExist)
	if fresh {
		w, err = core.NewWorkspace("pane-root")
		theme = "system"
	}
	if err != nil {
		return nil, fmt.Errorf("cannot restore workspace %q: %w", profile.Name, err)
	}
	content, err := builtInContent(dir)
	if err != nil {
		return nil, err
	}
	s := &shell{host: h, profile: profile, workspace: w, window: h.window, content: content, statePath: path, theme: theme}
	for _, pane := range w.Snapshot().Panes {
		for _, tab := range pane.Tabs {
			if _, err := restoreContentSession(content, tab.SessionID); err != nil {
				s.status = "Restore content failed: " + err.Error()
				log.Print(s.status)
			}
		}
	}
	if fresh {
		s.open("home")
		s.dirty = false
	}
	return s, nil
}

func (h *workspaceHost) view(c *ui.Context) {
	h.current.view(c.Key(h.current.profile.ID))
}

func (h *workspaceHost) showChooser() {
	h.newName, h.chooserErr = "", ""
	h.chooserOpen = true
	if err := h.catalog.Refresh(); err != nil {
		h.chooserErr = err.Error()
	}
}

func (h *workspaceHost) selectWorkspace(profile core.WorkspaceProfile) {
	if err := h.useWorkspace(profile); err != nil {
		h.chooserErr = err.Error()
	}
}

func (h *workspaceHost) createWorkspace() {
	profile, err := h.catalog.Create(h.newName)
	if err != nil {
		h.chooserErr = err.Error()
		return
	}
	h.newName = ""
	if err := h.useWorkspace(profile); err != nil {
		h.chooserErr = "Workspace created, but could not switch: " + err.Error()
	}
}

func (h *workspaceHost) renderChooser(c *ui.Context) {
	ui.Modal(c.Key("workspace-chooser"), &h.chooserOpen, func() {
		ui.Text(c, "Workspaces").FontSize(fontTitle).Bold()
		width, _ := c.Size()
		ui.Scroll(c).Width(max(1, min(390, width-80))).Height(220).Children(func() {
			ui.Column(c).Gap(spaceControl).Children(func() {
				profiles := h.catalog.Profiles()
				for _, profile := range profiles {
					label := profile.Name
					for _, other := range profiles {
						if other.ID != profile.ID && strings.EqualFold(other.Name, profile.Name) {
							label += " (" + profile.ID[:min(len(profile.ID), 8)] + ")"
							break
						}
					}
					if profile.ID == h.current.profile.ID {
						ui.Text(c.Key(profile.ID), "Current workspace: "+label).Bold()
						if h.catalog.MissingSavedWorkspace() {
							ui.Button(c.Key("keep-default"), "Keep Default on startup").OnClick(func() { h.selectWorkspace(profile) })
						}
					} else {
						ui.Button(c.Key(profile.ID), "Switch to "+label).OnClick(func() { h.selectWorkspace(profile) })
					}
				}
			})
		})
		ui.TextInput(c.Key("workspace-name"), &h.newName).Label("Workspace name").MaxLength(core.MaxWorkspaceNameRunes)
		if h.chooserErr != "" {
			ui.Text(c, h.chooserErr)
		}
		ui.Row(c).Gap(spaceControl).Wrap().Children(func() {
			ui.PrimaryButton(c, "Create workspace").Disabled(strings.TrimSpace(h.newName) == "").OnClick(h.createWorkspace)
			ui.Button(c, "Cancel").OnClick(func() { h.chooserOpen = false })
		})
	})
}

func (h *workspaceHost) confirmClose(dialog func(mygo.MessageOptions) (mygo.MessageResult, error)) bool {
	if !h.current.confirmClose(dialog) {
		return false
	}
	// Disk discovery can drop a profile while its loaded shell still owns
	// unsaved edits. Check the live shells, not the chooser inventory.
	var hidden []*shell
	for _, s := range h.loaded {
		if s != h.current {
			hidden = append(hidden, s)
		}
	}
	slices.SortFunc(hidden, func(a, b *shell) int {
		if aDefault, bDefault := a.profile.ID == core.DefaultWorkspaceID, b.profile.ID == core.DefaultWorkspaceID; aDefault != bDefault {
			if aDefault {
				return -1
			}
			return 1
		}
		if cmp := strings.Compare(strings.ToLower(a.profile.Name), strings.ToLower(b.profile.Name)); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.profile.ID, b.profile.ID)
	})
	for _, s := range hidden {
		status := s.status
		if !s.confirmClose(dialog) {
			if s.status != status {
				h.current.status = s.profile.Name + ": " + s.status
			}
			return false
		}
	}
	return true
}

func (s *shell) closeContent() {
	if search := s.searchSession(); search != nil {
		search.stop()
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
}
