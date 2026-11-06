//go:build linux && amd64

package shellinstaller

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"
)

// A plan changes supervision only for native installation. Generic services
// retain their existing arguments, cancellation and physical receipt handling.
func newUserNativeServicePlan(ctx context.Context, operation *userNativeSlice, custody *userNativeServiceCustody) (*userServicePlan, error) {
	if err := nativeRuntimeError(ctx, nil); err != nil {
		return nil, err
	}
	if operation == nil {
		return nil, privateError("refused", nil)
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, privateError("refused", nil)
	}
	if _, err := userNativeDeadline(strconv.FormatInt(deadline.UnixNano(), 10), time.Now()); err != nil {
		return nil, err
	}
	plan := &userServicePlan{}
	plan.arguments = func(unit string, interactive bool, self, cwd string, args, env []string) ([]string, error) {
		slice, err := operation.serviceArg(ctx)
		if err != nil {
			return nil, err
		}
		// env -i clears inherited controls before the Go service re-entry.
		// Fix native execution parallelism without changing generic services.
		controlled := make([]string, 0, len(env)+1)
		for _, value := range env {
			if !strings.HasPrefix(value, "GOMAXPROCS=") {
				controlled = append(controlled, value)
			}
		}
		env = append(controlled, "GOMAXPROCS=1")
		if custody != nil {
			sealed, err := custody.args(ctx, unit, interactive, self, cwd, args, env)
			if err != nil {
				return nil, err
			}
			return append([]string{slice}, sealed...), nil
		}
		// The coordinator keeps its full identity map and manager access.
		properties := []string{slice, "--property=CapabilityBoundingSet=", "--property=RuntimeMaxSec=300s"}
		return append(properties, userServiceArgs(unit, interactive, self, cwd, args, env)...), nil
	}
	if len(operation.env) != 0 {
		plan.cancel = func() error {
			// Cancellation must outlive the canceled operation context, but
			// keeps the existing service cancellation bound.
			stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return operation.stop(stopCtx)
		}
	}
	return plan, nil
}

// Only the creator-held manager identity can cancel a populated operation.
// Read-only membership handles cannot stop siblings; settle remains empty-only.
func (s *userNativeSlice) stop(ctx context.Context) error {
	if s == nil || len(s.env) == 0 || len(s.tools) == 0 {
		return privateError("refused", nil)
	}
	if err := s.read(); err != nil {
		return s.uncertain(err)
	}
	if _, err := s.run(ctx, "/usr/bin/systemctl", "--user", "stop", s.name); err != nil {
		return s.uncertain(err)
	}
	if _, err := os.Lstat(s.group.Path); !os.IsNotExist(err) {
		return s.uncertain(err)
	}
	return nil
}
