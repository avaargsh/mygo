package main

import (
	"fmt"
	"slices"
	"time"

	"github.com/egoist/mygo/apps/workspace/adapters/model"
	"github.com/egoist/mygo/ui"
)

const maxChatRuns = 64

// chatRunEntry belongs to the shell, independent of the view and its archive.
type chatRunEntry struct {
	id      uint64
	chat    string
	started time.Time
	run     *model.ChatRun
}

func (s *shell) recordChatRun(chatID string, run *model.ChatRun) {
	// Keep running and stopping generations available for inspection.
	if len(s.chatRuns) >= maxChatRuns {
		for i, entry := range s.chatRuns {
			state := entry.run.State()
			if state != model.ChatRunning && state != model.ChatStopping {
				s.chatRuns = slices.Delete(s.chatRuns, i, i+1)
				break
			}
		}
	}
	name := "Chat"
	if session := s.content.Session(chatID); session != nil {
		name = session.Title
	}
	s.nextChatRunID++
	s.chatRuns = append(s.chatRuns, chatRunEntry{
		id: s.nextChatRunID, chat: name, started: time.Now(), run: run,
	})
}

func (s *shell) renderRunHistory(c *ui.Context) {
	ui.Text(c, "Run History").FontSize(fontTitle).Bold()
	if len(s.chatRuns) == 0 {
		ui.Text(c, "No Chat runs in this session.")
		return
	}
	ui.Text(c, "Chat runs in this workspace session.").FontSize(fontSmall)
	active := false
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Gap(spaceControl).Children(func() {
			for i := len(s.chatRuns) - 1; i >= 0; i-- {
				entry := s.chatRuns[i]
				state := entry.run.State()
				active = active || state == model.ChatRunning || state == model.ChatStopping
				ui.Row(c.Key(entry.id)).Gap(spaceControl).Wrap().Children(func() {
					ui.Column(c).Gap(4).Grow(1).Children(func() {
						ui.Text(c, fmt.Sprintf("Run %d · %s", entry.id, state)).Bold()
						ui.Text(c, entry.chat+" · "+entry.started.Format("2006-01-02 15:04:05")).FontSize(fontSmall)
					})
					if state == model.ChatRunning {
						ui.Button(c, fmt.Sprintf("Stop run %d", entry.id)).OnClick(func() { entry.run.Cancel() })
					}
				})
			}
		})
	})
	if active {
		c.After(100 * time.Millisecond)
	}
}
