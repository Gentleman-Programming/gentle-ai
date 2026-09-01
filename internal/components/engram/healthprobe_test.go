package engram

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// setStdioHelperProcess routes the execCommandContext seam through this test
// binary re-executing TestHelperEngramMCPProcess in the given mode, so the
// handshake runs against a real child process speaking real stdio (#2078
// matrix cell b).
func setStdioHelperProcess(t *testing.T, mode string, env ...string) func() *exec.Cmd {
	t.Helper()
	orig := execCommandContext
	var helper *exec.Cmd
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		// -test.timeout keeps a pending runtime timer alive in the helper. An
		// idle helper parked only in select{} has no syscall-blocked goroutine,
		// so on Windows the child runtime declares "all goroutines are asleep",
		// exits before the probe context ends, and the parent reads a clean
		// stdout EOF instead of the context cause.
		cs := append([]string{"-test.run=TestHelperEngramMCPProcess", "-test.timeout=1m", "--", name}, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "GO_ENGRAM_HELPER_MODE="+mode)
		cmd.Env = append(cmd.Env, env...)
		// The probe discards child stderr; surface helper crashes in test output.
		cmd.Stderr = os.Stderr
		helper = cmd
		return cmd
	}
	t.Cleanup(func() { execCommandContext = orig })
	return func() *exec.Cmd { return helper }
}

// TestHelperEngramMCPProcess is not a real test: it is the fake engram
// endpoint. When re-executed with GO_ENGRAM_HELPER_MODE set it emulates an
// engram MCP server over stdio in one of several modes, then exits without
// letting the test framework print to stdout (stdout is the MCP channel).
func TestHelperEngramMCPProcess(t *testing.T) {
	mode := os.Getenv("GO_ENGRAM_HELPER_MODE")
	if mode == "" {
		return
	}
	defer os.Exit(0)

	switch mode {
	case "healthy", "rpc-error", "null-result", "scalar-result", "missing-protocol-version", "invalid-capabilities", "missing-server-info", "invalid-server-info", "missing-jsonrpc", "wrong-jsonrpc", "null-jsonrpc", "request-shaped-result", "missing-result", "malformed-result", "early-exit", "stray-stdout-then-healthy", "mismatched-id-then-healthy", "trailing-stdout-after-healthy", "descendant-holds-stdout", "descendant-holds-stdout-before-response":
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		var req struct {
			ID     json.Number `json:"id"`
			Method string      `json:"method"`
		}
		if json.Unmarshal([]byte(line), &req) != nil || req.Method != "initialize" {
			return
		}
		if mode == "rpc-error" {
			fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%s,\"error\":{\"code\":-32603,\"message\":\"store locked\"}}\n", req.ID.String())
			return
		}
		if mode == "early-exit" {
			return
		}
		healthyResponse := fmt.Sprintf("{\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{},\"serverInfo\":{\"name\":\"fake-engram\",\"version\":\"0\"}}}", req.ID.String())
		switch mode {
		case "missing-jsonrpc":
			fmt.Printf("{\"id\":%s,\"result\":{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{},\"serverInfo\":{\"name\":\"fake-engram\",\"version\":\"0\"}}}\n", req.ID.String())
			return
		case "wrong-jsonrpc":
			fmt.Printf("{\"jsonrpc\":\"1.0\",\"id\":%s,\"result\":{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{},\"serverInfo\":{\"name\":\"fake-engram\",\"version\":\"0\"}}}\n", req.ID.String())
			return
		case "null-jsonrpc":
			fmt.Printf("{\"jsonrpc\":null,\"id\":%s,\"result\":{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{},\"serverInfo\":{\"name\":\"fake-engram\",\"version\":\"0\"}}}\n", req.ID.String())
			return
		case "request-shaped-result":
			fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%s,\"method\":\"notifications/test\",\"result\":{\"protocolVersion\":\"2024-11-05\",\"capabilities\":{},\"serverInfo\":{\"name\":\"fake-engram\",\"version\":\"0\"}}}\n", req.ID.String())
			return
		case "missing-result":
			fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%s}\n", req.ID.String())
			return
		case "malformed-result":
			fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":\n", req.ID.String())
			return
		case "stray-stdout-then-healthy":
			fmt.Println("engram mcp starting")
			fmt.Println(healthyResponse)
			return
		case "mismatched-id-then-healthy":
			fmt.Println(strings.Replace(healthyResponse, `"id":1`, `"id":2`, 1))
			fmt.Println(healthyResponse)
			return
		case "trailing-stdout-after-healthy":
			// A single small pipe write makes both frames available before the
			// parent can terminate this helper.
			_, _ = os.Stdout.WriteString(healthyResponse + "\nengram mcp stopped\n")
			return
		case "descendant-holds-stdout", "descendant-holds-stdout-before-response":
			// -test.timeout: same pending-timer lifeline as setStdioHelperProcess.
			descendant := exec.Command(os.Args[0], "-test.run=TestHelperEngramMCPProcess", "-test.timeout=1m", "--", "descendant")
			descendant.Env = append(os.Environ(), "GO_ENGRAM_HELPER_MODE=hold-stdout")
			descendant.Stdout = os.Stdout
			descendant.Stderr = os.Stderr
			if err := descendant.Start(); err != nil {
				return
			}
			if path := os.Getenv("GO_ENGRAM_DESCENDANT_PID_FILE"); path != "" {
				_ = os.WriteFile(path, []byte(strconv.Itoa(descendant.Process.Pid)), 0o600)
			}
			if mode == "descendant-holds-stdout-before-response" {
				select {}
			}
			fmt.Println(healthyResponse)
			select {}
		}
		result := map[string]string{
			"healthy":                  `{"protocolVersion":"2024-11-05","capabilities":{},"serverInfo":{"name":"fake-engram","version":"0"}}`,
			"null-result":              `null`,
			"scalar-result":            `"not-an-object"`,
			"missing-protocol-version": `{"capabilities":{},"serverInfo":{"name":"fake-engram","version":"0"}}`,
			"invalid-capabilities":     `{"protocolVersion":"2024-11-05","capabilities":[],"serverInfo":{"name":"fake-engram","version":"0"}}`,
			"missing-server-info":      `{"protocolVersion":"2024-11-05","capabilities":{}}`,
			"invalid-server-info":      `{"protocolVersion":"2024-11-05","capabilities":{},"serverInfo":{"name":"","version":0}}`,
		}[mode]
		if mode == "healthy" {
			// MCP diagnostics are not protocol frames and therefore belong on stderr.
			fmt.Fprintln(os.Stderr, "engram mcp starting")
		}
		fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":%s}\n", req.ID.String(), result)
		if mode == "healthy" {
			// Stay alive until the probe tears the process down.
			select {}
		}
	case "garbage":
		fmt.Println("this is not a JSON-RPC message")
	case "silent":
		select {}
	case "hold-stdout":
		select {}
	}
}

