package xerj

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// BuildTextSet builds a fresh, independently specified set of bounded text
// datasets, validates the catalog and every dataset, and publishes together.
// The caller must exclusively own the node, immutable staged corpus and
// expected digests. Do not mix this with direct engine mutations or bypass
// publication through other Manager APIs. Run off the UI thread with a deadline.
func (m *Manager) BuildTextSet(ctx context.Context, workspace string, config AutoindexConfig, expected map[string]TextIndexVerifier) error {
	return m.buildTextSet(ctx, workspace, config.Prefix, expected, config.Run)
}

// Caller holds m.mu. Reservations survive restarts in persistent managers.
func (m *Manager) checkPrefixLocked(prefix string) error {
	if m.manifestFault {
		return ErrManifestUnavailable
	}
	for name := range m.reserved {
		if strings.HasPrefix(name, prefix+"-") {
			return errors.New("prefix already reserved")
		}
	}
	return nil
}

func (m *Manager) buildTextSet(ctx context.Context, workspace, prefix string, expected map[string]TextIndexVerifier, build func(context.Context, BuildTarget) error) error {
	if ctx == nil || build == nil || !autoindexPrefix.MatchString(prefix) || len(expected) == 0 || len(expected) > 8 || strings.TrimSpace(workspace) == "" || len(workspace) > 256 {
		return errors.New("invalid pipeline configuration")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	names := make([]string, 0, len(expected))
	for name, v := range expected {
		if !indexName.MatchString(name) || !strings.HasPrefix(name, prefix+"-") || name == prefix+"-" || len(v.Expected) == 0 || len(v.Expected) > 32 {
			return errors.New("invalid expected dataset")
		}
		names = append(names, name)
	}
	sort.Strings(names)
	m.mu.Lock()
	err := m.checkPrefixLocked(prefix)
	m.mu.Unlock()
	if err != nil {
		return err
	}
	found, err := DiscoverIndices(ctx, m.endpoint, prefix)
	if err != nil {
		return fmt.Errorf("catalog preflight: %w", err)
	}
	if len(found) != 0 {
		return errors.New("prefix already exists on node")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ticket, err := m.beginSet(workspace, names, prefix)
	if err != nil {
		return err
	}
	fail := func(err error) error { return errors.Join(err, m.Fail(ticket)) }
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
	target := BuildTarget{Workspace: workspace, Index: names[0], Endpoint: m.endpoint, Generation: ticket.number}
	if err := check(); err != nil {
		return fail(err)
	}
	if err := build(ctx, target); err != nil {
		return fail(fmt.Errorf("dataset build: %w", err))
	}
	verifyCatalog := func() error {
		if err := check(); err != nil {
			return err
		}
		namesFound, err := DiscoverIndices(ctx, m.endpoint, prefix)
		if err != nil {
			return err
		}
		return MatchIndexSet(namesFound, names)
	}
	if err := verifyCatalog(); err != nil {
		return fail(fmt.Errorf("post-build catalog: %w", err))
	}
	for _, name := range names {
		if err := check(); err != nil {
			return fail(err)
		}
		target.Index = name
		if err := expected[name].Verify(ctx, target); err != nil {
			return fail(fmt.Errorf("dataset verification: %w", err))
		}
	}
	if err := verifyCatalog(); err != nil {
		return fail(fmt.Errorf("final catalog: %w", err))
	}
	if err := check(); err != nil {
		return fail(err)
	}
	return m.Publish(ticket)
}
