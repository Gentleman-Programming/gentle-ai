package opencode

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"sync/atomic"
	"time"
)

// apiOutputPollInterval is how often the runAPICommand monitor checks the
// temp file size against maxCatalogOutput. 50ms bounds a runaway child's
// overshoot to a few MB at typical write speeds.
const apiOutputPollInterval = 50 * time.Millisecond

// runAPICommand is the file-backed runner for the V2 api discovery command.
// The V2 CLI aborts its stdout write nondeterministically when stdout is a
// pipe — observed on V2.0.16 with clean EOF, exit 0, and truncations between
// 221184 and 262144 bytes even under a fast concurrent reader, while output
// to a regular file always completes (318754 bytes) — so the child writes to
// a temp file through a direct file descriptor and the complete content is
// read back after a successful wait. The V1 pipe runner keeps its own
// drain-based abort mitigation; only the V2 route needs the file transport.
//
// Process handling mirrors runCatalogCommand (process-group setup, WaitDelay,
// context cancellation killing the child). A monitor goroutine enforces the
// maxCatalogOutput ceiling DURING execution by polling the file size and
// cancelling the command context on first breach — Stdout must stay the raw
// *os.File (any io.Writer wrapper would make exec create a pipe and
// reintroduce the V2 truncation), so the cap cannot be enforced by the
// writer. A killed runaway surfaces as output_too_large; the post-read
// bounded check remains as a backstop for a child that exits between polls.
// Failures are otherwise classified with the shared catalogCommandError
// taxonomy — a wait/exit error wins over anything the file contains. The
// temp file is always removed (best-effort close before remove).
func runAPICommand(ctx context.Context, command Command) (io.Reader, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	tmp, err := os.CreateTemp("", "gentle-ai-catalog-*")
	if err != nil {
		return nil, err
	}
	// Deferred calls run LIFO: the file is closed before it is removed, and
	// both happen on every path below. Best-effort cleanup — failures are
	// explicitly discarded.
	defer func() { _ = os.Remove(tmp.Name()) }()
	defer func() { _ = tmp.Close() }()

	cmd := exec.CommandContext(ctx, command.Path, command.Args...)
	cmd.Dir = command.Dir
	cmd.Stdout = tmp        // direct fd to a regular file, never a pipe
	cmd.Stderr = io.Discard // catalog errors never expose command output
	cmd.WaitDelay = catalogWaitDelay
	afterStart, release := configureProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		if release != nil {
			release()
		}
		return nil, catalogCommandError(ctx, err)
	}
	if afterStart != nil {
		afterStart()
	}

	// Overflow is flagged BEFORE cancelling so the post-Wait classification
	// can attribute the kill to the cap instead of the caller's deadline.
	var overflowObserved atomic.Bool
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		ticker := time.NewTicker(apiOutputPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if info, statErr := tmp.Stat(); statErr == nil && info.Size() > maxCatalogOutput {
					overflowObserved.Store(true)
					cancel()
					return
				}
			}
		}
	}()

	waitErr := cmd.Wait()
	// Classify the exit BEFORE stopping the monitor: the internal cancel
	// below must not masquerade as the caller's timeout.
	var classifiedErr error
	if waitErr != nil {
		classifiedErr = catalogCommandError(ctx, waitErr)
	}
	// The child is reaped; stop the size monitor. Without this a normal
	// exit would leave the monitor ticking on ctx.Done forever and the
	// join below would block.
	cancel()
	<-monitorDone
	if overflowObserved.Load() {
		return nil, &CatalogError{Kind: CatalogErrorOutputTooLarge}
	}
	if classifiedErr != nil {
		return nil, classifiedErr
	}

	// The child wrote through a dup of this descriptor: POSIX file
	// descriptions share their offset, so it sits at the end of the child's
	// output. Rewind before reading.
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	content, err := io.ReadAll(io.LimitReader(tmp, maxCatalogOutput+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maxCatalogOutput {
		return nil, &CatalogError{Kind: CatalogErrorOutputTooLarge}
	}
	return bytes.NewReader(content), nil
}

// discoverCatalogCommandRunner routes live discovery attempts: the V2 api
// invocation goes through the file-backed runner (piped stdout truncates),
// while the version probe and the V1 stream keep the unchanged pipe-based
// runner with its background-drain mitigation.
func discoverCatalogCommandRunner(ctx context.Context, command Command) (io.Reader, error) {
	if len(command.Args) > 0 && command.Args[0] == "api" {
		return runAPICommand(ctx, command)
	}
	return runCatalogCommand(ctx, command)
}
