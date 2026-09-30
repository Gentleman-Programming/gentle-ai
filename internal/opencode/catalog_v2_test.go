package opencode

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeExitReader mimics a process stream reader whose stdout content and
// child wait status can be set independently, so discovery flows can be
// exercised without spawning real processes.
type fakeExitReader struct {
	r       io.Reader
	waitErr error
}

func (f *fakeExitReader) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *fakeExitReader) Close() error               { return nil }
func (f *fakeExitReader) WaitError() error           { return f.waitErr }

// generateAPIEnvelope builds a valid V2 catalog envelope of roughly
// ~530 bytes per entry across two providers.
func generateAPIEnvelope(entries int) string {
	var b strings.Builder
	b.WriteString(`{"location":{"directory":"/proj"},"data":[`)
	for i := 0; i < entries; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":"m-%04d","modelID":"m-%04d","providerID":"p-%d","name":"Model %04d %s","capabilities":{"tools":true},"variants":[],"cost":[{"input":0,"output":0}],"enabled":true,"limit":{"context":8,"output":4}}`,
			i, i, i%2, i, strings.Repeat("x", 240))
	}
	b.WriteString(`]}`)
	return b.String()
}

func TestRunAPICommandReturnsCompleteLargeOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell script fixture")
	}
	dir := t.TempDir()
	// ~480KB: comfortably past the 221184-byte point where the V2 CLI
	// truncated its piped stdout on the live host.
	envelope := generateAPIEnvelope(900)
	if len(envelope) <= 221184 {
		t.Fatalf("fixture envelope = %d bytes, must exceed the observed 221184-byte pipe truncation", len(envelope))
	}
	payload := filepath.Join(dir, "payload.json")
	if err := os.WriteFile(payload, []byte(envelope), 0o644); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	// Confine os.CreateTemp to the test dir so cleanup assertions are
	// immune to unrelated files in the shared system temp directory.
	t.Setenv("TMPDIR", dir)
	script := filepath.Join(dir, "cat-fixture")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ncat \"$1\"\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	r, err := runAPICommand(context.Background(), Command{Path: script, Args: []string{payload}})
	if err != nil {
		t.Fatalf("runAPICommand() error = %v", err)
	}
	content, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if len(content) != len(envelope) {
		t.Fatalf("output = %d bytes, want the complete %d bytes the child wrote (no transport truncation)", len(content), len(envelope))
	}
	providers, parseErr := parseV2ModelAPI(bytes.NewReader(content))
	if parseErr != nil {
		t.Fatalf("parseV2ModelAPI() error = %v, want the full payload to parse", parseErr)
	}
	if len(providers) != 2 || len(providers["p-0"].Models) != 450 || len(providers["p-1"].Models) != 450 {
		t.Fatalf("parsed providers = %d (p-0: %d, p-1: %d), want 2 providers with 450 models each", len(providers), len(providers["p-0"].Models), len(providers["p-1"].Models))
	}
	if left, globErr := filepath.Glob(filepath.Join(dir, "gentle-ai-catalog-*")); globErr != nil || len(left) != 0 {
		t.Fatalf("temp catalog files left behind: %v (glob err %v)", left, globErr)
	}
}

