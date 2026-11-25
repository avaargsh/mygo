package main

import (
	"errors"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/ui"
)

// browserSession owns one native WebView, independently of its tab references.
// The window owns final destruction. Users can release the resource explicitly.
type browserSession struct {
	address string
	view    *mygo.WebView
}

func (s *shell) navigateBrowser(b *browserSession) {
	address, err := core.BrowserAddress(b.address)
	if err != nil {
		s.status = err.Error()
		return
	}
	if b.view == nil {
		if s.window == nil {
			s.status = errors.New("desktop window not ready").Error()
			return
		}
		view, err := s.window.NewWebView(mygo.WebViewOptions{
			Page: mygo.PageOptions{Untrusted: true},
		})
		if err != nil {
			s.status = "Browser start failed: " + err.Error()
			return
		}
		// Untrusted pages have no native bridge privileges. Keep the default
		// denial of permissions, downloads and popups. The navigation callback
		// only permits explicit web URLs; it does not sandbox network requests.
		view.Page().OnWillNavigate(func(e *mygo.NavigateEvent) {
			if _, err := core.BrowserAddress(e.URL); err != nil {
				e.PreventDefault()
			}
		})
		b.view = view
	}
	b.address = address
	b.view.Page().LoadURL(address)
	s.status = ""
}

func (s *shell) releaseBrowser(b *browserSession) {
	if b.view != nil {
		b.view.Destroy()
		b.view = nil
	}
	s.status = "Browser page closed; tab references retained"
}

func (s *shell) renderBrowser(c *ui.Context, snapshot core.WorkspaceSnapshot, paneID string, b *browserSession) {
	if core.SessionOwner(snapshot, "browser") != paneID {
		ui.Text(c, "Browser is displayed in another pane.")
		ui.Button(c, "Show browser here").OnClick(func() { s.apply(s.workspace.Focus(paneID)) })
		return
	}
	ui.Row(c).Gap(spaceControl).Wrap().Children(func() {
		ui.Button(c, "Back").Disabled(b.view == nil).OnClick(func() {
			if b.view != nil {
				b.view.Page().GoBack()
			}
		})
		ui.Button(c, "Forward").Disabled(b.view == nil).OnClick(func() {
			if b.view != nil {
				b.view.Page().GoForward()
			}
		})
		ui.Button(c, "Reload").Disabled(b.view == nil).OnClick(func() {
			if b.view != nil {
				b.view.Page().Reload()
			}
		})
		ui.Button(c, "Use current URL").Disabled(b.view == nil).OnClick(func() {
			if b.view != nil {
				b.address = b.view.Page().URL()
			}
		})
		ui.Button(c, "Close browser page").Disabled(b.view == nil).OnClick(func() { s.releaseBrowser(b) })
	})
	ui.Row(c).Gap(spaceControl).Children(func() {
		if ui.TextInput(c, &b.address).Label("Browser address").Placeholder("https://example.org").Grow(1).Submitted() {
			s.navigateBrowser(b)
		}
		ui.Button(c, "Go").OnClick(func() { s.navigateBrowser(b) })
	})
	if b.view != nil {
		ui.WebView(c, b.view).Grow(1).MinHeight(0)
	} else {
		ui.Text(c, "Enter a web address and choose Go. No page loads automatically.")
	}
}
