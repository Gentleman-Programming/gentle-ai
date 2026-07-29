package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/engram"
)

// ---------------------------------------------------------------------------
// Task 2.4 GREEN evidence: InjectOptions.Version threading from
// VerifyVersion() (internal/cli/run.go) into the Claude Code slim/full
// section selection.
// ---------------------------------------------------------------------------

func TestRunInstallThreadsEngramVersionIntoClaudeSlimSelection(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	restoreVerifyVersion := verifyEngramVersion
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		verifyEngramVersion = restoreVerifyVersion
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}
	runCommand = func(string, ...string) error { return nil }
	verifyEngramVersion = func() (string, error) { return "engram 1.18.0", nil }

	result, err := RunInstall(
		[]string{"--agent", "claude-code", "--component", "engram"},
		macOSDetectionResult(),
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("verification ready = false")
	}

	claudeMD, err := os.ReadFile(filepath.Join(home, ".claude", "CLAUDE.md"))
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}
	text := string(claudeMD)
	if strings.Contains(text, "needs_review") {
		t.Fatalf("above-floor engram version must render the SLIM section (must not contain full-only 'needs_review' text); got:\n%s", text)
	}
	if !strings.Contains(text, "SessionStart hook") {
		t.Fatalf("above-floor engram version must render the SLIM section with its pointer to the full protocol location; got:\n%s", text)
	}
}

func TestRunInstallBelowFloorVersionKeepsClaudeFullSelection(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	restoreVerifyVersion := verifyEngramVersion
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		verifyEngramVersion = restoreVerifyVersion
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}
	runCommand = func(string, ...string) error { return nil }
	verifyEngramVersion = func() (string, error) { return "engram 1.3.9", nil }

	result, err := RunInstall(
		[]string{"--agent", "claude-code", "--component", "engram"},
		macOSDetectionResult(),
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("verification ready = false")
	}

	claudeMD, err := os.ReadFile(filepath.Join(home, ".claude", "CLAUDE.md"))
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}
	if !strings.Contains(string(claudeMD), "needs_review") {
		t.Fatalf("below-floor engram version must keep the FULL section; got:\n%s", claudeMD)
	}
}

// ---------------------------------------------------------------------------
// Task 2.6/2.7 GREEN evidence: ProbeProtocolFlag wiring, per-slug forwarding
// with safest-wins semantics, and safe degradation when the probe fails or
// the flag is unsupported.
// ---------------------------------------------------------------------------

func TestRunInstallForwardsProtocolSlimForClaudeCodeWhenSupported(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	restoreVerifyVersion := verifyEngramVersion
	restoreProbe := probeEngramProtocolFlag
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		verifyEngramVersion = restoreVerifyVersion
		probeEngramProtocolFlag = restoreProbe
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}
	verifyEngramVersion = func() (string, error) { return "engram 1.18.0", nil }
	probeEngramProtocolFlag = func(context.Context) (string, error) {
		return "Usage: engram setup <slug> [--protocol=slim|full]\n", nil
	}

	recorder := &commandRecorder{}
	runCommand = recorder.record

	result, err := RunInstall(
		[]string{"--agent", "claude-code", "--component", "engram"},
		macOSDetectionResult(),
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("verification ready = false")
	}

	found := false
	for _, cmd := range recorder.get() {
		if cmd == "engram setup claude-code --protocol=slim" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'engram setup claude-code --protocol=slim', got commands: %v", recorder.get())
	}
}

// TestRunInstallSafestWinsAcrossSharedSlug asserts the Per-slug forwarding
// semantics from design.md: Gemini CLI and Antigravity share the gemini-cli
// slug, and neither ever verifies slim, so the forwarded flag for that slug
// MUST be --protocol=full even when the probe reports support.
func TestRunInstallSafestWinsAcrossSharedSlug(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	restoreVerifyVersion := verifyEngramVersion
	restoreProbe := probeEngramProtocolFlag
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		verifyEngramVersion = restoreVerifyVersion
		probeEngramProtocolFlag = restoreProbe
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}
	verifyEngramVersion = func() (string, error) { return "engram 1.18.0", nil }
	probeEngramProtocolFlag = func(context.Context) (string, error) {
		return "Usage: engram setup <slug> [--protocol=slim|full]\n", nil
	}

	recorder := &commandRecorder{}
	runCommand = func(name string, args ...string) error {
		if err := recorder.record(name, args...); err != nil {
			return err
		}
		if name == "engram" && len(args) >= 2 && args[0] == "setup" && args[1] == "gemini-cli" {
			settingsPath := filepath.Join(home, ".gemini", "settings.json")
			if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
				return err
			}
			return os.WriteFile(settingsPath, []byte("{\"theme\":\"dark\"}\n"), 0o644)
		}
		return nil
	}

	result, err := RunInstall(
		[]string{"--agent", "gemini-cli", "--agent", "antigravity", "--component", "engram", "--component", "context7", "--component", "permissions"},
		macOSDetectionResult(),
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("verification ready = false")
	}

	setupCount := 0
	foundFull := false
	for _, cmd := range recorder.get() {
		if strings.HasPrefix(cmd, "engram setup gemini-cli") {
			setupCount++
			if cmd == "engram setup gemini-cli --protocol=full" {
				foundFull = true
			}
		}
	}
	if setupCount != 1 {
		t.Fatalf("engram setup gemini-cli count = %d, want 1 (deduped per-slug)", setupCount)
	}
	if !foundFull {
		t.Fatalf("expected 'engram setup gemini-cli --protocol=full' (safest-wins, neither adapter is slim), got commands: %v", recorder.get())
	}
}

