package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/apps/workspace/adapters/model"
	"github.com/egoist/mygo/ui"
)

func TestWorkspaceChatArchiveExplicitSaveLoad(t *testing.T) {
	s := closeFixture(t)
	s.open("chat")
	ch := s.chatSession()
	ch.endpoint, ch.modelName = "https://example.invalid/v1/chat/completions", "fixture"
	ch.boundEndpoint, ch.boundModel = ch.endpoint, ch.modelName
	done, err := ch.chat.Start(context.Background(), func(_ context.Context, _ string, _ []model.Message, emit func(string) error) (model.Completion, error) {
		return model.Completion{Text: "reply", FinishReason: "stop"}, emit("reply")
	}, "", "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("fixture did not settle")
	}
	ch.apiKey, ch.draft = "credential-fixture", "unsent-fixture"
	path := filepath.Join(filepath.Dir(s.statePath), "chat.json")
	tt := ui.NewTester(s.view, 1600, 1000)
	if err := tt.Click("Save chat archive"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("saved without confirmation")
	}
	if err := tt.Click("Confirm archive save"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		t.Fatalf("archive not saved: %v", err)
	}
	restored, err := builtInContent(filepath.Dir(s.statePath))
	if err != nil {
		t.Fatal(err)
	}
	s.content = restored
	s.open("chat")
	ch = s.chatSession()
	if len(ch.chat.Snapshot().Turns) != 0 {
		t.Fatal("archive was loaded automatically")
	}
	tt = ui.NewTester(s.view, 1600, 1000)
	if err := tt.Click("Load chat archive"); err != nil {
		t.Fatal(err)
	}
	ch = s.chatSession()
	if len(ch.chat.Snapshot().Turns) != 1 || ch.apiKey != "" || ch.draft != "" || !ch.confirmConnection {
		t.Fatal("archive did not restore safely")
	}
	ch.draft = "next"
	s.sendChat(ch)
	if ch.chat.Snapshot().Active || ch.draft != "next" {
		t.Fatal("unconfirmed connection allowed sending")
	}
	tt = ui.NewTester(s.view, 1600, 1000)
	if err := tt.Click("Confirm restored model"); err != nil {
		t.Fatal(err)
	}
	if ch.confirmConnection {
		t.Fatal("connection confirmation did not take effect")
	}
}
