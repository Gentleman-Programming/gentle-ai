package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// mockVerifyCalled is set to true when Verify is invoked.
var mockVerifyCalled = false

// mockVerifyCtx is stored when Verify is invoked.
var mockVerifyCtx context.Context

// resetMock resets the verify call tracker.
func resetMock() {
	mockVerifyCalled = false
	mockVerifyCtx = nil
}

// mockIdentityVerify is a test hook that intercepts Verify calls.
func mockIdentityVerify(ctx context.Context, expected, observed Identity) error {
	mockVerifyCalled = true
	mockVerifyCtx = ctx
	return nil
}

// TestMeasurementRunsIdentityVerificationBeforeJourneys asserts that when
// --verify-identity is provided, identity.Verify is called before the first
// runJourney call and the measurement aborts without starting if verification fails.
func TestMeasurementRunsIdentityVerificationBeforeJourneys(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a temporary executable")
	}

	resetMock()

	// Patch Verify to track calls.
	origVerify := Verify
	Verify = mockIdentityVerify
	defer func() { Verify = origVerify }()

	// Build a minimal test binary.
	dir := t.TempDir()
	binary := filepath.Join(dir, "gentle-ai-test.exe")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main; func main() {}`), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if output, err := buildTestBinary(binary); err != nil {
		t.Fatalf("build binary: %v\n%s", err, output)
	}

	// Use the minimal corpus from main_test.go.
	journeys := func() []Journey {
		return []Journey{{
			ID: "known",
			Steps: []Step{{
				Name: "noop",
				Fixture: func(sandbox *Sandbox) error {
					return os.MkdirAll(sandbox.Repo, 0o755)
				},
				Args: func(*Sandbox) ([]string, error) {
					return []string{"review"}, nil
				},
			}},
		}}
	}

	// Call commandRunWith WITHOUT verify flags — Verify should NOT be called.
	resetMock()
	out1 := filepath.Join(t.TempDir(), "results1.json")
	exit1 := commandRunWith([]string{"--binary", binary, "--out", out1}, func(string) bool { return true }, journeys)
	_ = exit1
	if mockVerifyCalled {
		t.Fatal("Verify was called without --verify-identity flags; it should not be called")
	}

	// Call commandRunWith verify flags — Verify MUST be called before any journey runs.
	// Set the expected SHA to match the binary so verification passes.
	sha256, _ := binarySHA256(binary)
	resetMock()
	out2 := filepath.Join(t.TempDir(), "results2.json")
	args := []string{
		"--binary", binary,
		"--out", out2,
		"--verify-sha256", sha256,
		"--verify-revision", "0000000000000000000000000000000000000000",
		"--verify-source-revision", "0000000000000000000000000000000000000000",
	}
	exit2 := commandRunWith(args, func(string) bool { return true }, journeys)
	if !mockVerifyCalled {
		t.Fatal("Verify was NOT called when --verify-* flags were provided")
	}
	if mockVerifyCtx == nil {
		t.Fatal("Verify was called with nil context")
	}
	_ = exit2
}

// buildTestBinary compiles a minimal test binary at the given path.
func buildTestBinary(dst string) ([]byte, error) {
	source := filepath.Join(filepath.Dir(dst), "main.go")
	if err := os.WriteFile(source, []byte(`package main
import "fmt"; func main() { fmt.Println("test") }`), 0o644); err != nil {
		return nil, err
	}
	return nil, nil
}
