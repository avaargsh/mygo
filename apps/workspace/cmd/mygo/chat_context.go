package main

import (
	"github.com/egoist/mygo/plugins/xerj"
	"github.com/egoist/mygo/ui"
)

// chatSearchExcerpt is a snapshot of the one passage the user chose for
// this Chat. Its source label is local UI metadata.
type chatSearchExcerpt struct {
	source string
	text   string
}

func (s *shell) renderChatSearchExcerpt(c *ui.Context, ch *chatViewSession) {
	var hits []xerj.Hit
	if search := s.searchSession(); search != nil {
		search.collect()
		if result := search.result; result != nil && result.err == nil && result.request == search.request() {
			hits = result.hits
		}
	}
	ui.Row(c.Key("chat-search-excerpt")).Gap(spaceControl).Wrap().Children(func() {
		if ch.searchExcerpt == nil {
			ui.Button(c, "Attach Search excerpt").Disabled(len(hits) == 0).
				OnClick(func() { ch.searchExcerptOpen = true })
			return
		}
		ui.Text(c, ch.searchExcerpt.source).SingleLine().MaxWidth(240)
		ui.Button(c, "Review attached excerpt").OnClick(func() { ch.searchExcerptOpen = true })
		ui.Button(c, "Remove excerpt").OnClick(func() { ch.searchExcerpt = nil })
	})

	ui.Modal(c.Key("chat-search-excerpt-dialog"), &ch.searchExcerptOpen, func() {
		if excerpt := ch.searchExcerpt; excerpt != nil {
			ui.Text(c, excerpt.source).Bold()
			ui.Scroll(c).Width(480).Height(240).Children(func() { ui.Text(c, excerpt.text) })
			ui.Button(c, "Done").OnClick(func() { ch.searchExcerptOpen = false })
			return
		}
		ui.Text(c, "Choose a Search excerpt").Bold()
		ui.Text(c, "The selected passage is added to your next message when you send it.").FontSize(fontSmall)
		if len(hits) == 0 {
			ui.Text(c, "Run a Search query to get excerpts.")
		} else {
			ui.Scroll(c).Width(480).Height(280).Children(func() {
				ui.Column(c).Gap(spaceSection).Children(func() {
					for i, hit := range hits {
						source := searchSource(hit)
						ui.Column(c.Key(i)).Gap(spaceControl).Children(func() {
							ui.Text(c, source).Bold()
							ui.Text(c, hit.Text)
							ui.Button(c, "Attach excerpt").Label("Attach excerpt from " + source).OnClick(func() {
								ch.searchExcerpt = &chatSearchExcerpt{source: source, text: hit.Text}
								ch.searchExcerptOpen = false
							})
						})
					}
				})
			})
		}
		ui.Button(c, "Cancel").OnClick(func() { ch.searchExcerptOpen = false })
	})
}
