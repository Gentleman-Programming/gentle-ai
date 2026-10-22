//go:build linux && amd64

package shellinstaller

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// RunUserEntryReplacingOwner is exclusively for a process-owning CLI after its
// terminal has been restored. A fresh Separate install replaces this process:
// deferred calls and arbitrary caller cancellation callbacks do not survive.
// Programmatic callers must use RunUserEntry. Custom streams retain that path.
func RunUserEntryReplacingOwner(ctx context.Context, self string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return userReplaceOwner(ctx, self, args, stdin, stdout, stderr, syscall.Exec)
}

func userReplaceOwner(ctx context.Context, self string, args []string, stdin io.Reader, stdout, stderr io.Writer, replace func(string, []string, []string) error) error {
	if len(args) == 0 || args[0] != "install" || stdin != os.Stdin || stdout != os.Stdout || stderr != os.Stderr {
		return RunUserEntry(ctx, self, args, stdin, stdout, stderr)
	}
	if err := nativeRuntimeError(ctx, nil); err != nil {
		return err
	}
	_, fresh, err := userNativeFreshRequest(args[1:])
	if err != nil {
		return err
	}
	if !fresh {
		return RunUserEntry(ctx, self, args, stdin, stdout, stderr)
	}
	actual, err := os.Executable()
	if err != nil {
		return err
	}
	if self != actual {
		return privateError("refused", errors.New("native owner requires the current executable"))
	}
	deadline := time.Now().Add(140 * time.Second)
	if parent, ok := ctx.Deadline(); ok && parent.Before(deadline) {
		deadline = parent
	}
	argv := append([]string{actual, "shell", "internal-native-own", strconv.FormatInt(deadline.UnixNano(), 10)}, args[1:]...)
	if err := nativeRuntimeError(ctx, nil); err != nil {
		return err
	}
	// Exec the loaded inode, not a pathname that could select another binary.
	// The same PID and descriptors survive, but the former Go runtime does not.
	return replace("/proc/self/exe", argv, userNativeOwnerEnvironment(os.Environ()))
}

func userNativeOwnerEnvironment(env []string) []string {
	controlled := make([]string, 0, len(env)+1)
	for _, value := range env {
		if !strings.HasPrefix(value, "GOMAXPROCS=") {
			controlled = append(controlled, value)
		}
	}
	return append(controlled, "GOMAXPROCS=1")
}

// This selector transports a bounded deadline, not execution authority. It
// repeats fresh physical consent checks before creating any operation state.
func userNativeOwnerEntry(ctx context.Context, self string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if err := nativeRuntimeError(ctx, nil); err != nil {
		return err
	}
	if len(args) < 6 {
		return privateError("refused", errors.New("invalid native owner arguments"))
	}
	deadline, err := userNativeDeadline(args[0], time.Now())
	if err != nil {
		return err
	}
	req, fresh, err := userNativeFreshRequest(args[1:])
	if err != nil {
		return err
	}
	if !fresh {
		return privateError("refused", errors.New("native owner requires a fresh Separate selection"))
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	return userNativeInstall(ctx, self, args[1:], req, stdin, stdout, stderr)
}
