package main

import (
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/ui"
)

// filesSession retains an explicitly selected root only in memory.
type filesSession struct {
	root    *os.Root
	browser *core.FileBrowser
	label   string
}

func (f *filesSession) close() error {
	if f.root == nil {
		return nil
	}
	root := f.root
	f.root, f.browser, f.label = nil, nil, ""
	return root.Close()
}

func (s *shell) selectFilesRoot(f *filesSession) {
	if s.window == nil {
		s.status = "desktop window not ready"
		return
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: s.window, Title: "Choose a directory to browse", Directory: true,
	})
	if err != nil {
		s.status = "Directory selection failed: " + err.Error()
		return
	}
	if len(paths) == 0 {
		return
	}
	root, err := os.OpenRoot(paths[0])
	if err != nil {
		s.status = "Directory open failed: " + err.Error()
		return
	}
	browser, err := core.NewFileBrowser(root.FS())
	if err != nil {
		root.Close()
		s.status = "Directory listing failed: " + err.Error()
		return
	}
	closeErr := f.close()
	f.root, f.browser, f.label = root, browser, paths[0]
	s.fileResult(closeErr)
}

func (s *shell) fileResult(err error) {
	s.status = ""
	if err != nil {
		s.status = "Files: " + err.Error()
	}
}

func (s *shell) filesSession() *filesSession {
	if v := s.content.Session("files"); v != nil {
		f, _ := v.Value.(*filesSession)
		return f
	}
	return nil
}

func (s *shell) renderFiles(c *ui.Context, f *filesSession) {
	ui.Row(c).Gap(spaceControl).Wrap().Children(func() {
		ui.Button(c, "Choose folder").OnClick(func() { s.selectFilesRoot(f) })
		ui.Button(c, "Revoke folder access").Disabled(f.browser == nil).OnClick(func() { s.fileResult(f.close()) })
	})
	if f.browser == nil {
		ui.Text(c, "Choose a folder to browse. No folder is accessed automatically.")
		return
	}
	b := f.browser
	ui.Text(c, f.label)
	ui.Text(c, b.Path())
	ui.Row(c).Gap(spaceControl).Children(func() {
		ui.Button(c, "Parent folder").Disabled(b.Path() == ".").OnClick(func() { s.fileResult(b.Up()) })
		ui.Button(c, "Refresh files").OnClick(func() { s.fileResult(b.Refresh()) })
	})
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Gap(4).Children(func() {
			for _, entry := range b.Entries() {
				ui.Row(c.Key(entry.Name)).Children(func() {
					if entry.Directory {
						ui.Button(c, "Folder: "+entry.Name).OnClick(func() { s.fileResult(b.Enter(entry.Name)) })
					} else {
						ui.Text(c, entry.Name)
					}
				})
			}
		})
	})
}
