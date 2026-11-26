package model

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func awaitChat(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("chat worker did not finish")
	}
}

func TestChatSessionStreamsAndBuildsCompletedHistory(t *testing.T) {
	var s ChatSession
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, streamText)
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/v1/chat/completions", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	done, err := s.Start(context.Background(), client.Stream, "", "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	awaitChat(t, done)
	first := s.Snapshot()
	if first.Active || len(first.Turns) != 1 || first.Turns[0].State != ChatCompleted || first.Turns[0].Assistant != "hello world" {
		t.Fatalf("unexpected snapshot: %+v", first)
	}
	first.Turns[0].Assistant = "modified"
	var received []Message
	done, err = s.Start(context.Background(), func(_ context.Context, _ string, m []Message, emit func(string) error) (Completion, error) {
		received = m
		return Completion{Text: "next", FinishReason: "stop"}, emit("next")
	}, "", "follow-up", nil)
	if err != nil {
		t.Fatal(err)
	}
	awaitChat(t, done)
	want := []Message{{Role: "user", Content: "hello"}, {Role: "assistant", Content: "hello world"}, {Role: "user", Content: "follow-up"}}
	if !reflect.DeepEqual(received, want) {
		t.Fatalf("request history: %+v", received)
	}
}

func TestChatSessionCancelFencesWorkerAndKeepsPartial(t *testing.T) {
	var s ChatSession
	ready, release := make(chan struct{}), make(chan struct{})
	stream := func(ctx context.Context, _ string, _ []Message, emit func(string) error) (Completion, error) {
		if err := emit("partial"); err != nil {
			return Completion{}, err
		}
		close(ready)
		<-ctx.Done()
		<-release
		return Completion{}, ctx.Err()
	}
	done, err := s.Start(context.Background(), stream, "", "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	awaitChat(t, ready)
	if snap := s.Snapshot(); !snap.Active || snap.Turns[0].Assistant != "partial" {
		t.Fatalf("no incremental snapshot: %+v", snap)
	}
	if !s.Cancel() {
		t.Fatal("cancel not accepted")
	}
	if snap := s.Snapshot(); !snap.Active || snap.Turns[0].State != ChatStopping {
		t.Fatalf("not stopping: %+v", snap)
	}
	if _, err := s.Start(context.Background(), stream, "", "duplicate", nil); err == nil {
		t.Fatal("duplicate worker accepted")
	}
	if err := s.Clear(); err == nil {
		t.Fatal("cleared active worker")
	}
	close(release)
	awaitChat(t, done)
	if snap := s.Snapshot(); snap.Active || snap.Turns[0].State != ChatCanceled || snap.Turns[0].Assistant != "partial" {
		t.Fatalf("partial lost: %+v", snap)
	}
}

func TestChatSessionFailureDoesNotResendIncompleteTurn(t *testing.T) {
	var s ChatSession
	done, err := s.Start(context.Background(), func(_ context.Context, _ string, _ []Message, emit func(string) error) (Completion, error) {
		if err := emit("partial"); err != nil {
			return Completion{}, err
		}
		return Completion{}, errors.New("sensitive remote error")
	}, "", "failed prompt", nil)
	if err != nil {
		t.Fatal(err)
	}
	awaitChat(t, done)
	if snap := s.Snapshot(); snap.Turns[0].State != ChatFailed || snap.Turns[0].Assistant != "partial" {
		t.Fatalf("failure lost: %+v", snap)
	}
	var received []Message
	done, err = s.Start(context.Background(), func(_ context.Context, _ string, m []Message, emit func(string) error) (Completion, error) {
		received = m
		return Completion{Text: "", FinishReason: "stop"}, nil
	}, "", "new prompt", nil)
	if err != nil {
		t.Fatal(err)
	}
	awaitChat(t, done)
	if len(received) != 1 || received[0].Content != "new prompt" {
		t.Fatalf("incomplete history resent: %+v", received)
	}
	if err := s.Clear(); err != nil || len(s.Snapshot().Turns) != 0 {
		t.Fatal("clear failed")
	}
}

func TestChatSessionRejectsInvalidAndOversizedOutput(t *testing.T) {
	var s ChatSession
	if _, err := s.Start(context.Background(), nil, "", "hello", nil); err == nil {
		t.Fatal("nil stream accepted")
	}
	stream := func(_ context.Context, _ string, _ []Message, emit func(string) error) (Completion, error) {
		return Completion{}, emit(string(make([]byte, maxChatReplyBytes+1)))
	}
	if _, err := s.Start(context.Background(), stream, "", " ", nil); err == nil {
		t.Fatal("blank prompt accepted")
	}
	done, err := s.Start(context.Background(), stream, "", "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	awaitChat(t, done)
	if snap := s.Snapshot(); snap.Turns[0].State != ChatFailed || snap.Turns[0].Assistant != "" {
		t.Fatalf("oversized output accepted: %+v", snap)
	}
}

func TestChatSessionRejectedDeltaCannotBecomeCompleted(t *testing.T) {
	var s ChatSession
	done, err := s.Start(context.Background(), func(_ context.Context, _ string, _ []Message, emit func(string) error) (Completion, error) {
		if err := emit("valid"); err != nil {
			return Completion{}, err
		}
		if err := emit(string([]byte{0xff})); err == nil {
			return Completion{}, errors.New("invalid delta was accepted")
		}
		// A faulty adapter ignores the callback error and claims completion.
		return Completion{Text: "valid", FinishReason: "stop"}, nil
	}, "", "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	awaitChat(t, done)
	turn := s.Snapshot().Turns[0]
	if turn.State != ChatFailed || turn.Assistant != "valid" || turn.FinishReason != "" {
		t.Fatalf("rejected delta was treated as completion: %+v", turn)
	}
}

func TestChatSessionRejectsExcerptOutsidePromptBudget(t *testing.T) {
	stream := func(_ context.Context, _ string, _ []Message, _ func(string) error) (Completion, error) {
		t.Error("invalid excerpt started a request")
		return Completion{}, errors.New("unexpected request")
	}
	for _, tc := range []struct {
		name, prompt, excerpt string
	}{
		{"oversized combined message", strings.Repeat("a", maxChatPromptBytes), "reference"},
		{"invalid UTF-8", "hello", string([]byte{0xff})},
		{"blank excerpt", "hello", "  "},
		{"empty prompt", "  ", "reference"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var chat ChatSession
			done, err := chat.StartWithExcerpt(t.Context(), stream, "", tc.prompt, tc.excerpt, nil)
			if err == nil || done != nil || len(chat.Snapshot().Turns) != 0 {
				t.Fatal("invalid excerpt or prompt changed the session")
			}
		})
	}
}
