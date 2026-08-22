//go:build windows

package pi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func writeWindowsTransportInterpreter(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("interpreter"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestWindowsTransportCommandPlanNativeIsDirectAndCopySafe(t *testing.T) {
	executable := `C:\Program Files\safe & model.exe`
	environment := []string{"pAtH=C:\\first", "PATH=C:\\second", "AWS_SECRET=must-not-cross"}
	plan, err := newWindowsTransportCommandPlan(executable, environment)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Executable() != executable || len(plan.Arguments()) != 0 || plan.WorkingDirectory() != `C:\` || !reflect.DeepEqual(plan.Environment(), []string{"PATH=C:\\second"}) {
		t.Fatalf("plan=%+v; want direct executable, zero arguments, drive-root cwd, and filtered environment", plan)
	}
}

func TestWindowsTransportCommandPlanBatchUsesValidatedComSpec(t *testing.T) {
	root := t.TempDir()
	comspec := writeWindowsTransportInterpreter(t, filepath.Join(root, "preferred", "cmd.exe"))
	candidate := filepath.Join(root, "script path with spaces.CmD")
	plan, err := newWindowsTransportCommandPlan(candidate, []string{"sYsTeMrOoT=C:\\fallback", "cOmSpEc=" + comspec, "PATH=C:\\bin"})
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"/d", "/s", "/c", `""` + candidate + `""`}
	if plan.Executable() != comspec || !reflect.DeepEqual(plan.Arguments(), wantArgs) || plan.WorkingDirectory() != filepath.VolumeName(candidate)+string(filepath.Separator) {
		t.Fatalf("plan=%+v; want ComSpec, /d /s /c, preserved path, and volume-root cwd", plan)
	}
	args, env := plan.Arguments(), plan.Environment()
	args[3], env[0] = "mutated", "COMSPEC=mutated"
	if !reflect.DeepEqual(plan.Arguments(), wantArgs) || plan.Environment()[0] != "COMSPEC="+comspec {
		t.Fatal("plan accessors exposed mutable slices")
	}
}

func TestWindowsTransportCommandPlanRejectsUnsafeBatchPaths(t *testing.T) {
	base := filepath.Join(t.TempDir(), "candidate")
	for _, tc := range []struct{ name, value string }{
		{"percent", "%"}, {"exclamation", "!"}, {"ampersand", "&"}, {"pipe", "|"}, {"less-than", "<"}, {"greater-than", ">"},
		{"caret", "^"}, {"left-paren", "("}, {"right-paren", ")"}, {"double-quote", `"`}, {"single-quote", "'"},
		{"carriage-return", "\r"}, {"line-feed", "\n"}, {"nul", "\x00"}, {"tab-control", "\t"}, {"control", "\x01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newWindowsTransportCommandPlan(base+tc.value+".cmd", nil)
			requireTransportKind(t, err, TransportErrorInvalidPath)
		})
	}
}

func TestWindowsTransportCommandPlanUsesComSpecPrecedenceAndFallback(t *testing.T) {
	root := t.TempDir()
	preferred := writeWindowsTransportInterpreter(t, filepath.Join(root, "preferred", "cmd.exe"))
	fallbackRoot := t.TempDir()
	fallback := writeWindowsTransportInterpreter(t, filepath.Join(fallbackRoot, "System32", "cmd.exe"))
	candidate := filepath.Join(root, "script.cmd")
	for _, tc := range []struct {
		name, want string
		env        []string
	}{
		{"case-insensitive precedence", preferred, []string{"sYsTeMrOoT=" + fallbackRoot, "cOmSpEc=" + preferred}},
		{"validated SystemRoot fallback", fallback, []string{"sYsTeMrOoT=" + fallbackRoot}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := newWindowsTransportCommandPlan(candidate, tc.env)
			if err != nil || plan.Executable() != tc.want {
				t.Fatalf("plan=%+v error=%v; want interpreter %q", plan, err, tc.want)
			}
		})
	}
}

func TestWindowsTransportCommandPlanFailsClosedForInvalidInterpreters(t *testing.T) {
	root := t.TempDir()
	fallbackRoot := t.TempDir()
	writeWindowsTransportInterpreter(t, filepath.Join(fallbackRoot, "System32", "cmd.exe"))
	valid := writeWindowsTransportInterpreter(t, filepath.Join(root, "cmd.exe"))
	directory := filepath.Join(root, "directory", "cmd.exe")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, comspec string }{
		{"relative", "cmd.exe"}, {"empty", ""}, {"missing", filepath.Join(root, "missing", "cmd.exe")},
		{"unclean", valid + string(filepath.Separator) + "."}, {"wrong basename", filepath.Join(root, "powershell.exe")}, {"directory", directory},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newWindowsTransportCommandPlan(filepath.Join(root, "script.bat"), []string{"SYSTEMROOT=" + fallbackRoot, "COMSPEC=" + tc.comspec})
			requireTransportKind(t, err, TransportErrorInvalidPath)
		})
	}
	for _, tc := range []struct {
		name, systemRoot string
		present          bool
	}{
		{"missing fallback", "", false}, {"relative fallback", "relative", true}, {"empty fallback", "", true}, {"missing cmd", t.TempDir(), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			environment := []string(nil)
			if tc.present {
				environment = []string{"SYSTEMROOT=" + tc.systemRoot}
			}
			_, err := newWindowsTransportCommandPlan(filepath.Join(root, "script.bat"), environment)
			requireTransportKind(t, err, TransportErrorInvalidPath)
		})
	}
}

