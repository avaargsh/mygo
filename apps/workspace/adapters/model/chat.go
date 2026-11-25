package model

import (
	"context"
	"errors"
	"strings"
	"sync"
	"unicode/utf8"
)

const maxChatTurns = 32
const maxChatPromptBytes = 32 << 10
const maxChatReplyBytes = 256 << 10
const maxChatHistoryBytes = 512 << 10

type ChatState string

const (
	ChatRunning   ChatState = "running"
	ChatStopping  ChatState = "stopping"
	ChatCompleted ChatState = "completed"
	ChatCanceled  ChatState = "canceled"
	ChatFailed    ChatState = "failed"
)

type ChatTurn struct {
	User         string
	Assistant    string
	State        ChatState
	FinishReason string
}

type ChatSnapshot struct {
	Turns  []ChatTurn
	Active bool
}

type StreamFunc func(context.Context, string, []Message, func(string) error) (Completion, error)

// ChatSession is an in-memory conversation. Start, Snapshot, Cancel and Clear
// may be called concurrently. The stream function must emit synchronously and
// respect context cancellation. The optional notify callback must be fast and
// safe to call from a worker goroutine. It is never called under the mutex.
type ChatSession struct {
	mu     sync.Mutex
	turns  []ChatTurn
	cancel context.CancelFunc
}

func (s *ChatSession) Snapshot() ChatSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ChatSnapshot{Turns: append([]ChatTurn(nil), s.turns...), Active: s.cancel != nil}
}

// Start adds one user turn and starts one worker. Only completed prior turns
// are included in the outgoing request. Failed and canceled turns remain local.
func (s *ChatSession) Start(parent context.Context, stream StreamFunc, key, prompt string, notify func()) (<-chan struct{}, error) {
	if stream == nil || strings.TrimSpace(prompt) == "" || !utf8.ValidString(prompt) || len(prompt) > maxChatPromptBytes {
		return nil, errors.New("invalid chat prompt or stream")
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return nil, errors.New("chat generation already active")
	}
	if len(s.turns) >= maxChatTurns {
		return nil, errors.New("chat turn limit reached; clear conversation to continue")
	}
	messages := make([]Message, 0, len(s.turns)*2+1)
	total := len(prompt)
	for _, turn := range s.turns {
		if turn.State == ChatCompleted {
			total += len(turn.User) + len(turn.Assistant)
			messages = append(messages, Message{Role: "user", Content: turn.User}, Message{Role: "assistant", Content: turn.Assistant})
		}
	}
	if total > maxChatHistoryBytes {
		return nil, errors.New("chat history limit reached; clear conversation to continue")
	}
	messages = append(messages, Message{Role: "user", Content: prompt})
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	index := len(s.turns)
	s.turns = append(s.turns, ChatTurn{User: prompt, State: ChatRunning})
	done := make(chan struct{})
	go s.generate(ctx, cancel, stream, key, messages, index, notify, done)
	return done, nil
}

func (s *ChatSession) generate(ctx context.Context, cancel context.CancelFunc, stream StreamFunc, key string, messages []Message, index int, notify func(), done chan struct{}) {
	defer close(done)
	defer cancel()
	var text strings.Builder
	var rejected error
	result, err := stream(ctx, key, messages, func(delta string) error {
		s.mu.Lock()
		if err := ctx.Err(); err != nil {
			s.mu.Unlock()
			return err
		}
		if rejected != nil {
			s.mu.Unlock()
			return rejected
		}
		turn := &s.turns[index]
		if !utf8.ValidString(delta) || len(delta) > maxChatReplyBytes-len(turn.Assistant) {
			rejected = errors.New("invalid or oversized chat reply")
			s.mu.Unlock()
			return rejected
		}
		text.WriteString(delta)
		turn.Assistant = text.String()
		s.mu.Unlock()
		if notify != nil {
			notify()
		}
		return nil
	})
	s.mu.Lock()
	turn := &s.turns[index]
	switch {
	case ctx.Err() != nil:
		turn.State = ChatCanceled
	case rejected != nil || err != nil || result.Text != turn.Assistant:
		turn.State = ChatFailed
	case result.FinishReason != "stop" && result.FinishReason != "length" && result.FinishReason != "content_filter":
		turn.State = ChatFailed
	default:
		turn.State, turn.FinishReason = ChatCompleted, result.FinishReason
	}
	s.cancel = nil
	s.mu.Unlock()
	if notify != nil {
		notify()
	}
}

// Cancel requests stop; the session remains active until the worker exits.
func (s *ChatSession) Cancel() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel == nil || s.turns[len(s.turns)-1].State == ChatStopping {
		return false
	}
	s.turns[len(s.turns)-1].State = ChatStopping
	s.cancel()
	return true
}

func (s *ChatSession) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return errors.New("cannot clear an active chat")
	}
	s.turns = nil
	return nil
}