func TestRunAPICommandClassifiesWaitAndStartFailures(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell script fixture")
	}
	dir := t.TempDir()
	// Confine os.CreateTemp to the test dir so cleanup assertions are
	// immune to unrelated files in the shared system temp directory.
	t.Setenv("TMPDIR", dir)

	t.Run("non-zero exit wins over output", func(t *testing.T) {
		script := filepath.Join(dir, "fail-exit")
		if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '{\"location\":{},\"data\":['\nexit 1\n"), 0o755); err != nil {
			t.Fatalf("write script: %v", err)
		}
		r, err := runAPICommand(context.Background(), Command{Path: script})
		if r != nil {
			t.Fatalf("reader = %v, want nil on command failure", r)
		}
		var catalogErr *CatalogError
		if !errors.As(err, &catalogErr) || catalogErr.Kind != CatalogErrorCommandFailed {
			t.Fatalf("error = %v, want command_failed (exit status wins over partial output)", err)
		}
		if left, globErr := filepath.Glob(filepath.Join(dir, "gentle-ai-catalog-*")); globErr != nil || len(left) != 0 {
			t.Fatalf("temp catalog files left behind after failure: %v (glob err %v)", left, globErr)
		}
	})

	t.Run("missing binary", func(t *testing.T) {
		// A bare name absent from PATH triggers exec's LookPath failure,
		// which is what missing_binary classification covers.
		_, err := runAPICommand(context.Background(), Command{Path: "opencode-missing-from-path-3f91b"})
		var catalogErr *CatalogError
		if !errors.As(err, &catalogErr) || catalogErr.Kind != CatalogErrorMissingBinary {
			t.Fatalf("error = %v, want missing_binary", err)
		}
	})

	t.Run("output above the ceiling", func(t *testing.T) {
		// Practical with a script: dd streams 17MB of zeros in milliseconds,
		// tripping the shared maxCatalogOutput ceiling.
		script := filepath.Join(dir, "overflow")
		if err := os.WriteFile(script, []byte("#!/bin/sh\ndd if=/dev/zero bs=1048576 count=17 2>/dev/null\n"), 0o755); err != nil {
			t.Fatalf("write script: %v", err)
		}
		_, err := runAPICommand(context.Background(), Command{Path: script})
		var catalogErr *CatalogError
		if !errors.As(err, &catalogErr) || catalogErr.Kind != CatalogErrorOutputTooLarge {
			t.Fatalf("error = %v, want output_too_large", err)
		}
	})
}

