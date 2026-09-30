package opencode

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
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

// apiColdStartRetryDelay is the bounded wait before the single empty-catalog
// retry. A freshly activated V2 service exits 0 with an empty data array
// until its background boot completes (observed on V2.0.16 cold start), so
// one short, bounded retry covers that window without looping.
const apiColdStartRetryDelay = 1500 * time.Millisecond

// apiModelRoute is the V2 local API route returning the location-scoped
// model catalog. The brackets are pre-escaped (%5B/%5D) as the V2 CLI
// expects; the project directory is appended URL-escaped by the caller.
const apiModelRoute = "/api/model?location%5Bdirectory%5D="

// discoverAPICatalog runs the V2 api discovery attempt through the injected
// runner and parses the JSON envelope. On a clean exit with zero providers it
// retries exactly once after apiColdStartRetryDelay, honoring ctx; a failed
// retry never discards the first attempt's (empty) result, because that
// result was already legitimate. Classification mirrors the V1 pipeline: the
// child's exit status wins over parse classification, while a genuine
// overflow keeps its own category.
func discoverAPICatalog(ctx context.Context, projectDir string, runner CommandRunner) (map[string]Provider, error) {
	run := func() (map[string]Provider, error) {
		r, err := runner(ctx, Command{
			Path: "opencode",
			Args: []string{"api", "get", apiModelRoute + url.QueryEscape(projectDir)},
			Dir:  projectDir,
		})
		if err != nil {
			return nil, catalogCommandError(ctx, err)
		}
		if closer, ok := r.(io.Closer); ok {
			defer closer.Close()
		}
		providers, parseErr := parseV2ModelAPI(r)
		if parseErr != nil {
			var catalogErr *CatalogError
			if errors.As(parseErr, &catalogErr) && catalogErr.Kind == CatalogErrorOutputTooLarge {
				return nil, catalogErr
			}
			return nil, catalogCommandErrorWithRunnerWait(ctx, r, parseErr)
		}
		if waiter, ok := r.(waitErrorReader); ok {
			if waitErr := waiter.WaitError(); waitErr != nil {
				return nil, catalogCommandError(ctx, waitErr)
			}
		}
		return providers, nil
	}

	providers, err := run()
	if err != nil || len(providers) > 0 {
		return providers, err
	}
	select {
	case <-ctx.Done():
		// Out of budget: accept the empty catalog instead of racing the
		// deadline with a retry that cannot finish.
		return providers, nil
	case <-time.After(apiColdStartRetryDelay):
	}
	retried, err := run()
	if err != nil {
		return providers, nil
	}
	return retried, nil
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
