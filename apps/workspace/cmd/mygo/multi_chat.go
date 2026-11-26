package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/ui"
)

const chatInstancePrefix = "chat-instance-"
const maxChatInstances = 16

func validChatInstanceID(id string) bool {
	if !strings.HasPrefix(id, chatInstancePrefix) {
		return false
	}
	suffix := strings.TrimPrefix(id, chatInstancePrefix)
	if len(suffix) != 32 || strings.ToLower(suffix) != suffix {
		return false
	}
	_, err := hex.DecodeString(suffix)
	return err == nil
}

func restoreContentSession(m *core.ContentManager, id string) (*core.ContentSession, error) {
	if !validChatInstanceID(id) {
		return m.Open(id)
	}
	if existing := m.Session(id); existing != nil {
		return existing, nil
	}
	count := 0
	for _, s := range m.SessionsForProvider("chat") {
		if s.ID != "chat" {
			count++
		}
	}
	if count >= maxChatInstances {
		return nil, errors.New("chat instance limit reached")
	}
	session, err := m.OpenInstance("chat", id)
	if err != nil {
		return nil, err
	}
	session.Title = "Chat " + strings.TrimPrefix(id, chatInstancePrefix)[:8]
	return session, nil
}

func (s *shell) newChat() {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		s.status = "Could not generate a chat session ID"
		return
	}
	id := chatInstancePrefix + hex.EncodeToString(random[:])
	if s.content.Session(id) != nil {
		s.status = "Chat session ID collision; retry"
		return
	}
	if _, err := restoreContentSession(s.content, id); err != nil {
		s.status = err.Error()
		return
	}
	s.open(id)
}

func chatIdentity(ch *chatViewSession) string {
	if ch.sessionID == "" {
		return "chat"
	}
	return ch.sessionID
}

func (s *shell) closeChats() {
	for _, session := range s.content.SessionsForProvider("chat") {
		if ch, ok := session.Value.(*chatViewSession); ok && ch != nil {
			ch.chat.Cancel()
			ch.apiKey = ""
			ch.searchExcerpt, ch.searchExcerptOpen = nil, false
		}
	}
}

// Hidden conversations retain their state, but can be reopened and stopped
// explicitly. No hidden session starts a generation by itself.
func (s *shell) renderHiddenChats(c *ui.Context, snapshot core.WorkspaceSnapshot) {
	for _, session := range s.content.SessionsForProvider("chat") {
		ch, ok := session.Value.(*chatViewSession)
		if !ok || ch == nil || core.SessionOwner(snapshot, session.ID) != "" {
			continue
		}
		ui.Row(c.Key("hidden-"+session.ID)).Gap(spaceControl).Wrap().Children(func() {
			ui.Button(c, "Reopen "+session.Title).OnClick(func() { s.open(session.ID) })
			if ch.chat.Snapshot().Active {
				c.After(100 * time.Millisecond)
				ui.Text(c, "Generating").FontSize(fontSmall)
				ui.Button(c, "Stop "+session.Title).OnClick(func() { ch.chat.Cancel() })
			}
		})
	}
}
