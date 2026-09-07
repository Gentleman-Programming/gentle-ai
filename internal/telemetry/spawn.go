package telemetry

import (
	"context"
	"os"
	"os/exec"
)

// Spawner launches the detached sender for one event payload and returns
// immediately without waiting for it to finish. Production code defaults to
// DefaultSpawn (SpawnDetachedSend); a caller that needs to observe or
// suppress the real spawn injects its own function through Deps.Spawn, and a
// whole test binary that must never spawn a real process for any of its
// tests overrides the package-level DefaultSpawn once, in its TestMain (see
// internal/cli's and internal/app's TestMain for the recording/no-op fakes
// they install).
type Spawner func(ctx context.Context, payload []byte) error

// DefaultSpawn is the production spawner every Opportunistic call falls back
// to when Deps.Spawn is nil. It is a package-level var, not a constant,
// specifically so a package's TestMain can replace it for the lifetime of
// that test binary — the seam #2 of the telemetry review asked for, so that
// no test anywhere in the module can ever start a real child process or
// reach the network merely by exercising install/update/sync/review code
// that calls Opportunistic without injecting its own Spawn.
var DefaultSpawn Spawner = SpawnDetachedSend

// osExecutable resolves the running binary's path. It is a var so a test can
// point it at a small fixture executable that records its argv and stdin
// instead of the real gentle-ai binary.
var osExecutable = os.Executable

// SpawnDetachedSend starts `<self> telemetry send` as a background process,
// hands it the payload on stdin, and returns without waiting for it to
// finish. Both stdout and stderr are discarded (os.DevNull): the parent
// already printed the one-time notice during enrollment, and a detached
// child must never write to a terminal the triggering command no longer
// owns. It never blocks the caller and never surfaces the child's exit
// status: the triggering command's exit code is always its own, and the
// payload is carried over stdin (never a temp file, so there is never a
// path to clean up or accidentally delete).
func SpawnDetachedSend(ctx context.Context, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cmd, stdinRead, err := buildSendCommand(payload)
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		_ = stdinRead.Close()
		return err
	}
	// The child received its own duplicated copy of this descriptor at
	// Start(); the parent's handle is no longer needed and must be closed
	// so it is not leaked across the goroutine below, which outlives this call.
	_ = stdinRead.Close()
	// Deliberately not Wait()ed inline: the send happens in the background
	// and its outcome never affects the caller's exit code or output. The
	// process is still reaped (not left a zombie) by waiting on it from a
	// goroutine that outlives this call.
	go func() { _ = cmd.Wait() }()
	return nil
}

// buildSendCommand is split out from SpawnDetachedSend so a test can run it
// synchronously (cmd.Run()) against a fixture executable and assert on the
// argv and stdin it actually received, without exercising the "start and
// forget" behavior or any network access.
//
// The payload is written to an os.Pipe and closed for writing BEFORE
// Start(), and Stdin is set to the pipe's read end directly (an *os.File,
// not a Reader). This matters: had Stdin been a bytes.Reader, os/exec would
// copy it into the child through an internal goroutine that only runs to
// completion if something waits for it (normally cmd.Wait()) — but this
// sender deliberately never waits inline, and the whole point of a detached
// sender is that the parent process may exit almost immediately after
// Start() returns. That copy goroutine lives in the PARENT process, so a
// parent that exits before the goroutine finishes truncates or empties the
// child's stdin. Passing an *os.File instead makes exec dup the descriptor
// straight into the child with no parent-side copying at all: the bytes are
// already sitting in the kernel pipe buffer (comfortably larger than this
// contract's 4 KiB ceiling) by the time Start() is called, so the child can
// read them on its own schedule, entirely independent of the parent's
// lifetime.
func buildSendCommand(payload []byte) (cmd *exec.Cmd, stdinRead *os.File, err error) {
	self, err := osExecutable()
	if err != nil {
		return nil, nil, err
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	if _, err := w.Write(payload); err != nil {
		_ = r.Close()
		_ = w.Close()
		return nil, nil, err
	}
	if err := w.Close(); err != nil {
		_ = r.Close()
		return nil, nil, err
	}
	cmd = exec.Command(self, "telemetry", "send")
	cmd.Stdin = r
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd, r, nil
}
