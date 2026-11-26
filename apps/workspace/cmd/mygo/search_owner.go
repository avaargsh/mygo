package main

import (
	"context"
	"sync"
)

// searchRunner owns the worker associated with a single search connection.
// The owner does not start another connection until the old worker exits.
type searchRunner interface {
	Serve(context.Context, func()) error
	Close()
}

type searchRun struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type searchOwner struct {
	mu     sync.Mutex
	run    *searchRun
	runner searchRunner
	state  string
}

func (o *searchOwner) snapshot() (searchRunner, string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.state == "" {
		return nil, "Disconnected"
	}
	return o.runner, o.state
}

func (o *searchOwner) start(connect func(context.Context) (searchRunner, error), notify func()) bool {
	o.mu.Lock()
	if o.run != nil || connect == nil {
		o.mu.Unlock()
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &searchRun{cancel: cancel, done: make(chan struct{})}
	o.run, o.state = r, "Connecting"
	o.mu.Unlock()
	go func() {
		defer func() {
			o.mu.Lock()
			if o.runner != nil {
				o.runner.Close()
				o.runner = nil
			}
			if o.state != "Connection failed" {
				o.state = "Disconnected"
			}
			o.run = nil
			close(r.done)
			o.mu.Unlock()
			cancel()
			if notify != nil {
				notify()
			}
		}()
		runner, err := connect(ctx)
		o.mu.Lock()
		if ctx.Err() != nil || err != nil || runner == nil {
			if runner != nil {
				runner.Close()
			}
			if ctx.Err() == nil {
				o.state = "Connection failed"
			}
			o.mu.Unlock()
			return
		}
		o.runner, o.state = runner, "Connected"
		o.mu.Unlock()
		if notify != nil {
			notify()
		}
		_ = runner.Serve(ctx, notify)
	}()
	return true
}

// stop clears the connection synchronously and requests cancellation. It
// never waits on the UI thread. wait is used only after App.Run returns.
func (o *searchOwner) stop() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.run == nil {
		o.state = "Disconnected"
		return
	}
	o.state = "Disconnecting"
	o.run.cancel()
	if o.runner != nil {
		o.runner.Close()
		o.runner = nil
	}
}

func (o *searchOwner) wait() {
	o.mu.Lock()
	r := o.run
	o.mu.Unlock()
	if r != nil {
		<-r.done
	}
}