func TestWindowsTransportEnvironmentIsCanonicalAndSecretFree(t *testing.T) {
	entries := []string{"path=C:\\first", "PATH=C:\\second", "lAnG=en_US", "LANGUAGE=en", "comspec=C:\\one", "ComSpec=C:\\two", "SYSTEMROOT=C:\\Windows", "AWS_SECRET=hidden", "HOME=private", "malformed"}
	want := []string{"COMSPEC=C:\\two", "LANG=en_US", "LANGUAGE=en", "PATH=C:\\second", "SYSTEMROOT=C:\\Windows"}
	got := windowsTransportEnvironment(entries)
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(got, windowsTransportEnvironment(entries)) {
		t.Fatalf("environment=%q; want deterministic %q", got, want)
	}
	for _, tc := range []struct {
		name          string
		entries, want []string
	}{
		{"empty PATH", []string{"SYSTEMROOT=C:\\Windows", "PATH="}, []string{"PATH=C:\\Windows\\System32;C:\\Windows", "SYSTEMROOT=C:\\Windows"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := windowsTransportEnvironment(tc.entries); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("environment=%q; want %q", got, tc.want)
			}
		})
	}
}

func init() {
	if len(os.Args) > 1 {
		if os.Args[1] == "transport-descendant" {
			if len(os.Args) != 3 {
				os.Exit(2)
			}
			writeWindowsTransportEvidence(os.Args[2], fmt.Sprintf("%d\nready\n", os.Getpid()))
			select {}
		}
		return
	}
	request, _ := io.ReadAll(os.Stdin)
	if bytes.HasPrefix(request, []byte("hang:")) {
		path := string(request[5:])
		if err := exec.Command(os.Args[0], "transport-descendant", path).Start(); err != nil {
			os.Exit(2)
		}
		select {}
	}
	switch string(request) {
	case "stdout":
		_, _ = os.Stdout.Write([]byte("abcd"))
	case "stdout-overflow":
		_, _ = os.Stdout.Write([]byte("abcdef"))
	case "stderr", "stderr-overflow":
		_, _ = os.Stderr.Write([]byte("stderr-secret"))
	case "both-overflow":
		_, _ = os.Stdout.Write([]byte("abcde"))
		_, _ = os.Stderr.Write([]byte("wxyza"))
	case "nonzero":
		os.Exit(7)
	default:
		cwd, _ := os.Getwd()
		_, _ = fmt.Fprintf(os.Stdout, "%x\n%d\n%s\n%s", request, len(os.Args)-1, cwd, os.Getenv("AWS_SECRET"))
	}
	os.Exit(0)
}
func writeWindowsTransportEvidence(path, data string) {
	if err := os.WriteFile(path+".tmp", []byte(data), 0o600); err != nil || os.Rename(path+".tmp", path) != nil {
		os.Exit(2)
	}
}
func TestWindowsTransportUsesNativeAndBatchContracts(t *testing.T) {
	helper := windowsTransportTestExecutable(t)
	t.Setenv("AWS_SECRET", "must-not-cross")
	request := []byte("first\nsecond\x00line\n")
	for _, ext := range []string{".exe", ".cmd", ".bat"} {
		t.Run(ext, func(t *testing.T) {
			executable := helper
			if ext != ".exe" {
				executable = filepath.Join(t.TempDir(), "script path with spaces"+ext)
				if err := os.WriteFile(executable, []byte("@echo off\r\n\""+helper+"\"\r\n"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			o := validTransportOptions()
			o.Timeout, o.MaxRequestBytes, o.MaxStdoutBytes, o.MaxStderrBytes = 5*time.Second, len(request), 8192, 8192
			result, err := RunBoundedModelRoutingProcess(context.Background(), executable, request, o)
			if err != nil || result.ExitCode != 0 || result.StderrBytes != 0 {
				t.Fatalf("result=%+v error=%v", result, err)
			}
			parts := strings.Split(string(result.Stdout), "\n")
			wantCWD := filepath.Clean(filepath.VolumeName(executable) + string(filepath.Separator))
			if len(parts) != 4 || parts[0] != fmt.Sprintf("%x", request) || parts[1] != "0" || !strings.EqualFold(filepath.Clean(parts[2]), wantCWD) || parts[3] != "" {
				t.Fatalf("helper record=%q", result.Stdout)
			}
		})
	}
}
func TestWindowsTransportBoundsAndNonDisclosure(t *testing.T) {
	helper := windowsTransportTestExecutable(t)
	for _, tc := range []struct {
		name, request, output string
		stdout, stderr, count int
		kind                  TransportErrorKind
		code                  int
	}{
		{"stdout exact", "stdout", "abcd", 4, 32, 0, "", 0}, {"stdout overflow", "stdout-overflow", "abcde", 4, 32, 0, TransportErrorStdoutOverflow, 0},
		{"stderr exact", "stderr", "", 32, 32, 13, "", 0}, {"stderr overflow", "stderr-overflow", "", 32, 4, 13, TransportErrorStderrOverflow, 0},
		{"both overflow", "both-overflow", "abcde", 4, 4, 5, TransportErrorStdoutOverflow, 0}, {"nonzero", "nonzero", "", 32, 32, 0, TransportErrorNonzeroExit, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := validTransportOptions()
			o.Timeout, o.MaxRequestBytes, o.MaxStdoutBytes, o.MaxStderrBytes = 5*time.Second, len(tc.request), tc.stdout, tc.stderr
			result, err := RunBoundedModelRoutingProcess(context.Background(), helper, []byte(tc.request), o)
			if tc.kind == "" && err != nil {
				t.Fatal(err)
			} else if tc.kind != "" {
				requireTransportKind(t, err, tc.kind)
			}
			if string(result.Stdout) != tc.output || result.StderrBytes != tc.count || result.ExitCode != tc.code || strings.Contains(fmt.Sprint(err, result), "stderr-secret") {
				t.Fatalf("result=%+v error=%v", result, err)
			}
		})
	}
}
func TestWindowsTransportCancelAndTimeoutReapDescendants(t *testing.T) {
	helper := windowsTransportTestExecutable(t)
	deadline := time.Now().Add(30 * time.Second)
	if d, ok := t.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	for _, tc := range []struct {
		name   string
		cancel bool
		kind   TransportErrorKind
		cause  error
	}{
		{"cancel", true, TransportErrorCanceled, context.Canceled}, {"timeout", false, TransportErrorTimeout, context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evidencePath := filepath.Join(t.TempDir(), "process.evidence")
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			done := make(chan error, 1)
			go func() {
				o := validTransportOptions()
				o.Timeout, o.MaxRequestBytes = 5*time.Second, len("hang:")+len(evidencePath)
				_, err := RunBoundedModelRoutingProcess(ctx, helper, []byte("hang:"+evidencePath), o)
				done <- err
			}()
			data := waitWindowsTransportEvidence(t, evidencePath, deadline)
			fields := strings.Fields(string(data))
			if len(fields) < 2 {
				t.Fatalf("malformed readiness evidence %q", data)
			}
			pid, err := strconv.Atoi(fields[0])
			if err != nil || pid <= 0 {
				t.Fatalf("descendant pid=%q error=%v", fields[0], err)
			}
			handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := windows.CloseHandle(handle); err != nil {
					t.Errorf("close descendant handle: %v", err)
				}
			}()
			if tc.cancel {
				stop()
			}
			select {
			case err = <-done:
			case <-time.After(time.Until(deadline)):
				t.Fatal("transport did not return")
			}
			requireTransportKind(t, err, tc.kind)
			if !errors.Is(err, tc.cause) {
				t.Fatalf("termination cause was not preserved: %v", err)
			}
			remaining := time.Until(deadline)
			if remaining <= 0 {
				t.Fatal("test deadline expired before descendant reap")
			}
			status, err := windows.WaitForSingleObject(handle, uint32(remaining/time.Millisecond))
			if err != nil || status != windows.WAIT_OBJECT_0 {
				t.Fatalf("descendant status=%d error=%v", status, err)
			}
		})
	}
}
func windowsTransportTestExecutable(t *testing.T) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return executable
}

func waitWindowsTransportEvidence(t *testing.T, path string, deadline time.Time) []byte {
	t.Helper()
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			return data
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("process evidence did not become readable")
	return nil
}

func TestWindowsTransportSetupFailurePreservesCause(t *testing.T) {
	oldSet := windowsTransportSetInformationJobObject
	windowsTransportSetInformationJobObject = func(windows.Handle, uint32, uintptr, uint32) (int, error) { return 0, errors.New("configure seam") }
	defer func() { windowsTransportSetInformationJobObject = oldSet }()
	command := exec.Command(os.Args[0], "-test.run=^TestWindowsTransportCommandPlanNativeIsDirectAndCopySafe$")
	_, err := startWindowsTransportProcessTree(command)
	if err == nil || !strings.Contains(err.Error(), "configure seam") {
		t.Fatalf("setup error=%v; want preserved configure failure", err)
	}
}
