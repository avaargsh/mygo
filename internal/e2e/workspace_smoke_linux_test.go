//go:build linux && !android

package e2e

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestWorkspaceNativeTwoWebViews(t *testing.T) {
	var a, b *mygo.WebView
	showBoth := true
	percent := float32(50)
	w := newWindow(t, mygo.WindowOptions{
		Title: "Workspace acceptance", Width: 900, Height: 500,
		Content: ui.View(func(c *ui.Context) {
			if a == nil || b == nil {
				return
			}
			if showBoth {
				ui.SplitPercent(c, &percent, func() { ui.WebView(c, a).Fill() }, func() { ui.WebView(c, b).Fill() }).Fill()
			} else {
				ui.WebView(c, a).Fill()
			}
		}),
	})
	va, err := w.NewWebView(mygo.WebViewOptions{Page: mygo.PageOptions{Untrusted: true}})
	if err != nil {
		t.Fatal(err)
	}
	vb, err := w.NewWebView(mygo.WebViewOptions{Page: mygo.PageOptions{Untrusted: true}})
	if err != nil {
		t.Fatal(err)
	}
	w.Update(func() { a, b = va, vb })
	va.Page().LoadHTML("<input id='value' value='A'>", "")
	vb.Page().LoadHTML("<input id='value' value='B'>", "")
	waitForPage(t, va.Page(), "document.getElementById('value')")
	waitForPage(t, vb.Page(), "document.getElementById('value')")
	if _, err := va.Page().Eval("document.getElementById('value').value = 'edited'"); err != nil {
		t.Fatal(err)
	}
	w.Update(func() { percent = 65; showBoth = false })
	w.Update(func() { showBoth = true })
	for _, item := range []struct {
		view *mygo.WebView
		want string
	}{{va, "edited"}, {vb, "B"}} {
		waitForPage(t, item.view.Page(), "document.getElementById('value')")
		got, err := mygo.EvalAs[string](item.view.Page(), "document.getElementById('value').value")
		if err != nil || got != item.want {
			t.Fatalf("value = %q, want %q, err=%v", got, item.want, err)
		}
	}
	w.Update(func() { a, b = nil, nil })
	va.Destroy()
	vb.Destroy()
	if !va.IsDestroyed() || !vb.IsDestroyed() {
		t.Fatal("native views were not destroyed")
	}
}
