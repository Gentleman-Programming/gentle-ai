//go:build linux && amd64

package shellinstaller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func userNativeInstall(ctx context.Context, self string, args []string, req UserInstallRequest, stdin io.Reader, stdout, stderr io.Writer) error {
	if err := nativeRuntimeError(ctx, nil); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 140*time.Second)
	defer cancel()
	workspace, err := os.MkdirTemp(filepath.Dir(req.Destination), ".gentle-native-")
	if err != nil {
		return err
	}
	stage := workspace + "-stage"
	var operation *userNativeSlice
	retain := func(cause error) error {
		unit := ""
		if operation != nil {
			unit = operation.name
		}
		failure := privateError("uncertain", fmt.Errorf("operation %q; preserve publication stage %q: %w", unit, stage, cause))
		failure.Workspace, failure.Destination = workspace, req.Destination
		return failure
	}
	if _, err := privateDirectory(workspace); err != nil {
		return retain(err)
	}
	operation, err = newUserNativeSlice(ctx)
	if err != nil {
		return retain(err)
	}
	plan, err := newUserNativeServicePlan(ctx, operation, nil)
	if err == nil {
		deadline, _ := ctx.Deadline()
		entry := []string{"internal-native-coordinate", workspace, stage, strconv.FormatInt(deadline.UnixNano(), 10)}
		err = userServiceWithPlan(ctx, self, append(entry, args...), stdin, stdout, stderr, plan)
	}
	var settlement error
	if _, statErr := os.Lstat(operation.group.Path); !os.IsNotExist(statErr) {
		if err == nil {
			settlement = operation.settle(ctx)
		} else {
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
			settlement = operation.stop(stopCtx)
			stopCancel()
		}
	}
	if err != nil || settlement != nil {
		return retain(errors.Join(err, settlement))
	}
	return nil
}
