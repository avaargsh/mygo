package core

import (
	"errors"
	"testing"
)

type fakeTerminal struct {
	done         chan struct{}
	closes, code int
	closeErr     error
}

func (f *fakeTerminal) Done() <-chan struct{} { return f.done }
func (f *fakeTerminal) ExitCode() int         { return f.code }
func (f *fakeTerminal) Close() error          { f.closes++; return f.closeErr }

func TestTerminalExplicitStartAndStop(t *testing.T) {
	var s TerminalSession
	f := &fakeTerminal{done: make(chan struct{}), code: 130}
	calls := 0
	start := func() (TerminalResource, error) { calls++; return f, nil }
	if s.State() != TerminalIdle || s.Resource() != nil {
		t.Fatal("not idle")
	}
	if err := s.Start(start); err != nil {
		t.Fatal(err)
	}
	if s.State() != TerminalRunning || s.Resource() != f {
		t.Fatal("resource missing")
	}
	if s.Start(start) == nil || calls != 1 {
		t.Fatal("duplicate start")
	}
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
	if s.State() != TerminalStopping || s.Resource() != nil {
		t.Fatal("closing resource exposed")
	}
	if err := s.Stop(); err != nil || f.closes != 1 {
		t.Fatal("duplicate close")
	}
	if s.Start(start) == nil || calls != 1 {
		t.Fatal("restart while stopping")
	}
	close(f.done)
	if code, ok := s.ExitCode(); !ok || code != 130 || s.State() != TerminalExited {
		t.Fatal("missing exit evidence")
	}
	next := &fakeTerminal{done: make(chan struct{})}
	if err := s.Start(func() (TerminalResource, error) { return next, nil }); err != nil {
		t.Fatal(err)
	}
	if s.Resource() != next || f.closes != 1 {
		t.Fatal("restart failed")
	}
}

func TestTerminalNaturalExitPreservesOutputUntilRelease(t *testing.T) {
	var s TerminalSession
	f := &fakeTerminal{done: make(chan struct{}), code: 7}
	if err := s.Start(func() (TerminalResource, error) { return f, nil }); err != nil {
		t.Fatal(err)
	}
	close(f.done)
	if s.State() != TerminalExited || s.Resource() != f {
		t.Fatal("lost exited resource")
	}
	if code, ok := s.ExitCode(); !ok || code != 7 {
		t.Fatal("wrong exit code")
	}
	if err := s.Stop(); err != nil || s.Resource() != nil || f.closes != 1 {
		t.Fatal("release failed")
	}
}

func TestTerminalFailedStartAndClose(t *testing.T) {
	var s TerminalSession
	if s.Start(func() (TerminalResource, error) { return nil, errors.New("unavailable") }) == nil || s.State() != TerminalIdle {
		t.Fatal("start failure hidden")
	}
	if s.Start(func() (TerminalResource, error) { return nil, nil }) == nil {
		t.Fatal("nil resource accepted")
	}
	f := &fakeTerminal{done: make(chan struct{}), closeErr: errors.New("close failed")}
	if err := s.Start(func() (TerminalResource, error) { return f, nil }); err != nil {
		t.Fatal(err)
	}
	if s.Stop() == nil || s.State() != TerminalStopping || s.Resource() != nil {
		t.Fatal("close failure hidden")
	}
	if s.Start(func() (TerminalResource, error) { return f, nil }) == nil {
		t.Fatal("restart before exit")
	}
	close(f.done)
	if s.State() != TerminalExited {
		t.Fatal("exit not observed")
	}
}
