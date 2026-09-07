package telemetry

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// fakeRecorderExecutable writes a tiny shell script (or, on Windows, a
// batch file) that records the argv it was invoked with and the bytes on
// its stdin, then exits 0. It is a real executable and buildSendCommand
// really runs it as a real subprocess, but it never touches the network —
// this is the "fake executable" seam the spawner review asked for.
func fakeRecorderExecutable(t *testing.T) (path, argvFile, stdinFile string) {
	t.Helper()
	dir := t.TempDir()
	argvFile = filepath.Join(dir, "argv.txt")
	stdinFile = filepath.Join(dir, "stdin.bin")
	if runtime.GOOS == "windows" {
		path = filepath.Join(dir, "recorder.cmd")
		script := "@echo off\r\n" +
			"echo %* > \"" + argvFile + "\"\r\n" +
			"more > \"" + stdinFile + "\"\r\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		return path, argvFile, stdinFile
	}
	path = filepath.Join(dir, "recorder.sh")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" > '" + argvFile + "'\n" +
		"cat > '" + stdinFile + "'\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path, argvFile, stdinFile
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
	if got := string(argv); got != "telemetry send\n" && got != "telemetry send\r\n" {
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
	// is already there.
	deadline := time.Now().Add(5 * time.Second)
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