func TestStdioHandshake_TerminatesDescendantHoldingStdout(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "descendant.pid")
	helperCommand := setStdioHelperProcess(t, "descendant-holds-stdout", "GO_ENGRAM_DESCENDANT_PID_FILE="+pidPath)
	result := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		result <- stdioHandshake(context.Background(), stdioProbeTimeout, "engram", "mcp", "--tools=agent")
	}()

	pid := waitForHelperPID(t, pidPath)
	t.Cleanup(func() {
		terminateTestProcess(pid)
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Error("stdioHandshake() goroutine did not exit after descendant cleanup")
		}
	})

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("stdioHandshake() error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("stdioHandshake() waited for a descendant that inherited stdout")
	}

	command := helperCommand()
	if command == nil {
		t.Fatal("stdioHandshake() did not start its helper child")
	}
	if command.ProcessState == nil {
		t.Fatalf("stdioHandshake() did not reap its direct child: state=%#v", command.ProcessState)
	}
	assertTestProcessExited(t, pid)
}

func TestStdioHandshake_TerminatesDescendantOnContextEnd(t *testing.T) {
	tests := []struct {
		name       string
		newContext func() (context.Context, context.CancelFunc)
		cancel     bool
		want       error
	}{
		{
			name: "cancellation",
			newContext: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.Background())
			},
			cancel: true,
			want:   context.Canceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pidPath := filepath.Join(t.TempDir(), "descendant.pid")
			helperCommand := setStdioHelperProcess(t, "descendant-holds-stdout-before-response", "GO_ENGRAM_DESCENDANT_PID_FILE="+pidPath)
			ctx, cancel := tt.newContext()
			defer cancel()
			result := make(chan error, 1)
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				result <- stdioHandshake(ctx, stdioProbeTimeout, "engram", "mcp", "--tools=agent")
			}()

			pid := waitForHelperPID(t, pidPath)
			t.Cleanup(func() {
				terminateTestProcess(pid)
				select {
				case <-finished:
				case <-time.After(time.Second):
					t.Error("stdioHandshake() goroutine did not exit after descendant cleanup")
				}
			})
			if tt.cancel {
				cancel()
			}

			select {
			case err := <-result:
				if err != tt.want {
					t.Fatalf("stdioHandshake() error = %v, want %v", err, tt.want)
				}
			case <-time.After(time.Second):
				t.Fatalf("stdioHandshake() did not return after context %v", tt.want)
			}

			command := helperCommand()
			if command == nil || command.ProcessState == nil {
				t.Fatal("stdioHandshake() did not reap its direct child")
			}
			assertTestProcessExited(t, pid)
		})
	}
}

