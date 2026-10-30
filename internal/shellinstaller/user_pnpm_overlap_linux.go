//go:build linux && amd64

package shellinstaller

import (
	"context"
	"errors"
)

// Both traversals inspect the already-closed private stage. They keep separate
// readers and buffers and must terminate before comparison, cleanup or publish.
func userPNPMInventoryWhileSyncing(ctx context.Context, inventory func(context.Context) (map[string]string, error), synchronize func(context.Context) error) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		entries map[string]string
		err     error
	}
	completed := make(chan result, 1)
	go func() {
		entries, err := inventory(ctx)
		if err != nil {
			cancel()
		}
		completed <- result{entries, err}
	}()
	syncErr := synchronize(ctx)
	if syncErr != nil {
		cancel()
	}
	observed := <-completed
	if err := errors.Join(syncErr, observed.err); err != nil {
		return nil, err
	}
	return observed.entries, nil
}
