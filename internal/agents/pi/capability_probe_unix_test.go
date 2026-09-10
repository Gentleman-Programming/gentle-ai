//go:build !windows

package pi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func writeTransportScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "model-routing")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env sh\nset -eu\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
func TestUnixTransportEmptyPathUsesOneFallback(t *testing.T) {
	t.Setenv("PATH", "")
	env := strings.Join(transportEnvironment(), "\n")
	if got := strings.Count(env, "PATH="); got != 1 || !strings.Contains(env, "PATH=/usr/local/bin:/usr/bin:/bin") {
		t.Fatalf("PATH entries = %q; want one nonempty fallback", env)
	}
}
func TestUnixTransportUsesDirectProcessContract(t *testing.T) {
	fixture := t.TempDir()
	stdinPath := filepath.Join(fixture, "stdin")
	argsPath := filepath.Join(fixture, "args")
	argv0Path := filepath.Join(fixture, "argv0")
	cwdPath := filepath.Join(fixture, "cwd")
	envPath := filepath.Join(fixture, "env")
	secret := "model-routing-secret-token"
	body := fmt.Sprintf("cat > %s\nprintf '%%s\\n' \"$#\" > %s\nprintf '%%s\\n' \"$0\" > %s\npwd > %s\nenv > %s", shellQuote(stdinPath), shellQuote(argsPath), shellQuote(argv0Path), shellQuote(cwdPath), shellQuote(envPath))
	path := writeTransportScript(t, body)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MODEL_ROUTING_SECRET_TOKEN", secret)
	t.Setenv("AWS_ACCESS_KEY_ID", "access-key-must-not-cross-boundary")
	t.Setenv("HOME", "/sensitive-home")
	request := []byte("first line\nsecond\x00line\n")
	options := validTransportOptions()
	options.MaxRequestBytes = len(request)
	options.MaxStdoutBytes = 1
	result, err := RunBoundedModelRoutingProcess(context.Background(), path, request, options)
	if err != nil {
		t.Fatalf("transport error = %v", err)
	}
	if result.ExitCode != 0 || len(result.Stdout) != 0 || result.StderrBytes != 0 {
		t.Fatalf("result = %+v; want a silent successful process", result)
	}
	for file, want := range map[string][]byte{
		stdinPath: request,
		argsPath:  []byte("0\n"),
		argv0Path: []byte(path + "\n"),
		cwdPath:   []byte(transportWorkingDirectory + "\n"),
	} {
		got, readErr := os.ReadFile(file)
		if readErr != nil || string(got) != string(want) {
			t.Fatalf("%s = %q, read error = %v; want %q", file, got, readErr, want)
		}
	}
	env, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"PATH": true, "PWD": true, "LANG": true, "LANGUAGE": true, "LC_ALL": true, "LC_COLLATE": true, "LC_CTYPE": true, "LC_MESSAGES": true, "LC_MONETARY": true, "LC_NUMERIC": true, "LC_TIME": true, "TMPDIR": true, "TMP": true, "TEMP": true}
	for _, line := range strings.Split(strings.TrimSpace(string(env)), "\n") {
		key, _, ok := strings.Cut(line, "=")
		if !ok || !allowed[key] || strings.Contains(line, secret) {
			t.Fatalf("unexpected child environment entry %q", line)
		}
	}
	if strings.Contains(string(env), "access-key-must-not-cross-boundary") || strings.Contains(string(env), "sensitive-home") {
		t.Fatalf("sensitive environment crossed boundary: %q", env)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("executable changed, read error = %v", err)
	}
}
func TestUnixTransportOutputBoundariesAndPrecedence(t *testing.T) {
	secret := "stderr-under-limit-secret"
	cases := []struct {
		name, body     string
		stdout, stderr int
		wantOutput     string
		wantBytes      int
		wantKind       TransportErrorKind
		wantCode       int
	}{
		{name: "stdout exact", body: "printf '%s' 'abcd'", stdout: 4, stderr: 4, wantOutput: "abcd"},
		{name: "stdout overflow before nonzero", body: "printf '%s' 'abcdef'; exit 7", stdout: 4, stderr: 4, wantOutput: "abcde", wantKind: TransportErrorStdoutOverflow, wantCode: 7},
		{name: "stderr exact", body: "printf '%s' 'wxyz' >&2", stdout: 4, stderr: 4, wantBytes: 4},
		{name: "stderr overflow", body: "printf '%s' 'wxyza' >&2", stdout: 4, stderr: 4, wantKind: TransportErrorStderrOverflow, wantBytes: 5},
		{name: "stderr is not exposed", body: fmt.Sprintf("printf '%%s' %s >&2", shellQuote(secret)), stdout: 4, stderr: len(secret), wantBytes: len(secret)},
		{name: "both overflow selects stdout", body: "printf '%s' 'abcde'; printf '%s' 'wxyza' >&2; exit 7", stdout: 4, stderr: 4, wantOutput: "abcde", wantBytes: 5, wantKind: TransportErrorStdoutOverflow, wantCode: 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			options := validTransportOptions()
			options.MaxStdoutBytes = tc.stdout
			options.MaxStderrBytes = tc.stderr
			result, err := RunBoundedModelRoutingProcess(context.Background(), writeTransportScript(t, tc.body), nil, options)
			if tc.wantKind == "" {
				if err != nil {
					t.Fatalf("error = %v; want success", err)
				}
			} else {
				requireTransportKind(t, err, tc.wantKind)
			}
			if string(result.Stdout) != tc.wantOutput || result.StderrBytes != tc.wantBytes || result.ExitCode != tc.wantCode {
				t.Fatalf("result = %+v; want stdout %q, stderr bytes %d, exit %d", result, tc.wantOutput, tc.wantBytes, tc.wantCode)
			}
			if (err != nil && strings.Contains(err.Error(), secret)) || strings.Contains(fmt.Sprintf("%+v", result), secret) {
				t.Fatalf("stderr was exposed: error=%v result=%+v", err, result)
			}
		})
	}
}
func TestUnixTransportReportsStartAndNonzeroCauses(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	_, err := RunBoundedModelRoutingProcess(context.Background(), missing, nil, validTransportOptions())
	requireTransportKind(t, err, TransportErrorStart)
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("start cause = %T %v; want *os.PathError", err, err)
	}
	options := validTransportOptions()
	options.MaxStderrBytes = len("failure")
	result, err := RunBoundedModelRoutingProcess(context.Background(), writeTransportScript(t, "printf '%s' 'failure' >&2; exit 7"), nil, options)
	requireTransportKind(t, err, TransportErrorNonzeroExit)
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || result.ExitCode != 7 {
		t.Fatalf("nonzero result = %+v, error = %v; want exit cause and code 7", result, err)
	}
}
func TestUnixBoundedTransportOutputRetainsOnlyLimitPlusOne(t *testing.T) {
	for _, tc := range []struct {
		name, input, retained string
		limit, count          int
		overflow              bool
	}{
		{"exact", "abc", "abc", 3, 3, false}, {"one-over", "abcd", "abcd", 3, 4, true},
		{"many-over", "abcdef", "abcd", 3, 6, true}, {"zero-limit", "a", "a", 0, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := &boundedTransportOutput{limit: tc.limit}
			n, err := output.Write([]byte(tc.input))
			if err != nil || n != len(tc.input) || string(output.buf.Bytes()) != tc.retained || output.count != tc.count || output.overflow != tc.overflow {
				t.Fatalf("Write()=(%d,%v), output=%q count=%d overflow=%v", n, err, output.buf.Bytes(), output.count, output.overflow)
			}
		})
	}
}
func TestUnixDrainTransportOutputConsumesReader(t *testing.T) {
	output := &boundedTransportOutput{limit: 3}
	done := drainTransportOutput(strings.NewReader("drained"), output)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("drain did not finish")
	}
	if got := string(output.buf.Bytes()); got != "drai" || !output.overflow || len(output.buf.Bytes()) > output.limit+1 {
		t.Fatalf("drained output=%q overflow=%v", got, output.overflow)
	}
}

