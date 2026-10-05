package telemetry

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// fakeRecorderExecutable points the spawner at this test binary in recorder
// mode (see TestMain): a real executable that records the argv it was
// invoked with and the bytes on its stdin, then exits 0. buildSendCommand
// really runs it as a real subprocess, but it never touches the network.
func fakeRecorderExecutable(t *testing.T) (path, argvFile, stdinFile string) {
	t.Helper()
	dir := t.TempDir()
	argvFile = filepath.Join(dir, "argv.txt")
	stdinFile = filepath.Join(dir, "stdin.bin")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(recorderArgvEnv, argvFile)
	t.Setenv(recorderStdinEnv, stdinFile)
	return self, argvFile, stdinFile
}

func TestBuildSendCommandInvokesSelfWithTelemetrySendAndPipesPayloadOnStdin(t *testing.T) {
	fake, argvFile, stdinFile := fakeRecorderExecutable(t)
	orig := osExecutable
	osExecutable = func() (string, error) { return fake, nil }
	t.Cleanup(func() { osExecutable = orig })

	payload := []byte(`{"schema":"gentle-ai.telemetry-event/v1","event":"install"}`)
	cmd, stdinRead, err := buildSendCommand(payload)
	if err != nil {
		t.Fatal(err)
	}
	defer stdinRead.Close()
	// Run synchronously (unlike SpawnDetachedSend's fire-and-forget Start),
	// so the recorded argv/stdin files are guaranteed to exist once this
	// returns. No network access happens anywhere in this test.
	if err := cmd.Run(); err != nil {
		t.Fatalf("run recorder executable: %v", err)
	}

	argv, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(argv); got != "telemetry send" {
		t.Fatalf("recorded argv = %q, want \"telemetry send\"", got)
	}

	stdin, err := os.ReadFile(stdinFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(stdin) != string(payload) {
		t.Fatalf("recorded stdin = %q, want %q", stdin, payload)
	}
}

func TestSpawnDetachedSendDoesNotBlockAndReaches(t *testing.T) {
	fake, _, stdinFile := fakeRecorderExecutable(t)
	orig := osExecutable
	osExecutable = func() (string, error) { return fake, nil }
	t.Cleanup(func() { osExecutable = orig })

	payload := []byte(`{"schema":"gentle-ai.telemetry-event/v1","event":"heartbeat"}`)
	if err := DefaultSpawn(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	// SpawnDetachedSend does not wait for the child; poll briefly (bounded,
	// no unbounded sleep) for the file it produces rather than assuming it
	// is already there. Windows runners are slower to spawn a detached
	// process (console/job-object setup), so the deadline is more generous
	// than a bare Unix fork+exec needs.
	timeout := 5 * time.Second
	if runtime.GOOS == "windows" {
		timeout = 10 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		if data, err := os.ReadFile(stdinFile); err == nil && string(data) == string(payload) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("detached child never wrote the expected stdin file in time")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