func TestWaitForHelperPID_RetriesTransientEmptyContent(t *testing.T) {
	const wantPID = 2858
	pidPath := filepath.Join(t.TempDir(), "descendant.pid")
	if err := os.WriteFile(pidPath, nil, 0o600); err != nil {
		t.Fatalf("create empty descendant pid file: %v", err)
	}

	firstRead := make(chan []byte, 1)
	allowRetry := make(chan struct{})
	readCount := 0
	readFile := func(path string) ([]byte, error) {
		raw, err := os.ReadFile(path)
		if readCount == 0 {
			firstRead <- raw
			<-allowRetry
		}
		readCount++
		return raw, err
	}
	type waitResult struct {
		pid int
		err error
	}
	result := make(chan waitResult, 1)
	go func() {
		pid, err := waitForHelperPIDWithReadFile(pidPath, 100*time.Millisecond, readFile)
		result <- waitResult{pid: pid, err: err}
	}()
	t.Cleanup(func() {
		select {
		case <-allowRetry:
		default:
			close(allowRetry)
		}
	})

	select {
	case raw := <-firstRead:
		if string(raw) != "" {
			t.Fatalf("first descendant pid content = %q, want empty", raw)
		}
	case <-time.After(time.Second):
		t.Fatal("waitForHelperPID() did not read the empty pid file")
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(wantPID)), 0o600); err != nil {
		t.Fatalf("write descendant pid: %v", err)
	}
	close(allowRetry)

	select {
	case got := <-result:
		if got.err != nil {
			t.Fatalf("waitForHelperPID() error = %v", got.err)
		}
		if got.pid != wantPID {
			t.Fatalf("descendant pid = %d, want %d", got.pid, wantPID)
		}
		if readCount < 2 {
			t.Fatalf("descendant pid reads = %d, want retry after empty content", readCount)
		}
	case <-time.After(time.Second):
		t.Fatal("waitForHelperPID() did not retry after empty pid content")
	}
}

func TestWaitForHelperPIDWithTimeout_PermanentMalformedContent(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "descendant.pid")
	if err := os.WriteFile(pidPath, []byte("not-a-pid"), 0o600); err != nil {
		t.Fatalf("write malformed descendant pid: %v", err)
	}

	_, err := waitForHelperPIDWithTimeout(pidPath, 30*time.Millisecond)
	if err == nil {
		t.Fatal("waitForHelperPIDWithTimeout() error = nil, want malformed PID diagnostic")
	}
	if !strings.Contains(err.Error(), `last content = "not-a-pid"`) || !strings.Contains(err.Error(), "invalid syntax") {
		t.Fatalf("waitForHelperPIDWithTimeout() error = %v, want last malformed content and parse error", err)
	}
}

