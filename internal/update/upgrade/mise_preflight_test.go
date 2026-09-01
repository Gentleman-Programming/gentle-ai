package upgrade

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
	"github.com/gentleman-programming/gentle-ai/v2/internal/update"
)

// wantMiseManagedHint is the exact ManualHint text preflightMiseGentleAIUpgrades
// must produce for a mise-managed gentle-ai candidate. Asserted verbatim
// (design.md — Stream 2b), not via strings.Contains.
const wantMiseManagedHint = "mise-managed install — run `mise upgrade gentle-ai` instead; replacing the binary in place would desync the version mise tracks"

// setupMiseManaged configures the package-level mise detection vars so that
// runningBinaryIsMiseManaged() reports true for the duration of t, reusing
// the same env/var-swap helpers as mise_ownership_test.go.
func setupMiseManaged(t *testing.T) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "installs")
	exe := filepath.Join(root, "go", "1.25.10", "bin", "gentle-ai")
	mustWriteExecutable(t, exe)

	clearMisePrecedenceEnv(t)
	setMiseEnv(t, "MISE_INSTALLS_DIR", root)
	swapMiseCurrentExecutableFn(t, exe, nil)
}

// setupNonMiseManaged configures the package-level mise detection vars so
// that runningBinaryIsMiseManaged() reports false for the duration of t —
// a resolvable mise root exists, but the running executable lives outside it.
func setupNonMiseManaged(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "installs")
	mustMkdirAll(t, root)
	exe := filepath.Join(base, "usr-local-bin", "gentle-ai")
	mustWriteExecutable(t, exe)

	clearMisePrecedenceEnv(t)
	setMiseEnv(t, "MISE_INSTALLS_DIR", root)
	swapMiseCurrentExecutableFn(t, exe, nil)
}

// TestExecute_MiseManagedGentleAIIsSkippedWithVerbatimHintAndNoBackup covers
// the mise skip's core contract: a mise-managed gentle-ai candidate is the
// only requested tool, so it is removed from the executable set entirely.
// This proves both the exact hint text and that BackupID stays empty when
// the mise skip empties the candidate set (no other tools requested).
func TestExecute_MiseManagedGentleAIIsSkippedWithVerbatimHintAndNoBackup(t *testing.T) {
	setupMiseManaged(t)

	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })
	execCommand = func(name string, args ...string) *exec.Cmd {
		t.Fatalf("execCommand must not run for a mise-skipped candidate: %s %v", name, args)
		return nil
	}

	results := []update.UpdateResult{
		makeResult("gentle-ai", update.UpdateAvailable, "1.0.0", "1.1.0", update.InstallBinary),
	}

	report := Execute(context.Background(), results, linuxProfile(), t.TempDir(), false)

	if len(report.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(report.Results))
	}
	got := report.Results[0]
	if got.ToolName != "gentle-ai" {
		t.Fatalf("ToolName = %q, want gentle-ai", got.ToolName)
	}
	if got.Status != UpgradeSkipped {
		t.Errorf("Status = %q, want UpgradeSkipped", got.Status)
	}
	if got.ManualHint != wantMiseManagedHint {
		t.Errorf("ManualHint = %q, want verbatim %q", got.ManualHint, wantMiseManagedHint)
	}
	if report.BackupID != "" {
		t.Errorf("BackupID = %q, want empty — the mise skip emptied the executable candidate set", report.BackupID)
	}
}

// TestExecute_MiseManagedGentleAIStillAllowsOtherToolToUpgrade verifies the
// per-tool skip semantics: a second, unrelated requested tool in the same
// Execute() invocation still upgrades normally even though gentle-ai is
// mise-managed in that same invocation.
func TestExecute_MiseManagedGentleAIStillAllowsOtherToolToUpgrade(t *testing.T) {
	setupMiseManaged(t)

	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })
	execCommand = func(name string, args ...string) *exec.Cmd {
		return mockCmd("echo", "ok")
	}

	results := []update.UpdateResult{
		makeResult("gentle-ai", update.UpdateAvailable, "1.0.0", "1.1.0", update.InstallBinary),
		makeResult("engram", update.UpdateAvailable, "0.3.0", "0.4.0", update.InstallGoInstall),
	}
	results[1].Tool.GoImportPath = "github.com/Gentleman-Programming/engram/cmd/engram"

	report := Execute(context.Background(), results, linuxProfile(), t.TempDir(), false)

	if len(report.Results) != 2 {
		t.Fatalf("len(Results) = %d, want 2", len(report.Results))
	}

	var gentleAI, engram *ToolUpgradeResult
	for i := range report.Results {
		switch report.Results[i].ToolName {
		case "gentle-ai":
			gentleAI = &report.Results[i]
		case "engram":
			engram = &report.Results[i]
		}
	}
	if gentleAI == nil || engram == nil {
		t.Fatalf("expected both gentle-ai and engram in Results, got %#v", report.Results)
	}
	if gentleAI.Status != UpgradeSkipped || gentleAI.ManualHint != wantMiseManagedHint {
		t.Errorf("gentle-ai = %+v, want skipped with the verbatim mise hint", gentleAI)
	}
	if engram.Status != UpgradeSucceeded {
		t.Errorf("engram Status = %q, want UpgradeSucceeded — an unrelated tool must still upgrade", engram.Status)
	}
	if report.BackupID == "" {
		t.Errorf("BackupID is empty, want non-empty — engram is still executable and must be backed up first")
	}
}