// TestRunAPICommandCapsRunawayChildDuringExecution pins the execution-time
// output cap: a child that keeps writing far beyond maxCatalogOutput must be
// cancelled while still running and surface output_too_large promptly, not
// after the (never-arriving) exit. The script is bounded at 200MB so a
// leaked child can never fill the disk while the cap is being developed.
func TestRunAPICommandCapsRunawayChildDuringExecution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell script fixture")
	}
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	script := filepath.Join(dir, "runaway")
	body := "#!/bin/sh\nfor i in $(seq 1 200); do dd if=/dev/zero bs=1048576 count=1 2>/dev/null; sleep 0.05; done\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	type apiResult struct {
		r   io.Reader
		err error
	}
	done := make(chan apiResult, 1)
	started := time.Now()
	go func() {
		r, err := runAPICommand(context.Background(), Command{Path: script})
		done <- apiResult{r: r, err: err}
	}()

	select {
	case res := <-done:
		var catalogErr *CatalogError
		if !errors.As(res.err, &catalogErr) || catalogErr.Kind != CatalogErrorOutputTooLarge {
			t.Fatalf("error = %v, want output_too_large observed during execution", res.err)
		}
		if elapsed := time.Since(started); elapsed > 3*time.Second {
			t.Fatalf("cap observed after %v, want prompt cancellation well before the child finishes", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runAPICommand did not bound the runaway child; it blocked past 5s waiting for an exit that never comes")
	}
	if left, globErr := filepath.Glob(filepath.Join(dir, "gentle-ai-catalog-*")); globErr != nil || len(left) != 0 {
		t.Fatalf("temp catalog files left behind: %v (glob err %v)", left, globErr)
	}
}

// discoverV2CatalogWithRunner fakes the version probe as V2 so discovery
// tests can focus on the api attempt behavior.
func discoverV2CatalogWithRunner(ctx context.Context, dir string, runner CommandRunner) (map[string]Provider, error) {
	return DiscoverCatalogWithRunner(ctx, dir, func(ctx context.Context, command Command) (io.Reader, error) {
		if strings.Join(command.Args, " ") == "--version" {
			return strings.NewReader("opencode v2.0.18\n"), nil
		}
		return runner(ctx, command)
	})
}

// TestDiscoverCatalogV2EncodesSpecialCharsInProjectDir pins the query
// encoding contract on the discovery route: projectDir is percent-escaped,
// so characters like &, #, ? and spaces cannot truncate or corrupt the
// location[directory] parameter.
func TestDiscoverCatalogV2EncodesSpecialCharsInProjectDir(t *testing.T) {
	projectDir := `/tmp/x/y & z?q=1#frag`
	var got Command
	_, err := discoverV2CatalogWithRunner(context.Background(), projectDir, func(_ context.Context, command Command) (io.Reader, error) {
		got = command
		return strings.NewReader(`{"data":[]}`), nil
	})
	if err != nil {
		t.Fatalf("DiscoverCatalogWithRunner() error = %v", err)
	}
	if len(got.Args) != 3 {
		t.Fatalf("args = %v, want [api get <route>]", got.Args)
	}
	if strings.Contains(got.Args[2], projectDir) {
		t.Fatalf("route %q contains the raw project dir; it must be URL-escaped", got.Args[2])
	}
	if want := "/api/model?location%5Bdirectory%5D=" + url.QueryEscape(projectDir); got.Args[2] != want {
		t.Fatalf("route = %q, want %q", got.Args[2], want)
	}
}

// TestDiscoverCatalogV2RetriesEmptyColdStart pins the cold-start contract: a
// V2 binary may exit 0 with an empty data array while its background service
// boots; discovery must retry once and use the repopulated catalog instead
// of silently returning an empty one.
func TestDiscoverCatalogV2RetriesEmptyColdStart(t *testing.T) {
	var apiCalls int
	providers, err := discoverV2CatalogWithRunner(context.Background(), "project", func(_ context.Context, command Command) (io.Reader, error) {
		if command.Args[0] != "api" {
			t.Fatalf("unexpected non-api command during cold start: %v", command.Args)
		}
		apiCalls++
		if apiCalls == 1 {
			return strings.NewReader(`{"data":[]}`), nil
		}
		return strings.NewReader(`{"data":[{"id":"model","modelID":"model","providerID":"openai","name":"Model","enabled":true,"capabilities":{"tools":true},"variants":[]}]}`), nil
	})
	if err != nil {
		t.Fatalf("DiscoverCatalogWithRunner() error = %v", err)
	}
	if apiCalls != 2 {
		t.Fatalf("api calls = %d, want exactly one empty attempt plus one retry", apiCalls)
	}
	if _, ok := providers["openai"].Models["model"]; !ok {
		t.Fatalf("providers = %+v, want the retry's repopulated catalog", providers)
	}
}

// TestDiscoverCatalogV2KeepsEmptyCatalogWhenRetryFails pins the bounded
// retry: the first attempt legitimately reported an empty catalog, so a
// failing retry must not become an error and must not fall back to V1.
func TestDiscoverCatalogV2KeepsEmptyCatalogWhenRetryFails(t *testing.T) {
	var apiCalls, v1Calls int
	providers, err := discoverV2CatalogWithRunner(context.Background(), "project", func(_ context.Context, command Command) (io.Reader, error) {
		if command.Args[0] != "api" {
			v1Calls++
			return nil, errors.New("v1 must not run after a failed empty-retry")
		}
		apiCalls++
		if apiCalls == 1 {
			return strings.NewReader(`{"data":[]}`), nil
		}
		return &fakeExitReader{r: strings.NewReader(""), waitErr: errors.New("exit status 1")}, nil
	})
	if err != nil {
		t.Fatalf("DiscoverCatalogWithRunner() error = %v, want the empty first-attempt catalog", err)
	}
	if len(providers) != 0 {
		t.Fatalf("providers = %v, want empty", providers)
	}
	if apiCalls != 2 || v1Calls != 0 {
		t.Fatalf("api calls = %d, v1 calls = %d, want exactly one retry and no v1 fallback", apiCalls, v1Calls)
	}
}

// TestDiscoverCatalogV2FallsBackToV1OnAPIFailure pins the fallback contract:
// when the V2 api attempt fails (non-zero exit, truncation residue, schema
// drift), discovery falls back to the documented V1 stream path.
func TestDiscoverCatalogV2FallsBackToV1OnAPIFailure(t *testing.T) {
	var commands []string
	providers, err := discoverV2CatalogWithRunner(context.Background(), "project", func(_ context.Context, command Command) (io.Reader, error) {
		commands = append(commands, strings.Join(command.Args, " "))
		if command.Args[0] == "api" {
			return &fakeExitReader{r: strings.NewReader(`ERROR unknown command "api"`), waitErr: errors.New("exit status 1")}, nil
		}
		return strings.NewReader(verboseCatalog), nil
	})
	if err != nil {
		t.Fatalf("DiscoverCatalogWithRunner() error = %v, want v1 fallback success", err)
	}
	if len(commands) < 2 || commands[0] != "api get /api/model?location%5Bdirectory%5D=project" || commands[len(commands)-1] != "models --verbose" {
		t.Fatalf("commands = %v, want the api attempt followed by the v1 stream", commands)
	}
	if _, ok := providers["custom"].Models["qwen/qwen3"]; !ok {
		t.Fatalf("providers = %+v, want the v1 catalog mapped", providers)
	}
}

// TestDiscoverCatalogV2SurfacesV1ErrorWhenBothFail pins the error precedence:
// when both the V2 and the V1 path fail, the surfaced error is the V1 one —
// the path the installed binary actually documents.
func TestDiscoverCatalogV2SurfacesV1ErrorWhenBothFail(t *testing.T) {
	_, err := discoverV2CatalogWithRunner(context.Background(), "project", func(_ context.Context, command Command) (io.Reader, error) {
		if command.Args[0] == "api" {
			// Parse-level failure only: the child exited cleanly, so the
			// V2 classification (malformed) must lose to the V1 outcome.
			return strings.NewReader("not json"), nil
		}
		return &fakeExitReader{r: strings.NewReader(""), waitErr: errors.New("exit status 1")}, nil
	})
	var catalogErr *CatalogError
	if !errors.As(err, &catalogErr) || catalogErr.Kind != CatalogErrorCommandFailed {
		t.Fatalf("error = %v (%v), want the v1 command_failed outcome, not the v2 classification", err, err)
	}
}

// discoverCatalogCommandRunner must route the live V2 api attempt to the
// file-backed runner while leaving the probe and V1 stream commands on the
// pipe runner. With a bare binary name guaranteed absent from PATH, each
// route's failure shape identifies it: runAPICommand classifies internally
// (missing_binary) while runCatalogCommand returns the raw exec error for
// the discovery layer.
func TestDiscoverCatalogCommandRunnerRoutesAPItoFileRunner(t *testing.T) {
	const missing = "opencode-missing-from-path-9d27c"

	_, apiErr := discoverCatalogCommandRunner(context.Background(), Command{Path: missing, Args: []string{"api", "get", "/api/model"}})
	var apiCatalogErr *CatalogError
	if !errors.As(apiErr, &apiCatalogErr) || apiCatalogErr.Kind != CatalogErrorMissingBinary {
		t.Fatalf("api attempt err = %v (%T), want missing_binary classified by the file-backed runner", apiErr, apiErr)
	}

	_, streamErr := discoverCatalogCommandRunner(context.Background(), Command{Path: missing, Args: []string{"models", "--verbose"}})
	var streamCatalogErr *CatalogError
	if streamErr == nil || errors.As(streamErr, &streamCatalogErr) || !errors.As(streamErr, new(*exec.Error)) {
		t.Fatalf("stream attempt err = %v (%T), want the pipe runner's raw exec.Error", streamErr, streamErr)
	}
}
