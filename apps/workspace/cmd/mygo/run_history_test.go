package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/apps/workspace/adapters/model"
	"github.com/egoist/mygo/apps/workspace/core"
	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

func TestWorkspaceRunHistoryStopsOneHiddenChat(t *testing.T) {
	started, stopped := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/blocked/chat/completions":
			_, _ = io.Copy(io.Discard, r.Body)
			close(started)
			<-r.Context().Done()
			close(stopped)
		case "/complete/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"fixture reply\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		default:
			t.Errorf("unexpected model path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	s := closeFixture(t)
	defer s.closeChats()
	s.open("chat")
	first := s.chatSession()
	first.endpoint = server.URL + "/blocked/chat/completions"
	first.modelName, first.draft = "private-model", "private draft for the first run"
	tt := ui.NewTester(s.view, 1200, 800)
	click := func(label string) {
		t.Helper()
		tt.Frame()
		if err := tt.Click(label); err != nil {
			t.Fatalf("click %q: %v", label, err)
		}
	}
	click("Send message")
	wait.For(t, "first model request to start", started)

	click("New chat")
	snapshot := s.workspace.Snapshot()
	tabs := snapshot.Panes[snapshot.ActivePane].Tabs
	second := s.content.Session(tabs[len(tabs)-1].SessionID).Value.(*chatViewSession)
	second.endpoint = server.URL + "/complete/chat/completions"
	second.modelName, second.draft = "private-model", "private draft for the second run"
	click("Send message")
	wait.Until(t, "second Chat to finish", func() bool { return !second.chat.Snapshot().Active })

	click("Run History")
	tt.Frame()
	for _, label := range []string{"Run 1 · running", "Run 2 · completed", "Stop run 1"} {
		if _, ok := tt.Access(label); !ok {
			t.Errorf("missing %q", label)
		}
	}
	for _, node := range tt.AccessAll() {
		for _, private := range []string{server.URL, "private-model", "private draft", "fixture reply"} {
			if strings.Contains(node.Name, private) {
				t.Errorf("Run History exposed %q in %q", private, node.Name)
			}
		}
	}
	click("Stop run 1")
	wait.For(t, "the first HTTP request to be canceled", stopped)
	wait.Until(t, "Run History to display cancellation", func() bool {
		tt.Frame()
		_, ok := tt.Access("Run 1 · canceled")
		return ok
	})
	if _, ok := tt.Access("Run 2 · completed"); !ok {
		t.Fatal("stopping the first run changed the completed run")
	}
	s.clearChat(first)
	tt.Frame()
	if _, ok := tt.Access("Run 1 · canceled"); !ok {
		t.Fatal("clearing the conversation erased its run outcome")
	}

	s.save()
	data, err := os.ReadFile(s.statePath)
	if err != nil || strings.Contains(string(data), "canceled") || strings.Contains(string(data), "private draft") {
		t.Fatalf("run data was persisted or saving failed: %v", err)
	}
	layout, _, err := core.LoadState(s.statePath)
	if err != nil {
		t.Fatal(err)
	}
	restored := closeFixture(t)
	restored.workspace = layout
	if _, err := restoreContentSession(restored.content, "runs"); err != nil {
		t.Fatal(err)
	}
	tt = ui.NewTester(restored.view, 1200, 800)
	tt.Frame()
	if _, ok := tt.Access("No Chat runs in this session."); !ok {
		t.Fatal("Run History did not reopen empty")
	}
}

func TestRunHistoryPrunesFinishedRunsBeforeActiveOnes(t *testing.T) {
	s := closeFixture(t)
	var activeChat, otherChat model.ChatSession
	active, err := activeChat.StartRun(t.Context(), func(ctx context.Context, _ string, _ []model.Message, _ func(string) error) (model.Completion, error) {
		<-ctx.Done()
		return model.Completion{}, ctx.Err()
	}, "", "pending", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer active.Cancel()
	s.recordChatRun("chat", active)

	for range maxChatRuns {
		run, err := otherChat.StartRun(t.Context(), func(_ context.Context, _ string, _ []model.Message, _ func(string) error) (model.Completion, error) {
			return model.Completion{FinishReason: "stop"}, nil
		}, "", "question", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		wait.Until(t, "the run to complete", func() bool { return run.State() == model.ChatCompleted })
		s.recordChatRun("chat", run)
		if err := otherChat.Clear(); err != nil {
			t.Fatal(err)
		}
	}

	if len(s.chatRuns) != maxChatRuns || s.chatRuns[0].run != active || s.chatRuns[1].id != 3 {
		t.Fatal("retention removed the active run, kept the oldest finished run, or exceeded the limit")
	}
}
