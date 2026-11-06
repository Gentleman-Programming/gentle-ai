//go:build linux

package testenv_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestIsolateProtectsOpenCodeWithMalformedInheritedEnv(t *testing.T) {
	const launchEnv = "GENTLE_AI_TESTENV_RAW_ENV_LAUNCH"
	if os.Getenv(launchEnv) == "1" {
		// os/exec normalizes environments. execve preserves this non-empty
		// entry without '=', as a Linux process can receive at startup.
		env := append(os.Environ(), "GENTLE_AI_TESTENV_MALFORMED_ENTRY")
		args := []string{os.Args[0], "-test.run=^TestIsolateProtectsExternalOpenCodeConfig$", "-test.v"}
		if err := syscall.Exec(os.Args[0], args, env); err != nil {
			t.Fatalf("raw environment exec: %v", err)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestIsolateProtectsOpenCodeWithMalformedInheritedEnv$")
	cmd.Env = append(os.Environ(), launchEnv+"=1")
	out, err := cmd.CombinedOutput()
	t.Logf("raw-environment OpenCode regression:\n%s", out)
	if err != nil {
		t.Fatalf("raw-environment subprocess failed: %v (context: %v)", err, ctx.Err())
	}
	if !strings.Contains(string(out), "--- PASS: TestIsolateProtectsExternalOpenCodeConfig (") {
		t.Fatal("raw-environment subprocess did not confirm the OpenCode regression passed")
	}
}