func waitForHelperPID(t *testing.T, path string) int {
	t.Helper()
	pid, err := waitForHelperPIDWithTimeout(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func waitForHelperPIDWithTimeout(path string, timeout time.Duration) (int, error) {
	return waitForHelperPIDWithReadFile(path, timeout, os.ReadFile)
}

func waitForHelperPIDWithReadFile(path string, timeout time.Duration, readFile func(string) ([]byte, error)) (int, error) {
	deadline := time.Now().Add(timeout)
	var lastContent []byte
	var lastErr error
	for {
		raw, err := readFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(string(raw))
			if parseErr == nil && pid > 0 {
				return pid, nil
			}
			lastContent = raw
			if parseErr != nil {
				lastErr = parseErr
			} else {
				lastErr = fmt.Errorf("pid must be positive, got %d", pid)
			}
		} else {
			lastErr = err
		}
		if !time.Now().Before(deadline) {
			return 0, fmt.Errorf("wait for descendant pid: last content = %q, last error = %w", lastContent, lastErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func terminateTestProcess(pid int) {
	process, err := os.FindProcess(pid)
	if err == nil {
		_ = process.Kill()
	}
}

func TestStdioHandshake_Healthy(t *testing.T) {
	setStdioHelperProcess(t, "healthy")

	if err := stdioHandshake(context.Background(), stdioProbeTimeout, "engram", "mcp", "--tools=agent"); err != nil {
		t.Fatalf("stdioHandshake() error = %v, want nil for a healthy MCP server", err)
	}
}

func TestStdioHandshake_RPCError(t *testing.T) {
	setStdioHelperProcess(t, "rpc-error")

	err := stdioHandshake(context.Background(), stdioProbeTimeout, "engram", "mcp", "--tools=agent")
	if err == nil || !strings.Contains(err.Error(), "initialize returned error") {
		t.Fatalf("stdioHandshake() error = %v, want initialize error", err)
	}
}

func TestStdioHandshake_RejectsInvalidInitializeResult(t *testing.T) {
	tests := []struct {
		name string
		mode string
	}{
		{name: "null", mode: "null-result"},
		{name: "non-object", mode: "scalar-result"},
		{name: "missing protocol version", mode: "missing-protocol-version"},
		{name: "invalid capabilities", mode: "invalid-capabilities"},
		{name: "missing server info", mode: "missing-server-info"},
		{name: "invalid server info", mode: "invalid-server-info"},
		{name: "missing result", mode: "missing-result"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setStdioHelperProcess(t, tt.mode)

			err := stdioHandshake(context.Background(), stdioProbeTimeout, "engram", "mcp", "--tools=agent")
			if err == nil || !strings.Contains(err.Error(), "initialize returned invalid result") {
				t.Fatalf("stdioHandshake() error = %v, want invalid initialize result", err)
			}
		})
	}
}

func TestStdioHandshake_EarlyExit(t *testing.T) {
	setStdioHelperProcess(t, "early-exit")

	err := stdioHandshake(context.Background(), stdioProbeTimeout, "engram", "mcp", "--tools=agent")
	if err == nil || !strings.Contains(err.Error(), "exited without answering initialize") {
		t.Fatalf("stdioHandshake() error = %v, want early-exit error", err)
	}
}

func TestStdioHandshake_GarbageOutput(t *testing.T) {
	setStdioHelperProcess(t, "garbage")

	err := stdioHandshake(context.Background(), stdioProbeTimeout, "engram", "mcp", "--tools=agent")
	if err == nil || !strings.Contains(err.Error(), "invalid MCP stdout") {
		t.Fatalf("stdioHandshake() error = %v, want invalid stdout error", err)
	}
}

func TestStdioHandshake_RejectsInvalidResponseEnvelope(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want string
	}{
		{name: "missing jsonrpc", mode: "missing-jsonrpc", want: "jsonrpc must be \"2.0\""},
		{name: "wrong jsonrpc", mode: "wrong-jsonrpc", want: "jsonrpc must be \"2.0\""},
		{name: "null jsonrpc", mode: "null-jsonrpc", want: "jsonrpc must be \"2.0\""},
		{name: "request-shaped result", mode: "request-shaped-result", want: "must not include method"},
		{name: "malformed result", mode: "malformed-result", want: "invalid MCP stdout"},
		{name: "stray stdout before healthy response", mode: "stray-stdout-then-healthy", want: "invalid MCP stdout"},
		{name: "mismatched id before healthy response", mode: "mismatched-id-then-healthy", want: "unexpected response id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setStdioHelperProcess(t, tt.mode)

			err := stdioHandshake(context.Background(), stdioProbeTimeout, "engram", "mcp", "--tools=agent")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("stdioHandshake() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestStdioHandshake_TerminatesThenDrainsBufferedStdout(t *testing.T) {
	setStdioHelperProcess(t, "trailing-stdout-after-healthy")

	err := stdioHandshake(context.Background(), stdioProbeTimeout, "engram", "mcp", "--tools=agent")
	if err == nil || !strings.Contains(err.Error(), "invalid MCP stdout") {
		t.Fatalf("stdioHandshake() error = %v, want buffered trailing stdout error", err)
	}
}

func TestStdioHandshake_PreservesEarlierDeadline(t *testing.T) {
	setStdioHelperProcess(t, "silent")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := stdioHandshake(ctx, stdioProbeTimeout, "engram", "mcp", "--tools=agent")
	if err != context.DeadlineExceeded {
		t.Fatalf("stdioHandshake() error = %v, want earlier caller deadline", err)
	}
}

func TestStdioHandshake_PreservesCallerCancellation(t *testing.T) {
	setStdioHelperProcess(t, "silent")

	cause := errors.New("caller stopped the engram probe")
	ctx, cancel := context.WithCancelCause(context.Background())
	timer := time.AfterFunc(300*time.Millisecond, func() { cancel(cause) })
	defer timer.Stop()
	defer cancel(nil)

	err := stdioHandshake(ctx, stdioProbeTimeout, "engram", "mcp", "--tools=agent")
	if err != cause {
		t.Fatalf("stdioHandshake() error = %v, want caller cancellation", err)
	}
}

func TestStdioHandshake_TerminalOutcomes(t *testing.T) {
	tests := []struct {
		name            string
		mode            string
		newContext      func() (context.Context, context.CancelFunc)
		cancelAfter     time.Duration
		probeTimeout    time.Duration
		want            error
		wantContains    string
		wantLiveContext bool
	}{
		{
			name: "caller cancellation",
			mode: "silent",
			newContext: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.Background())
			},
			cancelAfter: 50 * time.Millisecond,
			want:        context.Canceled,
		},
		{
			name: "earlier caller deadline",
			mode: "silent",
			newContext: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 50*time.Millisecond)
			},
			want: context.DeadlineExceeded,
		},
		{
			name:         "internal timeout",
			mode:         "silent",
			newContext:   func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
			probeTimeout: 50 * time.Millisecond,
			want:         context.DeadlineExceeded,
		},
		{
			name:            "genuine child exit while context is live",
			mode:            "early-exit",
			newContext:      func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
			wantContains:    "exited without answering initialize",
			wantLiveContext: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setStdioHelperProcess(t, tt.mode)
			if tt.probeTimeout != 0 {
				originalTimeout := stdioProbeTimeout
				stdioProbeTimeout = tt.probeTimeout
				t.Cleanup(func() { stdioProbeTimeout = originalTimeout })
			}

			ctx, cancel := tt.newContext()
			defer cancel()
			if tt.cancelAfter != 0 {
				timer := time.AfterFunc(tt.cancelAfter, cancel)
				defer timer.Stop()
			}
			err := stdioHandshake(ctx, stdioProbeTimeout, "engram", "mcp", "--tools=agent")
			if tt.want != nil && err != tt.want {
				t.Fatalf("stdioHandshake() error = %v, want %v", err, tt.want)
			}
			if tt.wantContains != "" && (err == nil || !strings.Contains(err.Error(), tt.wantContains)) {
				t.Fatalf("stdioHandshake() error = %v, want %q", err, tt.wantContains)
			}
			if tt.wantLiveContext && ctx.Err() != nil {
				t.Fatalf("caller context = %v, want live while child exits", ctx.Err())
			}
		})
	}
}

func TestStdioHandshake_MissingBinary(t *testing.T) {
	if err := stdioHandshake(context.Background(), stdioProbeTimeout, "gentle-ai-test-no-such-binary"); err == nil {
		t.Fatal("stdioHandshake() expected error for a missing binary")
	}
}

func TestProbeStdio_NotInstalled(t *testing.T) {
	orig := stdioHandshakeFn
	stdioHandshakeFn = func(context.Context, time.Duration, string, ...string) error { return exec.ErrNotFound }
	t.Cleanup(func() { stdioHandshakeFn = orig })

	if err := ProbeStdio(context.Background(), 0, "engram", "mcp", "--tools=agent"); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("ProbeStdio() error = %v, want ErrNotInstalled", err)
	}
}

