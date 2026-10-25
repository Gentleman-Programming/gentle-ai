//go:build linux || darwin

package shellinstaller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func userLaunchTestPID(t *testing.T, path string) int {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if data, err := os.ReadFile(path); err == nil && strings.HasSuffix(string(data), "\n") {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil || pid <= 1 {
				t.Fatalf("invalid pid in %s: %q", path, data)
			}
			return pid
		}
	}
	t.Fatalf("launched group never wrote %s", path)
	return 0
}

func TestUnixLaunchGroupReapsBeforeReadback(t *testing.T) {
	dir := t.TempDir()
	// userLaunchGroup sets Cancel, which requires a context-bound command.
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", `(sleep 1; : > "$1/survived") & exit 7`, "sh", dir)
	stdin, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	defer writer.Close()
	cmd.Stdin = stdin
	sentinel := errors.New("readback refused")
	readbackRan := false
	err = userLaunchGroup(cmd, stdin, func(waitErr error) error {
		readbackRan = true
		var exit *exec.ExitError
		if !errors.As(waitErr, &exit) || exit.ExitCode() != 7 {
			t.Errorf("readback wait error = %v, want exit 7", waitErr)
		}
		if probe := syscall.Kill(-cmd.Process.Pid, 0); !errors.Is(probe, syscall.ESRCH) {
			t.Errorf("readback ran before the group was reaped: %v", probe)
		}
		return errors.Join(waitErr, sentinel)
	})
	if !readbackRan || !errors.Is(err, sentinel) {
		t.Fatalf("launch = %v, readback ran %v; want the readback result", err, readbackRan)
	}
	<-time.After(1500 * time.Millisecond)
	if _, err := os.Lstat(filepath.Join(dir, "survived")); !os.IsNotExist(err) {
		t.Fatalf("backgrounded descendant ran after launch returned: %v", err)
	}
}

func TestUnixLaunchGroupCancelKillsGroup(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", `(sleep 1; : > "$1/survived") & echo $$ > "$1/pid"; sleep 30`, "sh", dir)
	done := make(chan error, 1)
	go func() { done <- userLaunchGroup(cmd, nil, func(err error) error { return err }) }()
	pid := userLaunchTestPID(t, filepath.Join(dir, "pid"))
	cancel()
	select {
	case err := <-done:
		// The kill is the expected outcome; only a failed reap is uncertain.
		var private *PrivateRuntimeError
		var exit *exec.ExitError
		if errors.As(err, &private) || !errors.As(err, &exit) {
			t.Fatalf("canceled launch = %v, want the killed child status after a clean reap", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("launch ignored cancellation")
	}
	if err := syscall.Kill(-pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("group after cancel = %v, want ESRCH", err)
	}
	<-time.After(1500 * time.Millisecond)
	if _, err := os.Lstat(filepath.Join(dir, "survived")); !os.IsNotExist(err) {
		t.Fatalf("backgrounded descendant ran after cancel: %v", err)
	}
}

// TestUnixLaunchGroupTerminalFixture runs as a session leader whose
// controlling terminal is a fresh pty, then launches through userLaunchGroup.
func TestUnixLaunchGroupTerminalFixture(t *testing.T) {
	dir := os.Getenv("GENTLE_SHELL_LAUNCH_FIXTURE")
	if dir == "" {
		return
	}
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", `ps -o tpgid= -o pgid= -p $$ > "$1/child"`, "sh", dir)
	cmd.Stdin = os.Stdin
	err := userLaunchGroup(cmd, os.Stdin, func(err error) error { return err })
	group, groupErr := unix.IoctlGetInt(0, unix.TIOCGPGRP)
	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	result := fmt.Sprintf("launch=%v restored=%v group=%d self=%d pid=%d\n", err, groupErr == nil && group == syscall.Getpgrp(), group, syscall.Getpgrp(), pid)
	if writeErr := os.WriteFile(filepath.Join(dir, "result"), []byte(result), 0600); writeErr != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestUnixLaunchGroupHandsOffAndRestoresForeground(t *testing.T) {
	terminal := openTestTerminal(t)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	fixture := exec.Command(self, "-test.run=^TestUnixLaunchGroupTerminalFixture$")
	fixture.Env = []string{"GENTLE_SHELL_LAUNCH_FIXTURE=" + dir, "PATH=/usr/bin:/bin"}
	fixture.Stdin = terminal
	fixture.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	output, err := fixture.CombinedOutput()
	if err != nil {
		t.Fatalf("terminal fixture = %v: %s", err, output)
	}
	result, err := os.ReadFile(filepath.Join(dir, "result"))
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(result))
	if len(fields) != 5 || fields[0] != "launch=<nil>" || fields[1] != "restored=true" {
		t.Fatalf("fixture result = %q", result)
	}
	child, err := os.ReadFile(filepath.Join(dir, "child"))
	if err != nil {
		t.Fatal(err)
	}
	ids := strings.Fields(string(child))
	pid := strings.TrimPrefix(fields[4], "pid=")
	// The launched command led its own group and owned the terminal foreground.
	if len(ids) != 2 || ids[0] != pid || ids[1] != pid || "self="+pid == fields[3] {
		t.Fatalf("child tpgid/pgid = %q, launched pid %s; want both equal to the launched pid", child, pid)
	}
}