// TestPreflightMiseGentleAIUpgrades_NonMiseInstallLeavesExecutableSetUnchanged
// proves a non-mise install (runningBinaryIsMiseManaged() false) is
// byte-identical to today's existing behavior: the preflight must return the
// input executable set completely unmutated and skip nothing, so nothing
// downstream of this new composition point can observe any difference.
func TestPreflightMiseGentleAIUpgrades_NonMiseInstallLeavesExecutableSetUnchanged(t *testing.T) {
	setupNonMiseManaged(t)

	executable := []executableUpdate{
		{result: makeResult("gentle-ai", update.UpdateAvailable, "1.0.0", "1.1.0", update.InstallBinary)},
		{result: makeResult("engram", update.UpdateAvailable, "0.3.0", "0.4.0", update.InstallGoInstall)},
	}

	remaining, skipped := preflightMiseGentleAIUpgrades(executable, linuxProfile())

	if len(skipped) != 0 {
		t.Fatalf("skipped = %#v, want none — a non-mise install must not be preflight-skipped", skipped)
	}
	if len(remaining) != len(executable) {
		t.Fatalf("len(remaining) = %d, want %d — a non-mise install must pass through unchanged", len(remaining), len(executable))
	}
	for i := range executable {
		if remaining[i].result.Tool.Name != executable[i].result.Tool.Name {
			t.Errorf("remaining[%d].Tool.Name = %q, want %q — candidate order/identity must be preserved", i, remaining[i].result.Tool.Name, executable[i].result.Tool.Name)
		}
	}
}

// TestExecute_MiseAndWindowsPreflightsComposeWithoutDoubleSkippingSameCandidate
// verifies the composition order decided in design.md: mise runs first, so a
// candidate that is both mise-managed AND would otherwise trigger the Windows
// go-install preflight is removed from the executable set before the Windows
// preflight ever sees it — it must be skipped exactly once, carrying the mise
// hint, never a duplicate Windows-hint result.
func TestExecute_MiseAndWindowsPreflightsComposeWithoutDoubleSkippingSameCandidate(t *testing.T) {
	setupMiseManaged(t)

	origExecCommand := execCommand
	t.Cleanup(func() { execCommand = origExecCommand })
	execCommand = func(name string, args ...string) *exec.Cmd {
		t.Fatalf("execCommand must not run for a mise-skipped candidate: %s %v", name, args)
		return nil
	}

	result := makeResult("gentle-ai", update.UpdateAvailable, "1.0.0", "1.1.0", update.InstallBinary)
	result.Tool.GoImportPath = "github.com/gentleman-programming/gentle-ai/v2/cmd/gentle-ai"

	// Windows + GoAvailable + GoImportPath is exactly the shape that would
	// route gentle-ai through the Windows go-install preflight
	// (preflightWindowsGentleAIGoInstall) if mise had not already removed it
	// from the executable set first.
	profile := system.PlatformProfile{OS: "windows", PackageManager: "winget", Supported: true, GoAvailable: true}

	report := Execute(context.Background(), []update.UpdateResult{result}, profile, t.TempDir(), false)

	var gentleAIResults []ToolUpgradeResult
	for _, r := range report.Results {
		if r.ToolName == "gentle-ai" {
			gentleAIResults = append(gentleAIResults, r)
		}
	}
	if len(gentleAIResults) != 1 {
		t.Fatalf("gentle-ai appeared %d times in Results, want exactly 1 — mise and Windows preflights must not double-skip the same candidate: %#v", len(gentleAIResults), gentleAIResults)
	}
	if gentleAIResults[0].Status != UpgradeSkipped || gentleAIResults[0].ManualHint != wantMiseManagedHint {
		t.Errorf("gentle-ai result = %+v, want a single skip carrying the mise hint (mise runs first)", gentleAIResults[0])
	}
	if report.BackupID != "" {
		t.Errorf("BackupID = %q, want empty — the mise skip emptied the executable candidate set", report.BackupID)
	}
}