func TestRunInstallOmitsProtocolFlagWhenProbeFails(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	restoreVerifyVersion := verifyEngramVersion
	restoreProbe := probeEngramProtocolFlag
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		verifyEngramVersion = restoreVerifyVersion
		probeEngramProtocolFlag = restoreProbe
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}
	verifyEngramVersion = func() (string, error) { return "engram 1.18.0", nil }
	probeEngramProtocolFlag = func(context.Context) (string, error) {
		return "", errors.New("engram setup --help: context deadline exceeded")
	}

	recorder := &commandRecorder{}
	runCommand = recorder.record

	result, err := RunInstall(
		[]string{"--agent", "claude-code", "--component", "engram"},
		macOSDetectionResult(),
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("verification ready = false")
	}

	for _, cmd := range recorder.get() {
		if strings.Contains(cmd, "--protocol") {
			t.Fatalf("probe failure MUST omit --protocol entirely (today's behavior), got command: %s", cmd)
		}
	}
	found := false
	for _, cmd := range recorder.get() {
		if cmd == "engram setup claude-code" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unchanged 'engram setup claude-code' invocation, got commands: %v", recorder.get())
	}
}

// TestRunInstallSkipsProtocolProbeWhenSetupModeOff pins JD-013: under
// GENTLE_AI_ENGRAM_SETUP_MODE=off no adapter will ever attempt `engram
// setup` (engram.ShouldAttemptSetup returns false for every agent), so the
// --protocol probe (up to a 5s deadline in production) must not run either
// — its result would never be used. verifyEngramVersion stays unconditional
// (it still feeds InjectOptions.Version for section rendering), only the
// probe is gated.
func TestRunInstallSkipsProtocolProbeWhenSetupModeOff(t *testing.T) {
	t.Setenv(engram.SetupModeEnvVar, "off")

	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	restoreVerifyVersion := verifyEngramVersion
	restoreProbe := probeEngramProtocolFlag
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		verifyEngramVersion = restoreVerifyVersion
		probeEngramProtocolFlag = restoreProbe
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}
	runCommand = func(string, ...string) error { return nil }
	verifyEngramVersion = func() (string, error) { return "engram 1.18.0", nil }

	probeCalls := 0
	probeEngramProtocolFlag = func(context.Context) (string, error) {
		probeCalls++
		return "Usage: engram setup <slug> [--protocol=slim|full]\n", nil
	}

	result, err := RunInstall(
		[]string{"--agent", "claude-code", "--component", "engram"},
		macOSDetectionResult(),
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("verification ready = false")
	}

	if probeCalls != 0 {
		t.Fatalf("probeEngramProtocolFlag call count = %d, want 0 under GENTLE_AI_ENGRAM_SETUP_MODE=off", probeCalls)
	}
}

// TestRunInstallShellsOutEngramVersionOnlyOnce pins JD-016: componentApplyStep.Run
// resolves the installed engram version once (Decision 1 gate) and the
// post-apply health check (engramHealthChecks) must reuse that resolved
// value instead of shelling out to `engram version` a second time.
//
// verifyEngramVersion is restored to the real engram.VerifyVersion (rather
// than TestMain's hermetic error fake) so the count reflects the actual
// underlying `engram version` command seam (engram.CountVersionCallsForTest),
// not just the cli-level wrapper var.
func TestRunInstallShellsOutEngramVersionOnlyOnce(t *testing.T) {
	home := t.TempDir()
	restoreHome := osUserHomeDir
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	restoreVerifyVersion := verifyEngramVersion
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		verifyEngramVersion = restoreVerifyVersion
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	cmdLookPath = func(name string) (string, error) {
		return "/usr/local/bin/" + name, nil
	}
	runCommand = func(string, ...string) error { return nil }
	verifyEngramVersion = engram.VerifyVersion

	callCount := engram.CountVersionCallsForTest(t, "engram 1.18.0")

	result, err := RunInstall(
		[]string{"--agent", "claude-code", "--component", "engram"},
		macOSDetectionResult(),
	)
	if err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("verification ready = false")
	}

	if *callCount != 1 {
		t.Fatalf("underlying `engram version` invocation count = %d, want 1 (spawned once per run)", *callCount)
	}
}
