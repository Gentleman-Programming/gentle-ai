package engram

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// TestRunProtocolProbeCommandRealProcessDeadline exercises the REAL,
// unfaked runProtocolProbeCommand seam body (no canned-output fake) against
// a real, short-lived-but-long-running process substituted via a command
// override, standing in for a menu-printing old engram binary that would
// otherwise hang the setup loop (see design.md Decision 4 Open Questions).
//
// This pins three guarantees for JD-012: the call returns promptly after
// the context deadline fires, no data race is reported under `-race`, and
// no goroutine is leaked. Run with `go test -race -count=5` to make both
// the race and the leak reliably observable across repeated runs.
func TestRunProtocolProbeCommandRealProcessDeadline(t *testing.T) {
	// Old (pre-fix) implementation reads cmd.Process via execCommand only;
	// new (post-fix) implementation spawns via execCommandContext. Override
	// both so this single test is valid RED against the hand-rolled
	// goroutine/select implementation and GREEN against the
	// exec.CommandContext implementation, without needing to change the
	// test between the two phases.
	origExecCommand := execCommand
	execCommand = func(string, ...string) *exec.Cmd {
		return exec.Command("sleep", "10")
	}
	t.Cleanup(func() { execCommand = origExecCommand })

	origExecCommandContext := execCommandContext
	execCommandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sleep", "10")
	}
	t.Cleanup(func() { execCommandContext = origExecCommandContext })

	before := runtime.NumGoroutine()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := runProtocolProbeCommand(ctx, "engram")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("runProtocolProbeCommand() error = nil, want a cancellation/kill error for a process outliving the deadline")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("runProtocolProbeCommand() took %v, want a prompt return shortly after the 100ms deadline", elapsed)
	}

	// Give the runtime a brief window to reap any goroutines spawned
	// internally by os/exec (e.g. the exec.CommandContext watcher) before
	// comparing counts, to avoid a flaky false-positive leak report.
	deadline := time.Now().Add(1 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > before {
		t.Fatalf("goroutine leak: before=%d after=%d", before, after)
	}
}
