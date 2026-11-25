package core

import "errors"

// TerminalResource is the lifecycle contract of a local terminal. Done is
// closed only after process exit. Close requests teardown, not proof of exit.
type TerminalResource interface {
	Done() <-chan struct{}
	ExitCode() int
	Close() error
}

type TerminalState string

const (
	TerminalIdle     TerminalState = "idle"
	TerminalRunning  TerminalState = "running"
	TerminalStopping TerminalState = "stopping"
	TerminalExited   TerminalState = "exited"
)

// TerminalSession is owned by the UI thread; a tab never owns the process.
type TerminalSession struct {
	resource TerminalResource
	closing  bool
}

func (s *TerminalSession) State() TerminalState {
	if s.resource == nil {
		return TerminalIdle
	}
	select {
	case <-s.resource.Done():
		return TerminalExited
	default:
	}
	if s.closing {
		return TerminalStopping
	}
	return TerminalRunning
}

func (s *TerminalSession) Resource() TerminalResource {
	if s.closing {
		return nil
	}
	return s.resource
}

func (s *TerminalSession) ExitCode() (int, bool) {
	if s.State() != TerminalExited {
		return 0, false
	}
	return s.resource.ExitCode(), true
}

func (s *TerminalSession) Start(factory func() (TerminalResource, error)) error {
	switch s.State() {
	case TerminalRunning, TerminalStopping:
		return errors.New("terminal already active")
	case TerminalExited:
		if err := s.Stop(); err != nil {
			return err
		}
	}
	if factory == nil {
		return errors.New("terminal factory required")
	}
	r, err := factory()
	if err != nil {
		return err
	}
	if r == nil {
		return errors.New("terminal resource missing")
	}
	s.resource = r
	s.closing = false
	return nil
}

func (s *TerminalSession) Stop() error {
	if s.resource == nil || s.closing {
		return nil
	}
	s.closing = true
	return s.resource.Close()
}
