//go:build !linux

package system

import (
	"context"
	"time"

	"github.com/egoist/mygo/apps/workspace/core"
)

type unsupportedProvider struct{}

func New() Provider { return unsupportedProvider{} }

func (unsupportedProvider) Snapshot(ctx context.Context) (core.SystemSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return core.SystemSnapshot{}, err
	}
	return core.SystemSnapshot{
		CapturedAt:  time.Now(),
		Unavailable: []string{"cpu", "memory", "disk", "network"},
	}, nil
}
