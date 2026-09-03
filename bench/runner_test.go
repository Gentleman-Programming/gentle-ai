package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/xpty"
)

// fakeBinary writes an executable that answers a fixed argv with a fixed
// message, so the capability probe can be tested without a real gentle-ai.
func fakeBinary(t *testing.T, script string) *Sandbox {
	t.Helper()
	root := t.TempDir()
	sandbox, err := newSandbox(filepath.Join(root, "fake"), root)
	if err != nil {
		t.Fatalf("newSandbox: %v", err)
	}
	if err := os.MkdirAll(sandbox.Repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.WriteFile(sandbox.Binary, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	return sandbox
}

// TestTTYHelperProcess is run as a child through a real PTY.
// as the Welcome journey on every platform that supports xpty.
func TestTTYHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_TTY_HELPER") != "1" {
		return
	}
	switch os.Getenv("GO_TTY_HELPER_MODE") {
	case "hang":
		for {
			time.Sleep(time.Hour)
		}
	case "exit":
		fmt.Fprint(os.Stdout, "ready\n")
	case "burst", "quit":
		fmt.Fprint(os.Stdout, "ready\n")
		var input [1]byte
		_, _ = os.Stdin.Read(input[:])
		if os.Getenv("GO_TTY_HELPER_MODE") == "burst" {
			fmt.Fprint(os.Stdout, strings.Repeat("x", 128*1024)+"done\n")
		}
	default:
		t.Fatalf("unknown TTY helper mode %q", os.Getenv("GO_TTY_HELPER_MODE"))
	}
}

