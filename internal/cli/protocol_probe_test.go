package cli

import (
	"context"
	"errors"
	"os"
	"testing"
)

// TestMain overrides verifyEngramVersion and probeEngramProtocolFlag with
// hermetic fakes for the whole internal/cli test binary, so pre-existing
// tests never depend on a real installed engram binary being present (or
// absent) on the machine running `go test`. Individual tests that need to
// exercise the new Decision 1 version-gate / Decision 4 probe behavior
// override these vars locally (save/restore), the same pattern already used
// for cmdLookPath elsewhere in this package.
//
// Defaults intentionally mirror "engram not verifiable": empty version
// (falls back to the full protocol section, matching pre-change behavior)
// and an unsupported --protocol flag (omit it, matching pre-change setup
// invocations exactly — see the exact-argv assertions in
// run_integration_test.go).
func TestMain(m *testing.M) {
	if code, ok := reviewGitProcessHelperExitCode(); ok {
		os.Exit(code)
	}
	if err := os.Unsetenv("GENTLE_AI_CHANNEL"); err != nil {
		panic(err)
	}

	verifyEngramVersion = func() (string, error) {
		return "", errors.New("engram version not available in tests")
	}
	probeEngramProtocolFlag = func(context.Context) (string, error) {
		return "", errors.New("engram setup --help not available in tests")
	}

	os.Exit(m.Run())
}
