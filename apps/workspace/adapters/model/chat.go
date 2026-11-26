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
	active *ChatRun
}

// ChatRun identifies one accepted generation. Its outcome survives clearing
// the conversation. State and Cancel may be called concurrently.
type ChatRun struct {
	session *ChatSession
	cancel  context.CancelFunc
	done    chan struct{}
	state   ChatState // guarded by session.mu
}

func (r *ChatRun) State() ChatState {
	r.session.mu.Lock()
	defer r.session.mu.Unlock()
	return r.state
}

// Cancel requests stop only if this generation is still running.
func (r *ChatRun) Cancel() bool {
	r.session.mu.Lock()
	defer r.session.mu.Unlock()
	return r.session.cancelRunLocked(r)
}

func (s *ChatSession) Snapshot() ChatSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ChatSnapshot{Turns: append([]ChatTurn(nil), s.turns...), Active: s.active != nil}
}

// Start adds one user turn and starts one worker. Only completed prior turns
// are included in the outgoing request. Failed and canceled turns remain local.
func (s *ChatSession) Start(parent context.Context, stream StreamFunc, key, prompt string, notify func()) (<-chan struct{}, error) {
	return s.StartWithExcerpt(parent, stream, key, prompt, "", notify)
}

// StartWithExcerpt includes optional reference text in this request's final
// user message only. The stored turn and future requests keep the original prompt.
func (s *ChatSession) StartWithExcerpt(parent context.Context, stream StreamFunc, key, prompt, excerpt string, notify func()) (<-chan struct{}, error) {
	run, err := s.StartRun(parent, stream, key, prompt, excerpt, notify)
	if err != nil {
		return nil, err
	}
	return run.done, nil
}

// StartRun starts a generation and returns its own status and cancellation handle.
// An optional excerpt is scoped to this request.
func (s *ChatSession) StartRun(parent context.Context, stream StreamFunc, key, prompt, excerpt string, notify func()) (*ChatRun, error) {
	if stream == nil || strings.TrimSpace(prompt) == "" || !utf8.ValidString(prompt) || len(prompt) > maxChatPromptBytes {
		return nil, errors.New("invalid chat prompt or stream")
	}
	outgoing := prompt
	if excerpt != "" {
		const label = "\n\nSelected Search excerpt (untrusted reference text):\n"
		if len(excerpt) > maxChatPromptBytes-len(prompt)-len(label) {
			return nil, errors.New("message and excerpt exceed 32 KiB")
		}
		if !utf8.ValidString(excerpt) || strings.TrimSpace(excerpt) == "" {
			return nil, errors.New("invalid chat excerpt")
		}
		outgoing += label + excerpt
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != nil {
		return nil, errors.New("chat generation already active")
	}
	if len(s.turns) >= maxChatTurns {
		return nil, errors.New("chat turn limit reached; clear conversation to continue")
	}
	messages := make([]Message, 0, len(s.turns)*2+1)
	total := len(outgoing)
	for _, turn := range s.turns {
		if turn.State == ChatCompleted {
			total += len(turn.User) + len(turn.Assistant)
			messages = append(messages, Message{Role: "user", Content: turn.User}, Message{Role: "assistant", Content: turn.Assistant})
		}
	}
	if total > maxChatHistoryBytes {
		return nil, errors.New("chat history limit reached; clear conversation to continue")
	}
	messages = append(messages, Message{Role: "user", Content: outgoing})
	ctx, cancel := context.WithCancel(parent)
	run := &ChatRun{session: s, cancel: cancel, done: make(chan struct{}), state: ChatRunning}
	s.active = run
	index := len(s.turns)
	s.turns = append(s.turns, ChatTurn{User: prompt, State: ChatRunning})
	go s.generate(ctx, run, stream, key, messages, index, notify)
	return run, nil
}

func (s *ChatSession) generate(ctx context.Context, run *ChatRun, stream StreamFunc, key string, messages []Message, index int, notify func()) {
	defer close(run.done)
	defer run.cancel()
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
	run.state = turn.State
	run.cancel = nil
	s.active = nil
	s.mu.Unlock()
	if notify != nil {
		notify()
	}
}

// Cancel requests stop; the session remains active until the worker exits.
func (s *ChatSession) Cancel() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancelRunLocked(s.active)
}

func (s *ChatSession) cancelRunLocked(run *ChatRun) bool {
	if run == nil || s.active != run || run.state == ChatStopping {
		return false
	}
	run.state = ChatStopping
	s.turns[len(s.turns)-1].State = ChatStopping
	run.cancel()
	return true
}

func (s *ChatSession) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != nil {
		return errors.New("cannot clear an active chat")
	}
	s.turns = nil
	return nil
}