func TestProbeStdio_MissingAbsoluteConfiguredCommandFails(t *testing.T) {
	orig := stdioHandshakeFn
	stdioHandshakeFn = func(context.Context, time.Duration, string, ...string) error { return exec.ErrNotFound }
	t.Cleanup(func() { stdioHandshakeFn = orig })

	err := ProbeStdio(context.Background(), 0, "/configured/engram", "mcp", "--tools=agent")
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("ProbeStdio() error = %v, want missing configured command", err)
	}
	if errors.Is(err, ErrNotInstalled) {
		t.Fatalf("ProbeStdio() error = %v, must not hide a missing absolute configured command", err)
	}
}

func TestProbeStdio_ClassifiesCommandNotFound(t *testing.T) {
	orig := stdioHandshakeFn
	stdioHandshakeFn = func(context.Context, time.Duration, string, ...string) error { return exec.ErrNotFound }
	t.Cleanup(func() { stdioHandshakeFn = orig })

	tests := []struct {
		name             string
		command          string
		wantNotInstalled bool
	}{
		{name: "bare Engram name", command: "engram", wantNotInstalled: true},
		{name: "Windows bare executable", command: "engram.exe", wantNotInstalled: runtime.GOOS == "windows"},
		{name: "Windows case-insensitive bare executable", command: "EnGrAm.ExE", wantNotInstalled: runtime.GOOS == "windows"},
		{name: "relative path", command: `./engram`},
		{name: "nested relative path", command: `bin/engram`},
		{name: "Windows drive path", command: `C:\Engram\engram.exe`},
		{name: "Windows drive-relative path", command: `C:Engram\engram.exe`},
		{name: "Windows UNC path", command: `\\server\share\engram.exe`},
		{name: "custom command", command: "custom-engram"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ProbeStdio(context.Background(), 0, tt.command, "mcp", "--tools=agent")
			if tt.wantNotInstalled {
				if !errors.Is(err, ErrNotInstalled) {
					t.Fatalf("ProbeStdio() error = %v, want ErrNotInstalled", err)
				}
				return
			}
			if !errors.Is(err, exec.ErrNotFound) {
				t.Fatalf("ProbeStdio() error = %v, want configured command error", err)
			}
			if errors.Is(err, ErrNotInstalled) {
				t.Fatalf("ProbeStdio() error = %v, must preserve configured command error", err)
			}
		})
	}
}

