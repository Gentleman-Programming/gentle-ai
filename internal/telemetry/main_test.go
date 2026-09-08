package telemetry

import (
	"io"
	"os"
	"strings"
	"testing"
)

// The test binary doubles as the fake sender executable: when these
// variables are set, it records its argv and stdin to the named files and
// exits instead of running tests. A real Go executable works the same on
// every platform, unlike a .cmd fixture, which cmd.exe cannot run as a
// detached process and which rewrites its output with CRLF.
const (
	recorderArgvEnv  = "GENTLE_AI_TELEMETRY_TEST_RECORDER_ARGV"
	recorderStdinEnv = "GENTLE_AI_TELEMETRY_TEST_RECORDER_STDIN"
)

func TestMain(m *testing.M) {
	argvFile := os.Getenv(recorderArgvEnv)
	stdinFile := os.Getenv(recorderStdinEnv)
	if argvFile == "" || stdinFile == "" {
		os.Exit(m.Run())
	}
	if err := os.WriteFile(argvFile, []byte(strings.Join(os.Args[1:], " ")), 0o600); err != nil {
		os.Exit(2)
	}
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(3)
	}
	if err := os.WriteFile(stdinFile, stdin, 0o600); err != nil {
		os.Exit(4)
	}
	os.Exit(0)
}
