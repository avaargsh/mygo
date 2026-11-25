package system

import (
	"context"

	"github.com/egoist/mygo/apps/workspace/core"
)

// Provider returns only metrics the platform can actually measure.
type Provider interface {
	Snapshot(context.Context) (core.SystemSnapshot, error)
}
