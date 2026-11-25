package model

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const archiveEndpoint = "https://example.invalid/v1/chat/completions"

func TestChatArchiveRoundTrip(t *testing.T) {
	s := &ChatSession{turns: []ChatTurn{
		{User: "hello", Assistant: "world", State: ChatCompleted, FinishReason: "stop"},
		{User: "second", Assistant: "partial", State: ChatCanceled},
		{User: "third", State: ChatFailed},
	}}
	path := filepath.Join(t.TempDir(), "chat.json")
	if err := SaveChatArchive(path, archiveEndpoint, "test-model", s); err != nil {
		t.Fatal(err)
	}
	restored, endpoint, model, err := LoadChatArchive(path)
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != archiveEndpoint || model != "test-model" || !reflect.DeepEqual(s.Snapshot(), restored.Snapshot()) {
		t.Fatal("archive did not restore the original conversation")
	}
	done, err := restored.Start(context.Background(), func(_ context.Context, _ string, m []Message, emit func(string) error) (Completion, error) {
		want := []Message{{Role: "user", Content: "hello"}, {Role: "assistant", Content: "world"}, {Role: "user", Content: "next"}}
		if !reflect.DeepEqual(m, want) {
			t.Error("incomplete history leaked into next request")
		}
		return Completion{Text: "ok", FinishReason: "stop"}, emit("ok")
	}, "", "next", nil)
	if err != nil {
		t.Fatal(err)
	}
	awaitChat(t, done)
}

func TestChatArchiveRejectsInvalidWithoutOverwriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chat.json")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, turn := range []ChatTurn{
		{User: "x", State: ChatRunning},
		{User: "x", State: ChatStopping},
		{User: "x", State: ChatCompleted},
		{User: "x", State: ChatCanceled, FinishReason: "stop"},
		{User: " ", State: ChatFailed},
		{User: strings.Repeat("x", maxChatPromptBytes+1), State: ChatFailed},
	} {
		s := &ChatSession{turns: []ChatTurn{turn}}
		if err := SaveChatArchive(path, archiveEndpoint, "model", s); err == nil {
			t.Fatalf("accepted invalid turn: %s", turn.State)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != "original" {
			t.Fatal("invalid save replaced existing file")
		}
	}
}

func TestChatArchiveStrictLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chat.json")
	valid := `{"version":1,"endpoint":"https://example.invalid/v1/chat/completions","model":"model","turns":[{"User":"x","Assistant":"y","State":"completed","FinishReason":"stop"}]}`
	for _, invalid := range []string{
		strings.Replace(valid, `"version":1`, `"version":2`, 1),
		strings.Replace(valid, `"version":1`, `"version":1,"apiKey":"secret"`, 1),
		strings.Replace(valid, "completed", "running", 1),
		strings.Replace(valid, "https://", "http://", 1),
		valid + "{}",
		"null",
		strings.Repeat(" ", maxChatArchiveBytes+1),
	} {
		if err := os.WriteFile(path, []byte(invalid), 0600); err != nil {
			t.Fatal(err)
		}
		if s, _, _, err := LoadChatArchive(path); err == nil || s != nil {
			t.Fatal("invalid archive accepted")
		}
	}
}
