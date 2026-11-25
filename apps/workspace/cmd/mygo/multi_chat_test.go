package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestMultiChatViewsAreIndependentAndRestoreIdle(t *testing.T) {
	s := closeFixture(t)
	tt := ui.NewTester(s.view, 1600, 1000)
	if err := tt.Click("New chat"); err != nil { t.Fatal(err) }
	a := s.workspace.Snapshot()
	firstID := a.Panes[a.ActivePane].Tabs[len(a.Panes[a.ActivePane].Tabs)-1].SessionID
	first := s.content.Session(firstID).Value.(*chatViewSession)
	first.draft, first.apiKey = "first draft", "first-key"
	if err := tt.Click("New chat"); err != nil { t.Fatal(err) }
	b := s.workspace.Snapshot()
	secondID := b.Panes[b.ActivePane].Tabs[len(b.Panes[b.ActivePane].Tabs)-1].SessionID
	second := s.content.Session(secondID).Value.(*chatViewSession)
	if firstID == secondID || first == second || second.draft != "" || second.apiKey != "" {
		t.Fatal("chat instances are not independent")
	}
	if err := s.workspace.Close(b.ActivePane, b.Panes[b.ActivePane].ActiveTab); err != nil { t.Fatal(err) }
	s.open(secondID)
	if s.content.Session(secondID).Value != second || first.draft != "first draft" {
		t.Fatal("closing and reopening a tab replaced a session")
	}
	fresh, err := builtInContent(t.TempDir())
	if err != nil { t.Fatal(err) }
	for _, id := range []string{firstID, secondID} {
		session, err := restoreContentSession(fresh, id)
		if err != nil { t.Fatal(err) }
		ch := session.Value.(*chatViewSession)
		if ch.chat.Snapshot().Active || ch.draft != "" || ch.apiKey != "" {
			t.Fatal("restored chat was not idle and empty")
		}
	}
	if _, err := restoreContentSession(fresh, "chat-instance-../bad"); err == nil {
		t.Fatal("invalid instance restored")
	}
}
