package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// runWebViewSmoke is an opt-in, local-only desktop capability probe.
// It does not read or write the user's workspace, bind services or load remote pages.
func runWebViewSmoke() {
	var a, b *mygo.WebView
	percent := float32(50)
	showBoth := true
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title: "MyGo — two WebViews smoke test", Width: 1100, Height: 700,
			Content: ui.View(func(c *ui.Context) {
				ui.Column(c).Fill().Padding(12).Gap(8).Children(func() {
					ui.Text(c, "Type a different value in each page; resize, hide and restore the second page.")
					ui.Button(c, "Toggle second page").OnClick(func() { showBoth = !showBoth })
					if a == nil || b == nil {
						ui.Text(c, "WebViews not ready")
						return
					}
					left := func() { ui.WebView(c, a).Fill() }
					right := func() { ui.WebView(c, b).Fill() }
					if showBoth {
						ui.SplitPercent(c, &percent, left, right).Grow(1)
					} else {
						ui.WebView(c, a).Grow(1)
					}
				})
			}),
		})
		var err error
		a, err = win.NewWebView(mygo.WebViewOptions{Page: mygo.PageOptions{Untrusted: true}})
		if err != nil {
			log.Fatal(err)
		}
		b, err = win.NewWebView(mygo.WebViewOptions{Page: mygo.PageOptions{Untrusted: true}})
		if err != nil {
			log.Fatal(err)
		}
		a.Page().LoadHTML("<!doctype html><h1>Page A</h1><label>A <input value='A'></label>", "")
		b.Page().LoadHTML("<!doctype html><h1>Page B</h1><label>B <input value='B'></label>", "")
		win.Invalidate()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
