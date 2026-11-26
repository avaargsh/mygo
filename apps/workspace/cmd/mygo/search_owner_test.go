package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type testSearchRunner struct {
	started chan struct{}
	closed  chan struct{}
}

func (r *testSearchRunner) Serve(ctx context.Context, _ func()) error {
	close(r.started)
	<-ctx.Done()
	return ctx.Err()
}
func (r *testSearchRunner) Close() {
	select {
	case <-r.closed:
	default:
		close(r.closed)
	}
}
func awaitSearch(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out")
	}
}
func TestSearchOwnerConnectionLifetime(t *testing.T) {
	var o searchOwner
	if r, s := o.snapshot(); r != nil || s != "Disconnected" {
		t.Fatalf("initial %v %s", r, s)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	runner := &testSearchRunner{make(chan struct{}), make(chan struct{})}
	if !o.start(func(ctx context.Context) (searchRunner, error) { close(entered); <-release; return runner, nil }, nil) {
		t.Fatal("start rejected")
	}
	awaitSearch(t, entered)
	if o.start(func(context.Context) (searchRunner, error) { panic("duplicate") }, nil) {
		t.Fatal("duplicate accepted")
	}
	o.stop()
	if r, s := o.snapshot(); r != nil || s != "Disconnecting" {
		t.Fatalf("stop %v %s", r, s)
	}
	close(release)
	o.wait()
	awaitSearch(t, runner.closed)
	if r, s := o.snapshot(); r != nil || s != "Disconnected" {
		t.Fatalf("late connection %v %s", r, s)
	}
}
func TestSearchOwnerReconnectAndFailure(t *testing.T) {
	var o searchOwner
	if !o.start(func(context.Context) (searchRunner, error) { return nil, errors.New("failure") }, nil) {
		t.Fatal("start rejected")
	}
	o.wait()
	if r, s := o.snapshot(); r != nil || s != "Connection failed" {
		t.Fatalf("failure %v %s", r, s)
	}
	runner := &testSearchRunner{make(chan struct{}), make(chan struct{})}
	if !o.start(func(context.Context) (searchRunner, error) { return runner, nil }, nil) {
		t.Fatal("reconnect rejected")
	}
	awaitSearch(t, runner.started)
	if r, s := o.snapshot(); r != runner || s != "Connected" {
		t.Fatalf("connected %v %s", r, s)
	}
	o.stop()
	o.wait()
	awaitSearch(t, runner.closed)
	if r, s := o.snapshot(); r != nil || s != "Disconnected" {
		t.Fatalf("disconnected %v %s", r, s)
	}
}
