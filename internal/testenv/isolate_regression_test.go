package testenv_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v3/internal/testenv"
)

// isolateSentinelSubprocessEnv triggers the sentinel-check stand-in below.
// It plays the same role as internal/cli/protocol_probe_test.go's own
// GENTLE_AI_TEST_CLI_STANDIN re-exec guard.
const isolateSentinelSubprocessEnv = "GENTLE_AI_TESTENV_ISOLATE_SUBPROCESS"

// isolateSentinelHomeEnv carries the isolated home the subprocess resolves
// Pi's agent config path against, once PI_CODING_AGENT_DIR is neutralized.
const isolateSentinelHomeEnv = "GENTLE_AI_TESTENV_ISOLATE_HOME"

// TestMain re-execs into runIsolateSentinelSubprocess when the parent test
// below spawns this same test binary with PI_CODING_AGENT_DIR already set to
// a sentinel directory — mirroring exactly how Gentle Shell exports it
// before `go test` ever starts, i.e. before any package TestMain runs.
func TestMain(m *testing.M) {
	if os.Getenv(isolateSentinelSubprocessEnv) == "1" {
		runIsolateSentinelSubprocess()
	}
	os.Exit(m.Run())
}

// runIsolateSentinelSubprocess simulates a package TestMain that calls
// testenv.Isolate() before m.Run(): PI_CODING_AGENT_DIR is already set to a
// sentinel directory by the parent process before this function ever runs.
// It prints the resolved Pi agent config path and exits 0 only when that
// path stays inside the isolated home instead of the sentinel.
func runIsolateSentinelSubprocess() {
	testenv.Isolate()
	home := os.Getenv(isolateSentinelHomeEnv)
	got := pi.AgentConfigPath(home)
	want := filepath.Join(home, ".pi", "agent")
	if got != want {
		fmt.Fprintf(os.Stderr, "AgentConfigPath(%q) = %q, want %q\n", home, got, want)
		os.Exit(1)
	}
	fmt.Print(got)
	os.Exit(0)
}

// TestIsolateNeutralizesSentinelSetBeforeTestMain is the regression test for
// the hermetic-Pi-tests bug: a PI_CODING_AGENT_DIR pointing at a sentinel
// directory is set BEFORE the (subprocess's) package TestMain runs, exactly
// as Gentle Shell exports it into every child process including `go test`.
// A package TestMain that calls testenv.Isolate() must neutralize it before
// any Pi-path test resolves a config path, so the sentinel directory is
// never touched.
func TestIsolateNeutralizesSentinelSetBeforeTestMain(t *testing.T) {
	sentinel := t.TempDir()
	home := t.TempDir()

	cmd := exec.Command(os.Args[0], "-test.run=^TestIsolateNeutralizesSentinelSetBeforeTestMain$")
	cmd.Env = append(os.Environ(),
		isolateSentinelSubprocessEnv+"=1",
		"PI_CODING_AGENT_DIR="+sentinel,
		isolateSentinelHomeEnv+"="+home,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sentinel subprocess failed: %v\noutput: %s", err, out)
	}
	if strings.Contains(string(out), sentinel) {
		t.Fatalf("sentinel subprocess resolved a path under the sentinel dir %q: %s", sentinel, out)
	}
}
