//go:build linux && amd64

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

func userNativeDeadline(value string, now time.Time) (time.Time, error) {
	nanos, err := strconv.ParseInt(value, 10, 64)
	deadline := time.Unix(0, nanos)
	if err != nil || !deadline.After(now) || deadline.After(now.Add(140*time.Second)) {
		return time.Time{}, privateError("refused", errors.New("invalid native operation deadline"))
	}
	return deadline, nil
}

// Derive membership from the live kernel, never from a caller-supplied slice ID.
// This read-only handle cannot manage or settle the manager-owned operation.
func userNativeOperation(ctx context.Context) (*userNativeSlice, error) {
	if err := nativeRuntimeError(ctx, nil); err != nil {
		return nil, err
	}
	membership, err := userNativeProc("cgroup")
	if err != nil {
		return nil, err
	}
	mounts, err := userNativeProc("mountinfo")
	if err != nil {
		return nil, err
	}
	leaf, err := privateCgroupPath(membership, mounts)
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(leaf)
	if !userNativeSliceName(filepath.Base(parent)) {
		return nil, privateError("refused", nil)
	}
	if err := privateCgroupPhysical(parent, true); err != nil {
		return nil, err
	}
	info, err := os.Stat(parent)
	if err != nil {
		return nil, err
	}
	stat := info.Sys().(*syscall.Stat_t)
	operation := &userNativeSlice{name: filepath.Base(parent), group: userUnitEvidence{parent, stat.Dev, stat.Ino}}
	if err := operation.member(ctx); err != nil {
		return nil, err
	}
	return operation, nil
}

// Re-entry selectors only transport paths, the original deadline and consent.
// Execution still requires live isolation, actual aggregate limits, the fixed
// source snapshot and the ordinary physical selection confirmation.
func userNativeChild(ctx context.Context, args []string, stdout io.Writer) error {
	if err := nativeRuntimeError(ctx, nil); err != nil {
		return err
	}
	if len(args) < 8 {
		return errors.New("invalid native child arguments")
	}
	deadline, err := userNativeDeadline(args[2], time.Now())
	if err != nil {
		return err
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	req, err := UserInstallFromEntry(args[3:])
	if err != nil {
		return err
	}
	if err := userNativeExecutionCheck(args[0], "/run/user/"+strconv.Itoa(os.Getuid())); err != nil {
		return err
	}
	operation, err := userNativeOperation(ctx)
	if err != nil {
		return err
	}
	artifact, err := userNativeCarrierDescriptor()
	if err != nil {
		return err
	}
	worker, err := userNativeRuntimeFromSnapshot(ctx, args[0], artifact)
	if err != nil {
		return err
	}
	if _, err := privateNativeFile(ctx, worker.node, 0700, privateNativeNodeSize, privateNativeNodeSHA); err != nil {
		return err
	}
	native, location, err := worker.produceForDestination(ctx, req, args[1])
	if err != nil {
		return err
	}
	if location == nil {
		return userNativePublicationFailure(native)
	}
	if err := operation.member(ctx); err != nil {
		return err
	}
	return userWriteNativeReceipt(worker.root, native, location)
}

func userNativePublicationFailure(result NativeInstallerResult) error {
	message := fmt.Sprintf("original installer did not publish: %s (%s)", result.Outcome, result.Reason)
	if len(result.Details) != 0 {
		var detail struct {
			FailedStep string `json:"failedStep"`
			Detail     string `json:"detail"`
		}
		if len(result.Details) > 16384 || json.Unmarshal(result.Details, &detail) != nil {
			message += "; original diagnostics unavailable"
		} else if detail.FailedStep != "" || detail.Detail != "" {
			message += fmt.Sprintf("; step=%q; detail=%q; completed=%d", detail.FailedStep, detail.Detail, len(result.Completed))
		}
	}
	return privateError("refused", errors.New(message))
}
