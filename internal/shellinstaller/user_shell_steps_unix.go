//go:build linux || darwin

package shellinstaller

import (
	"context"
	"fmt"
	"os"
)

type userInstallProgressKey struct{}

func userProgressEnvironment(ctx context.Context) []string {
	if os.Getenv("GENTLE_AI_SHELL_STEPS") == "1" {
		return []string{"GENTLE_AI_SHELL_STEPS=1"}
	}
	if ctx == nil {
		return nil
	}
	if _, ok := ctx.Value(userInstallProgressKey{}).(func(UserInstallProgress)); ok {
		// The owned service is a separate process: its stdout is piped back and
		// parsed by the in-process owner.
		return []string{"GENTLE_AI_SHELL_STEPS=1"}
	}
	return nil
}

func userInstallReport(ctx context.Context, progress UserInstallProgress) {
	if ctx == nil {
		return
	}
	if report, ok := ctx.Value(userInstallProgressKey{}).(func(UserInstallProgress)); ok {
		report(progress)
	}
}

// userShellStep prints one completed plan step only when the TUI requested the
// plan output. It never changes installation authority or ordering.
func userShellStepBegin(ctx context.Context, index, total int, label string) {
	userInstallReport(ctx, UserInstallProgress{Index: index, Total: total, Label: label, State: "running"})
}

func userShellDetail(ctx context.Context, index, total int, label string) {
	userInstallReport(ctx, UserInstallProgress{Index: index, Total: total, Label: label, State: "detail", Detail: label})
	if os.Getenv("GENTLE_AI_SHELL_STEPS") != "1" {
		return
	}
	fmt.Fprintf(os.Stdout, "~ %d/%d %s\n", index, total, label)
}

func userShellStep(ctx context.Context, index, total int, label string) {
	userInstallReport(ctx, UserInstallProgress{Index: index, Total: total, Label: label, State: "done"})
	if os.Getenv("GENTLE_AI_SHELL_STEPS") != "1" {
		return
	}
	fmt.Fprintf(os.Stdout, "✓ %d/%d %s\n", index, total, label)
}