func startTTYHelper(t *testing.T, mode string) (*exec.Cmd, io.ReadWriteCloser) {
	t.Helper()
	terminal, err := xpty.NewPty(80, 24)
	if err != nil {
		t.Fatalf("new PTY: %v", err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestTTYHelperProcess")
	cmd.Env = append(os.Environ(), "GO_WANT_TTY_HELPER=1", "GO_TTY_HELPER_MODE="+mode)
	if err := terminal.Start(cmd); err != nil {
		_ = terminal.Close()
		t.Fatalf("start TTY helper: %v", err)
	}
	return cmd, terminal
}

type closeReleasesExchangeTTY struct {
	closed       chan struct{}
	exchangeDone <-chan struct{}
	read         bool
}

func (terminal *closeReleasesExchangeTTY) Read(p []byte) (int, error) {
	<-terminal.closed
	if terminal.read {
		return 0, io.EOF
	}
	terminal.read = true
	return copy(p, "ready\n"), nil
}

func (*closeReleasesExchangeTTY) Write(p []byte) (int, error) { return len(p), nil }

func (terminal *closeReleasesExchangeTTY) Close() error {
	close(terminal.closed)
	<-terminal.exchangeDone
	return nil
}

func TestRunTTYRegainsExchangeOwnershipAfterClosingTheTerminal(t *testing.T) {
	exchangeDone := make(chan struct{})
	terminal := &closeReleasesExchangeTTY{closed: make(chan struct{}), exchangeDone: exchangeDone}
	_, err := runTTYWithTimeout(&exec.Cmd{}, terminal, []string{"welcome"}, func(reader *bufio.Reader, _ io.WriteCloser) error {
		defer close(exchangeDone)
		_, err := reader.ReadString('\n')
		return err
	}, time.Second, func(context.Context, *exec.Cmd) error {
		return nil
	})
	if err != nil {
		t.Fatalf("runTTY error after Close joined the exchange = %v", err)
	}
}

type delayedDrainTTY struct {
	closed       chan struct{}
	drainStarted chan struct{}
	releaseDrain chan struct{}
}

func (terminal *delayedDrainTTY) Read([]byte) (int, error) {
	<-terminal.closed
	close(terminal.drainStarted)
	<-terminal.releaseDrain
	return 0, io.EOF
}

func (*delayedDrainTTY) Write(p []byte) (int, error) { return len(p), nil }
func (terminal *delayedDrainTTY) Close() error {
	close(terminal.closed)
	return nil
}

func TestRunTTYJoinsTranscriptDrainAfterClosingTheTerminal(t *testing.T) {
	terminal := &delayedDrainTTY{
		closed:       make(chan struct{}),
		drainStarted: make(chan struct{}),
		releaseDrain: make(chan struct{}),
	}
	result := make(chan error, 1)
	go func() {
		_, err := runTTYWithTimeout(&exec.Cmd{}, terminal, []string{"welcome"}, func(*bufio.Reader, io.WriteCloser) error {
			return nil
		}, time.Second, func(context.Context, *exec.Cmd) error {
			return nil
		})
		result <- err
	}()
	<-terminal.drainStarted
	select {
	case err := <-result:
		close(terminal.releaseDrain)
		t.Fatalf("runTTY returned before transcript drain ownership was regained: %v", err)
	case <-time.After(ttyCleanupGrace + 100*time.Millisecond):
	}
	close(terminal.releaseDrain)
	if err := <-result; err != nil {
		t.Fatalf("runTTY error after joining the drain = %v", err)
	}
}

func TestRunTTYTimeoutKillsAndReapsTheHelper(t *testing.T) {
	cmd, terminal := startTTYHelper(t, "hang")
	waitCalls := 0
	started := time.Now()
	observation, err := runTTYWithTimeout(cmd, terminal, []string{"welcome"}, func(*bufio.Reader, io.WriteCloser) error {
		return nil
	}, 100*time.Millisecond, func(ctx context.Context, cmd *exec.Cmd) error {
		waitCalls++
		if _, ok := ctx.Deadline(); ok {
			return errors.New("WaitProcess context has a deadline")
		}
		return xpty.WaitProcess(ctx, cmd)
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("runTTY timeout error = %v, want context deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("runTTY timeout took %v, want bounded cleanup", elapsed)
	}
	if waitCalls != 1 {
		t.Fatalf("WaitProcess calls = %d, want 1", waitCalls)
	}
	if cmd.ProcessState == nil {
		t.Fatal("helper process was not reaped")
	}
	if observation.ExitCode != -1 {
		t.Fatalf("timeout exit code = %d, want -1", observation.ExitCode)
	}
}

func TestRunTTYContinuesDrainingAfterTheExchange(t *testing.T) {
	cmd, terminal := startTTYHelper(t, "burst")
	observation, err := runTTYWithTimeout(cmd, terminal, []string{"welcome"}, func(reader *bufio.Reader, writer io.WriteCloser) error {
		if line, err := reader.ReadString('\n'); err != nil || line != "ready\r\n" {
			return fmt.Errorf("prompt = %q, %v; want ready", line, err)
		}
		_, err := io.WriteString(writer, "q\n")
		return err
	}, 5*time.Second, xpty.WaitProcess)
	if err != nil {
		t.Fatalf("runTTY drain error = %v", err)
	}
	if !strings.Contains(observation.Stdout, "done") || len(observation.Stdout) < 128*1024 {
		t.Fatalf("transcript was not drained after the exchange: %d bytes", len(observation.Stdout))
	}
	if cmd.ProcessState == nil {
		t.Fatal("helper process was not reaped")
	}
}

func TestRunTTYCleansUpAfterARealExchangeFailure(t *testing.T) {
	cmd, terminal := startTTYHelper(t, "hang")
	exchangeFailure := errors.New("exchange failed")
	waitCalls := 0
	_, err := runTTYWithTimeout(cmd, terminal, []string{"welcome"}, func(*bufio.Reader, io.WriteCloser) error {
		return exchangeFailure
	}, time.Second, func(ctx context.Context, cmd *exec.Cmd) error {
		waitCalls++
		return xpty.WaitProcess(ctx, cmd)
	})
	if !errors.Is(err, exchangeFailure) {
		t.Fatalf("runTTY error = %v, want exchange failure", err)
	}
	if waitCalls != 1 {
		t.Fatalf("WaitProcess calls = %d, want 1", waitCalls)
	}
	if cmd.ProcessState == nil {
		t.Fatal("helper process was not reaped")
	}
}

func TestInvokeTTYReportsPromptStartFailure(t *testing.T) {
	sandbox := fakeBinary(t, "")
	sandbox.Binary = filepath.Join(t.TempDir(), "missing-gentle-ai")
	observation, err := sandbox.invokeTTY(sandbox.Repo, []string{"welcome"}, func(*bufio.Reader, io.WriteCloser) error { return nil })
	if err == nil {
		t.Fatal("invokeTTY error = nil, want prompt start failure")
	}
	if observation.ExitCode != -1 || !strings.Contains(observation.Stderr, "bench:") {
		t.Fatalf("start observation = %+v, want failed bench observation", observation)
	}
}

type closeErrorTTY struct {
	io.ReadWriteCloser
	closeCalls int
	err        error
}

func (tty *closeErrorTTY) Close() error {
	tty.closeCalls++
	_ = tty.ReadWriteCloser.Close()
	return tty.err
}

func TestRunTTYJoinsCloseAndDeterministicWaitErrors(t *testing.T) {
	cmd, pty := startTTYHelper(t, "hang")
	waitFailure := errors.New("wait failed")
	closeFailure := errors.New("close failed")
	terminal := &closeErrorTTY{ReadWriteCloser: pty, err: closeFailure}
	waitCalls := 0
	observation, err := runTTYWithTimeout(cmd, terminal, []string{"welcome"}, func(*bufio.Reader, io.WriteCloser) error {
		return errors.New("exchange failed")
	}, time.Second, func(ctx context.Context, cmd *exec.Cmd) error {
		waitCalls++
		_ = xpty.WaitProcess(ctx, cmd)
		return waitFailure
	})
	if !errors.Is(err, waitFailure) || !errors.Is(err, closeFailure) {
		t.Fatalf("runTTY error = %v, want joined wait and close failures", err)
	}
	if observation.ExitCode != -1 || !strings.Contains(observation.Stderr, "wait failed") {
		t.Fatalf("wait observation = %+v, want failed bench observation", observation)
	}
	if waitCalls != 1 || terminal.closeCalls != 1 {
		t.Fatalf("WaitProcess/Close calls = %d/%d, want 1/1", waitCalls, terminal.closeCalls)
	}
	if cmd.ProcessState == nil {
		t.Fatal("helper process was not reaped")
	}
}

// A build that HAS the flag fails on state, not on shape. The probe must read
// that as supported: "sdd-attempt requires --cwd" is the repository's answer,
// not the CLI's.

func TestProbeCapabilityAcceptsAStateFailure(t *testing.T) {
	sandbox := fakeBinary(t, `echo "Error: sdd-attempt requires --cwd" >&2; exit 1`)
	capability := &Capability{
		Verb:  []string{"sdd-attempt", "finish"},
		Probe: []string{"sdd-attempt", "finish", "--expected-binding-revision=probe"},
	}
	supported, reason := newCapabilityProbe(sandbox).supported(capability)
	if !supported {
		t.Fatalf("supported = false (%s), want true: a state failure is not a missing surface", reason)
	}
}

// A build that LACKS the flag rejects the shape, and the journey must record
// `unsupported` rather than a state failure it never had.
func TestProbeCapabilityRejectsAMissingFlag(t *testing.T) {
	sandbox := fakeBinary(t, `echo "Error: flag provided but not defined: -expected-binding-revision" >&2; exit 1`)
	capability := &Capability{
		Verb:  []string{"sdd-attempt", "finish"},
		Probe: []string{"sdd-attempt", "finish", "--expected-binding-revision=probe"},
	}
	supported, reason := newCapabilityProbe(sandbox).supported(capability)
	if supported {
		t.Fatal("supported = true, want false: the binary rejected the shape of the command")
	}
	if reason == "" {
		t.Fatal("an unsupported surface must say which argv it probed")
	}
}

func TestHelpProbeRejectsALegacySurface(t *testing.T) {
	sandbox := fakeBinary(t, `echo "Error: flag provided but not defined: -help" >&2; exit 1`)
	supported, _ := newCapabilityProbe(sandbox).supported(&Capability{Verb: []string{"legacy", "finish"}})
	if supported {
		t.Fatal("the help read should have reported unsupported for this fake")
	}
}

// Probe and the default help read must not share a cache slot, or one verb's
// answer would silently decide the other's.
func TestProbeAndHelpProbeDoNotShareACacheEntry(t *testing.T) {
	sandbox := fakeBinary(t, `
case "$*" in
  *--help*) echo "Error: flag provided but not defined: -help" >&2; exit 1 ;;
  *)        echo "Error: sdd-attempt requires --cwd" >&2; exit 1 ;;
esac`)
	probe := newCapabilityProbe(sandbox)
	if supported, _ := probe.supported(&Capability{Verb: []string{"legacy", "finish"}}); supported {
		t.Fatal("the help read should have reported unsupported for this fake")
	}
	supported, reason := probe.supported(&Capability{
		Verb:  []string{"legacy", "finish"},
		Probe: []string{"legacy", "finish", "--expected-binding-revision=probe"},
	})
	if !supported {
		t.Fatalf("supported = false (%s), want true: the probe answer must not come from the help cache", reason)
	}
}

// readBack must never charge its git subprocesses to the next counted
// invocation, or an uncounted proof would inflate a measured dimension.
func TestReadBackBlanksGitTrace(t *testing.T) {
	sandbox := fakeBinary(t, `echo "GIT_TRACE=[$GIT_TRACE]"`)
	observation := sandbox.readBack("sdd-attempt", "status")
	if observation.Stdout != "GIT_TRACE=[]\n" {
		t.Fatalf("readBack stdout = %q, want a blanked GIT_TRACE", observation.Stdout)
	}
	counted := sandbox.invoke([]string{"sdd-attempt", "status"})
	if counted.Stdout == "GIT_TRACE=[]\n" {
		t.Fatal("a counted invocation lost GIT_TRACE, so git_subprocesses would stop being observable")
	}
}

// reviewModeStubBinary writes an executable that logs every argv it is given
// and answers `review mode enable` with the effective mode the test wants,
// so the review precondition can be tested without a real gentle-ai.
func reviewModeStubBinary(t *testing.T, effective string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	binary := filepath.Join(dir, "gentle-ai-stub")
	log := filepath.Join(dir, "argv.log")
	script := "#!/bin/sh\n" +
		"echo \"$*\" >> " + log + "\n" +
		"case \"$1 $2 $3\" in\n" +
		"'review mode enable') printf '{\"status\":{\"effective\":\"" + effective + "\"}}\\n'; exit 0;;\n" +
		"esac\n" +
		"exit 0\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatalf("write review mode stub: %v", err)
	}
	return binary, log
}

func stubJourney(precondition ReviewPrecondition) Journey {
	return Journey{
		ID:     "jStub",
		Review: precondition,
		Steps: []Step{{
			Name:    "journey command",
			Fixture: func(sandbox *Sandbox) error { return os.MkdirAll(sandbox.Repo, 0o755) },
			Args:    func(*Sandbox) ([]string, error) { return []string{"review", "status"}, nil },
		}},
	}
}

func argvLines(t *testing.T, log string) []string {
	t.Helper()
	content, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("read argv log: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(content)), "\n")
}

// An opted-in journey has to opt in the way a user does — through the product's
// own command — and it has to do it BEFORE the first command it measures.
func TestOptedInJourneyEnablesReviewModeFirst(t *testing.T) {
	binary, log := reviewModeStubBinary(t, "on")
	result := runJourney(binary, stubJourney(reviewOptedIn))
	if result.Status != StatusCompleted {
		t.Fatalf("status = %s (%s), want completed", result.Status, result.FailureReason)
	}
	lines := argvLines(t, log)
	if lines[0] != "review mode enable --scope global --json" {
		t.Fatalf("first invocation = %q, want the global opt-in before anything the journey measures", lines[0])
	}
	if len(lines) != 2 || lines[1] != "review status" {
		t.Fatalf("invocations = %#v, want the opt-in followed by the journey's own command", lines)
	}
}

// The opt-in is read back, not assumed. A product that answers the enable with
// the switch still off must fail the journey loudly: measuring a review-off
// flow under a journey that says it reviews is the silent degradation this
// declaration exists to stop.
func TestOptedInJourneyFailsWhenTheSwitchDoesNotComeOn(t *testing.T) {
	binary, log := reviewModeStubBinary(t, "off")
	result := runJourney(binary, stubJourney(reviewOptedIn))
	if result.Status != StatusFailed {
		t.Fatalf("status = %s, want failed when the switch never came on", result.Status)
	}
	if !strings.Contains(result.FailureReason, "reviews off") {
		t.Fatalf("failure reason = %q, want it to say the journey would have measured a flow with reviews off", result.FailureReason)
	}
	if lines := argvLines(t, log); len(lines) != 1 {
		t.Fatalf("invocations = %#v, want the journey's own commands never to have run", lines)
	}
}

// A journey whose subject IS the switch declares reviewUntouched, and the
// runner must then keep its hands off: reaching in first would overwrite the
// very state under test.
func TestUntouchedJourneyNeverTouchesTheSwitch(t *testing.T) {
	binary, log := reviewModeStubBinary(t, "on")
	result := runJourney(binary, stubJourney(reviewUntouched))
	if result.Status != StatusCompleted {
		t.Fatalf("status = %s (%s), want completed", result.Status, result.FailureReason)
	}
	if lines := argvLines(t, log); len(lines) != 1 || lines[0] != "review status" {
		t.Fatalf("invocations = %#v, want only the journey's own command", lines)
	}
}

func TestSandboxEnvKeepsTempFilesInsideTheSandbox(t *testing.T) {
	sandbox, err := newSandbox("gentle-ai", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(sandbox.Root, "tmp")
	found := map[string]bool{}
	for _, entry := range sandbox.env() {
		if entry == "TMP="+want {
			found["TMP"] = true
			continue
		}
		if entry == "TEMP="+want {
			found["TEMP"] = true
			continue
		}
		if entry == "TMPDIR="+want {
			found["TMPDIR"] = true
			continue
		}
		if strings.HasPrefix(entry, "TMP=") || strings.HasPrefix(entry, "TEMP=") || strings.HasPrefix(entry, "TMPDIR=") {
			t.Fatalf("temporary directory = %q, want %q", entry, want)
		}
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("sandbox temp directory %q: %v", want, err)
	}
	if !found["TMP"] || !found["TEMP"] || !found["TMPDIR"] {
		t.Fatalf("sandbox temp variables = %v, want TMP, TEMP and TMPDIR", found)
	}
}

func TestSandboxEnvKeepsWindowsHomeInsideTheSandbox(t *testing.T) {
	sandbox, err := newSandbox("gentle-ai", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range sandbox.env() {
		if entry == "USERPROFILE="+sandbox.Home {
			return
		}
	}
	t.Fatalf("sandbox environment has no USERPROFILE=%q", sandbox.Home)
}

func TestSelectedAuthorityCaptureHelpersSelectTheSandboxLineage(t *testing.T) {
	tests := []struct {
		name string
		run  func(*journeyRun) error
	}{
		{name: "results", run: sddCaptureSelectedAuthorityLenses},
		{name: "evidence", run: sddCaptureSelectedAuthorityEvidence},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sandbox := fakeBinary(t, `printf '%s\n' '{"next_transition":{"kind":"complete"}}'`)
			sandbox.Lineage = "review-sdd-newer"
			run := &journeyRun{sandbox: sandbox, accumulator: newAccumulator(), step: tt.name}

			if err := tt.run(run); err != nil {
				t.Fatalf("capture helper: %v", err)
			}
			if len(run.accumulator.records) != 1 {
				t.Fatalf("invocations = %d, want 1", len(run.accumulator.records))
			}
			args := run.accumulator.records[0].Args
			if len(args) < 2 || args[len(args)-2] != "--lineage" || args[len(args)-1] != sandbox.Lineage {
				t.Fatalf("status args = %q, want selected lineage %q", args, sandbox.Lineage)
			}
		})
	}
}