type transportEvidence struct{ process, descendant int }
type transportOutcome struct {
	result ModelRoutingProcessResult
	err    error
}

func hangingTransportScript(t *testing.T) (string, string) {
	evidence := filepath.Join(t.TempDir(), "process.evidence")
	body := fmt.Sprintf(`(sleep 30) &
child=$!
tmp=%s.tmp.$$
printf '%%s\n%%s\n' "$$" "$child" > "$tmp"
mv "$tmp" %s
while :; do sleep 1; done`, shellQuote(evidence), shellQuote(evidence))
	return writeTransportScript(t, body), evidence
}
func readTransportEvidence(data []byte) (transportEvidence, error) {
	lines := strings.Split(string(data), "\n")
	if len(lines) != 3 || lines[2] != "" || lines[0] == "" || lines[1] == "" {
		return transportEvidence{}, fmt.Errorf("malformed process evidence %q", data)
	}
	process, err := strconv.Atoi(lines[0])
	if err != nil || process <= 0 {
		return transportEvidence{}, fmt.Errorf("invalid process pid %q", lines[0])
	}
	descendant, err := strconv.Atoi(lines[1])
	if err != nil || descendant <= 0 {
		return transportEvidence{}, fmt.Errorf("invalid descendant pid %q", lines[1])
	}
	return transportEvidence{process: process, descendant: descendant}, nil
}

