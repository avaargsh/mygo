package xerj

import (
	"context"
	"errors"
	"fmt"
)

// BuildTarget identifies one reserved generation, not a source authorization.
// Trusted application code must supply an approved, isolated corpus to the
// builder and independently verify the resulting exact index.
type BuildTarget struct {
	Workspace  string
	Index      string
	Endpoint   string
	Generation uint64
}

// Rebuild coordinates one synchronous build and verification attempt. Call it
// off the UI thread. Both callbacks must honor ctx and only operate on target's
// dedicated corpus/index. It does not start a process or schedule retries.
// Cancellation observed before publication prevents READY; a cancellation
// racing publication may occur after the successful state transition.
func (m *Manager) Rebuild(ctx context.Context, workspace, index string, build, verify func(context.Context, BuildTarget) error) error {
	if ctx == nil || build == nil || verify == nil {
		return errors.New("context, builder and verifier required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ticket, err := m.Begin(workspace, index)
	if err != nil {
		return err
	}
	target := BuildTarget{Workspace: workspace, Index: index, Endpoint: m.endpoint, Generation: ticket.number}
	fail := func(cause error) error {
		return errors.Join(cause, m.Fail(ticket))
	}
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.manifestFault {
			return ErrManifestUnavailable
		}
		b := m.workspaces[workspace]
		if b.status.Generation != ticket.number || b.status.State != IndexBuilding {
			return ErrStaleGeneration
		}
		return nil
	}
	if err := check(); err != nil {
		return fail(err)
	}
	if err := build(ctx, target); err != nil {
		return fail(fmt.Errorf("index build: %w", err))
	}
	if err := check(); err != nil {
		return fail(err)
	}
	if err := verify(ctx, target); err != nil {
		return fail(fmt.Errorf("index verification: %w", err))
	}
	if err := check(); err != nil {
		return fail(err)
	}
	return m.Publish(ticket)
}