func TestProbeStdio_UsesExactPersistedCommand(t *testing.T) {
	var gotName string
	var gotArgs []string
	orig := stdioHandshakeFn
	stdioHandshakeFn = func(_ context.Context, _ time.Duration, name string, args ...string) error {
		gotName = name
		gotArgs = args
		return nil
	}
	t.Cleanup(func() { stdioHandshakeFn = orig })

	if err := ProbeStdio(context.Background(), 0, "/opt/gentle-engram/bin/engram", "mcp", "--tools=custom"); err != nil {
		t.Fatalf("ProbeStdio() error = %v", err)
	}
	if gotName != "/opt/gentle-engram/bin/engram" {
		t.Errorf("probe command = %q, want persisted command", gotName)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "mcp" || gotArgs[1] != "--tools=custom" {
		t.Errorf("probe args = %v, want persisted arguments", gotArgs)
	}
}

func TestReadTOMLEngramCommand(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantCommand string
		wantArgs    []string
		wantErr     bool
	}{
		{
			name: "inline comments",
			raw: `# Codex MCP configuration
[mcp_servers.engram] # persisted Engram transport
command = "/configured/codex-engram" # do not resolve through PATH
args = ["mcp", "--tools=codex"] # exact persisted arguments
`,
			wantCommand: "/configured/codex-engram",
			wantArgs:    []string{"mcp", "--tools=codex"},
		},
		{
			name: "literal strings",
			raw: `[mcp_servers.engram]
command = '/configured/codex-engram'
args = ['mcp', '--tools=codex']
`,
			wantCommand: "/configured/codex-engram",
			wantArgs:    []string{"mcp", "--tools=codex"},
		},
		{
			name: "escaped basic strings",
			raw: `[mcp_servers.engram]
command = "C:\\Program Files\\Engram\\engram.exe"
args = [
  "mcp",
  "--label=\"codex\"", # preserve the escaped quote
]
`,
			wantCommand: `C:\Program Files\Engram\engram.exe`,
			wantArgs:    []string{"mcp", `--label="codex"`},
		},
		{
			name: "malformed TOML fails closed",
			raw: `[mcp_servers.engram]
command = "engram"
args = ["mcp", "--tools=codex"]
unclosed = [
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found, err := readTOMLEngramCommand(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("readTOMLEngramCommand() error = nil, want malformed TOML rejection")
				}
				return
			}
			if err != nil {
				t.Fatalf("readTOMLEngramCommand() error = %v", err)
			}
			if !found {
				t.Fatal("readTOMLEngramCommand() found = false, want persisted Engram command")
			}
			if got.Command != tt.wantCommand {
				t.Errorf("command = %q, want %q", got.Command, tt.wantCommand)
			}
			if strings.Join(got.Args, "\x00") != strings.Join(tt.wantArgs, "\x00") {
				t.Errorf("args = %q, want %q", got.Args, tt.wantArgs)
			}
		})
	}
}

func TestReadPersistedStdioCommands_PreservesConfiguredCommandAndArguments(t *testing.T) {
	homeDir := t.TempDir()
	cases := []struct {
		name    string
		agentID string
		path    string
		content string
		command string
		args    []string
	}{
		{
			name:    "Claude user configuration",
			agentID: "claude-code",
			path:    ".claude.json",
			content: `{"mcpServers":{"engram":{"command":"/configured/claude-engram","args":["mcp","--tools=claude"]}}}`,
			command: "/configured/claude-engram",
			args:    []string{"mcp", "--tools=claude"},
		},
		{
			name:    "OpenCode command array",
			agentID: "opencode",
			path:    ".config/opencode/opencode.json",
			content: `{"mcp":{"engram":{"command":["/configured/opencode-engram","mcp","--tools=opencode"],"type":"local"}}}`,
			command: "/configured/opencode-engram",
			args:    []string{"mcp", "--tools=opencode"},
		},
		{
			name:    "Codex TOML configuration",
			agentID: "codex",
			path:    ".codex/config.toml",
			content: "[mcp_servers.engram]\ncommand = \"/configured/codex-engram\"\nargs = [\"mcp\", \"--tools=codex\"]\n",
			command: "/configured/codex-engram",
			args:    []string{"mcp", "--tools=codex"},
		},
		{
			name:    "Hermes YAML configuration",
			agentID: "hermes",
			path:    ".hermes/config.yaml",
			content: "mcp_servers:\n  engram:\n    command: /configured/hermes-engram\n    args:\n      - mcp\n      - --tools=hermes\n",
			command: "/configured/hermes-engram",
			args:    []string{"mcp", "--tools=hermes"},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(homeDir, tt.path)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}

			commands, err := ReadPersistedStdioCommands(homeDir, []string{tt.agentID})
			if err != nil {
				t.Fatalf("ReadPersistedStdioCommands() error = %v", err)
			}
			if len(commands) != 1 {
				t.Fatalf("commands = %v, want one configured command", commands)
			}
			if commands[0].Command != tt.command {
				t.Errorf("command = %q, want %q", commands[0].Command, tt.command)
			}
			if strings.Join(commands[0].Args, "\x00") != strings.Join(tt.args, "\x00") {
				t.Errorf("args = %v, want %v", commands[0].Args, tt.args)
			}
			if commands[0].Source != path {
				t.Errorf("source = %q, want %q", commands[0].Source, path)
			}
		})
	}
}