func waitForTransportEvidence(path string) (transportEvidence, error) {
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			return readTransportEvidence(data)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return transportEvidence{}, fmt.Errorf("read process evidence: %w", err)
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			return transportEvidence{}, fmt.Errorf("process evidence did not become readable")
		}
	}
}

func waitForTransportPIDDeath(pid int) error {
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("checking pid %d: %w", pid, err)
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			return fmt.Errorf("pid %d remained alive", pid)
		}
	}
}

func runTransportTerminationCase(t *testing.T, canceled bool) {
	t.Helper()
	script, evidencePath := hangingTransportScript(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	timeout := 100 * time.Millisecond
	wantKind, wantCause := TransportErrorTimeout, context.DeadlineExceeded
	if canceled {
		timeout, wantKind, wantCause = 5*time.Second, TransportErrorCanceled, context.Canceled
	}
	done := make(chan transportOutcome, 1)
	go func() {
		result, err := RunBoundedModelRoutingProcess(ctx, script, nil, ModelRoutingProcessOptions{Timeout: timeout, MaxRequestBytes: 1, MaxStdoutBytes: 4, MaxStderrBytes: 4})
		done <- transportOutcome{result: result, err: err}
	}()
	evidence, err := waitForTransportEvidence(evidencePath)
	if err != nil {
		cancel()
		t.Fatalf("readiness evidence: %v", err)
	}
	for _, pid := range []int{evidence.process, evidence.descendant} {
		if err := syscall.Kill(pid, 0); err != nil {
			cancel()
			t.Fatalf("readiness pid %d: %v", pid, err)
		}
	}
	if canceled {
		cancel()
	}
	var outcome transportOutcome
	select {
	case outcome = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("transport did not return")
	}
	requireTransportKind(t, outcome.err, wantKind)
	if !errors.Is(outcome.err, wantCause) {
		t.Fatalf("termination cause was not preserved: %v", outcome.err)
	}
	for _, pid := range []int{evidence.process, evidence.descendant} {
		if err := waitForTransportPIDDeath(pid); err != nil {
			t.Fatalf("ordinary descendant survived: %v", err)
		}
	}
}

func TestUnixTransportCancelKillsOrdinaryDescendantsAndReaps(t *testing.T) {
	runTransportTerminationCase(t, true)
}

func TestUnixTransportTimeoutKillsOrdinaryDescendantsAndReaps(t *testing.T) {
	runTransportTerminationCase(t, false)
}
func TestUnixTransportEvidenceRejectsMalformedOrUnreadableRecords(t *testing.T) {
	malformed := filepath.Join(t.TempDir(), "malformed")
	if err := os.WriteFile(malformed, []byte("not-a-pid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(t.TempDir(), "unreadable")
	if err := os.Mkdir(unreadable, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{malformed, unreadable} {
		if _, err := waitForTransportEvidence(path); err == nil {
			t.Fatalf("invalid evidence %q was accepted", path)
		}
	}
}
