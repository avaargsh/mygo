package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/apps/workspace/adapters/model"
	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

func TestWorkspaceChatUsesOnlySelectedSearchExcerpt(t *testing.T) {
	const selected = "The selected fixture excerpt."
	const other = "This other passage must stay local."
	const firstPrompt = "Summarize my selection."
	const nextPrompt = "Continue."

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fixture-index/_search" {
			t.Errorf("search path = %q", r.URL.Path)
		}
		fmt.Fprint(w, `{"hits":{"hits":[{"_index":"fixture-index","_source":{"path":"first.md","line":3},"fields":{"_passage":[{"text":"The selected fixture excerpt.","start_offset":0}]}},{"_index":"fixture-index","_source":{"path":"second.md","line":5},"fields":{"_passage":[{"text":"This other passage must stay local.","start_offset":0}]}}]}}`)
	}))
	defer searchServer.Close()

	requests := make(chan []model.Message, 2)
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []model.Message
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("model request: %v", err)
		}
		select {
		case requests <- body.Messages:
		default:
			t.Error("unexpected extra model request")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"reply\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer modelServer.Close()

	s := closeFixture(t)
	s.open("search")
	f := s.searchSession()
	f.endpoint, f.index, f.query = searchServer.URL, "fixture-index", "private search query"
	tt := ui.NewTester(s.view, 1200, 800)
	click := func(label string) {
		t.Helper()
		tt.Frame()
		if err := tt.Click(label); err != nil {
			t.Fatalf("click %q: %v", label, err)
		}
	}
	click("Find passages")
	wait.Until(t, "search results to arrive", func() bool {
		f.collect()
		return f.result != nil
	})

	s.open("chat")
	ch := s.chatSession()
	ch.endpoint, ch.modelName = modelServer.URL+"/chat/completions", "fixture"
	searchQuery := f.query
	f.query = "changed search query"
	tt.Frame()
	if button, ok := tt.Access("Attach Search excerpt"); !ok || !button.Disabled {
		t.Fatal("a stale result was offered for attachment")
	}
	f.query = searchQuery
	choose := func() {
		click("Attach Search excerpt")
		click(`Attach excerpt from "first.md":3`)
	}
	choose()
	click("Remove excerpt")
	choose()
	if canLoadChatArchive(ch) || ch.chat.Snapshot().Active || len(requests) != 0 {
		t.Fatal("selecting an excerpt sent data or allowed archive load to discard it")
	}
	s.save()
	state, err := os.ReadFile(s.statePath)
	if err != nil || strings.Contains(string(state), selected) {
		t.Fatalf("workspace state contains the pending excerpt or failed to save: %v", err)
	}

	s.newChat()
	if s.chatSession() != ch {
		t.Fatal("creating another Chat replaced the default instance")
	}
	tt.Frame()
	if _, ok := tt.Access("Review attached excerpt"); ok {
		t.Fatal("the attachment appeared in another Chat instance")
	}
	s.open("chat")
	ch.draft = firstPrompt
	click("Send message with excerpt")
	first := wait.For(t, "first model request", requests)
	if len(first) != 1 || first[0].Role != "user" || !strings.HasPrefix(first[0].Content, firstPrompt+"\n") || !strings.HasSuffix(first[0].Content, selected) {
		t.Fatalf("unexpected request with selected excerpt: %+v", first)
	}
	for _, private := range []string{other, "first.md", "fixture-index", f.query, f.endpoint} {
		if strings.Contains(first[0].Content, private) {
			t.Fatalf("the request included unselected or private Search data %q", private)
		}
	}
	wait.Until(t, "first chat reply to finish", func() bool { return !ch.chat.Snapshot().Active })
	if turn := ch.chat.Snapshot().Turns[0]; turn.User != firstPrompt || turn.State != model.ChatCompleted {
		t.Fatalf("the scoped excerpt changed the stored user turn: %+v", turn)
	}

	ch.draft = nextPrompt
	click("Send message")
	followUp := wait.For(t, "follow-up model request", requests)
	want := []model.Message{
		{Role: "user", Content: firstPrompt},
		{Role: "assistant", Content: "reply"},
		{Role: "user", Content: nextPrompt},
	}
	if !reflect.DeepEqual(followUp, want) {
		t.Fatalf("follow-up messages = %+v, want %+v", followUp, want)
	}
	wait.Until(t, "follow-up chat reply to finish", func() bool { return !ch.chat.Snapshot().Active })
}
