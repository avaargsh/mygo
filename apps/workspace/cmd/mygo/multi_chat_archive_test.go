package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/apps/workspace/adapters/model"
)

func TestChatInstanceArchivesDoNotOverwriteEachOther(t *testing.T) {
	s := closeFixture(t)
	ids := []string{"chat-instance-00000000000000000000000000000001", "chat-instance-00000000000000000000000000000002"}
	for _, id := range ids {
		session, err := restoreContentSession(s.content, id)
		if err != nil {
			t.Fatal(err)
		}
		ch := session.Value.(*chatViewSession)
		ch.endpoint, ch.modelName = "https://example.invalid/v1/chat/completions", "fixture"
		ch.boundEndpoint, ch.boundModel = ch.endpoint, ch.modelName
		done, err := ch.chat.Start(context.Background(), func(_ context.Context, _ string, _ []model.Message, emit func(string) error) (model.Completion, error) {
			return model.Completion{Text: id, FinishReason: "stop"}, emit(id)
		}, "", id, nil)
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("fixture did not settle")
		}
		ch.archiveConfirm = true
		s.saveChatArchive(ch)
	}
	fresh, err := builtInContent(filepath.Dir(s.statePath))
	if err != nil {
		t.Fatal(err)
	}
	s.content = fresh
	for _, id := range ids {
		session, err := restoreContentSession(fresh, id)
		if err != nil {
			t.Fatal(err)
		}
		ch := session.Value.(*chatViewSession)
		s.loadChatArchive(ch)
		snapshot := ch.chat.Snapshot()
		if len(snapshot.Turns) != 1 || snapshot.Turns[0].Assistant != id || !ch.confirmConnection || snapshot.Active {
			t.Fatal("archive crossed session boundaries or bypassed confirmation")
		}
	}
}
