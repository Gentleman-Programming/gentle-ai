package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

func TestReviewModeStatusReportsBothSourcesWithoutMutating(t *testing.T) {
	home := reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	// This test pins that status leaves global user state alone, so it needs
	// state on disk to compare against. It writes its own rather than relying
	// on a shared fixture: nothing else in this flow creates one.
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{"opencode"}}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(state.Path(home))
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReviewMode([]string{"status", "--cwd", repo, "--json"}, &output); err != nil {
		t.Fatalf("RunReviewMode(status) error = %v", err)
	}
	result := decodeReviewModeResult(t, output.Bytes())
	if result.Schema != ReviewModeSchema || result.Operation != "status" {
		t.Fatalf("status result = %#v", result)
	}
	// Nobody opted in here, so both sources stay unset and the default
	// decides -- and the default is off, because receipt-driven development
	// is opt-in. Status still has to name both sources rather than collapsing
	// them into the one effective answer.
	if result.Status.Effective != reviewtransaction.RDDModeOff ||
		result.Status.Source != reviewtransaction.RDDModeSourceDefault ||
		result.Status.Global != reviewtransaction.RDDModeUnset ||
		result.Status.CloneLocal != reviewtransaction.RDDModeUnset {
		t.Fatalf("status did not report both sources and the effective mode: %#v", result.Status)
	}
	after, err := os.ReadFile(state.Path(home))
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("status changed global user state: err=%v before=%q after=%q", err, before, after)
	}
	if _, err := os.Lstat(filepath.Join(repo, ".git", "gentle-ai")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("status created repository state: %v", err)
	}
}

func TestReviewModeDisableGlobalWinsOverEveryRepository(t *testing.T) {
	home := reviewModeHome(t)
	repo := initReviewCLIRepo(t)

	var output bytes.Buffer
	if err := RunReviewMode([]string{"disable", "--cwd", repo, "--json"}, &output); err != nil {
		t.Fatalf("RunReviewMode(disable) error = %v", err)
	}
	if result := decodeReviewModeResult(t, output.Bytes()); result.Status.Effective != reviewtransaction.RDDModeOff ||
		result.Status.Source != reviewtransaction.RDDModeSourceGlobal {
		t.Fatalf("global disable result = %#v", result.Status)
	}
	persisted, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read error = %v", err)
	}
	if persisted.RDDMode != string(reviewtransaction.RDDModeOff) || persisted.RDDModeRecordedAt == nil {
		t.Fatalf("global disable was not persisted in user state: %#v", persisted)
	}

	output.Reset()
	if err := RunReviewMode([]string{"enable", "--cwd", repo, "--json"}, &output); err != nil {
		t.Fatalf("RunReviewMode(enable) error = %v", err)
	}
	if result := decodeReviewModeResult(t, output.Bytes()); result.Status.Effective != reviewtransaction.RDDModeOn {
		t.Fatalf("global enable result = %#v", result.Status)
	}
}

func TestReviewModeGlobalScopeWorksFromNonGitDirectory(t *testing.T) {
	home := reviewModeHome(t)
	nonGit := t.TempDir()

	var output bytes.Buffer
	if err := RunReviewMode([]string{"status", "--cwd", nonGit, "--json"}, &output); err != nil {
		t.Fatalf("unset global status from non-Git cwd error = %v\n%s", err, output.String())
	}
	if before := decodeReviewModeResult(t, output.Bytes()); before.Status.Effective != reviewtransaction.RDDModeOff ||
		before.Status.Source != reviewtransaction.RDDModeSourceDefault ||
		before.Status.Global != reviewtransaction.RDDModeUnset || before.Status.CloneLocal != reviewtransaction.RDDModeUnset {
		t.Fatalf("unset global status from non-Git cwd = %#v", before.Status)
	}

	output.Reset()
	if err := RunReviewMode([]string{"enable", "--cwd", nonGit, "--scope", "global", "--json"}, &output); err != nil {
		t.Fatalf("global enable from non-Git cwd error = %v\n%s", err, output.String())
	}
	result := decodeReviewModeResult(t, output.Bytes())
	if result.Operation != "enable" || result.Scope != reviewModeScopeGlobal ||
		result.Status.Effective != reviewtransaction.RDDModeOn ||
		result.Status.Source != reviewtransaction.RDDModeSourceGlobal ||
		result.Status.Global != reviewtransaction.RDDModeOn ||
		result.Status.CloneLocal != reviewtransaction.RDDModeUnset {
		t.Fatalf("global enable from non-Git cwd = %#v", result)
	}
	persisted, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read error = %v", err)
	}
	if persisted.RDDMode != string(reviewtransaction.RDDModeOn) || persisted.RDDModeRecordedAt == nil {
		t.Fatalf("global enable did not persist an explicit on: %#v", persisted)
	}
	if entries, err := os.ReadDir(nonGit); err != nil || len(entries) != 0 {
		t.Fatalf("global enable touched non-Git cwd: entries=%v err=%v", entries, err)
	}

	output.Reset()
	if err := RunReviewMode([]string{"status", "--cwd", nonGit, "--json"}, &output); err != nil {
		t.Fatalf("global status from non-Git cwd error = %v\n%s", err, output.String())
	}
	status := decodeReviewModeResult(t, output.Bytes())
	if status.Operation != "status" || status.Scope != reviewModeScopeBoth ||
		status.Status.Effective != reviewtransaction.RDDModeOn ||
		status.Status.Source != reviewtransaction.RDDModeSourceGlobal ||
		status.Status.Global != reviewtransaction.RDDModeOn ||
		status.Status.CloneLocal != reviewtransaction.RDDModeUnset {
		t.Fatalf("global status from non-Git cwd = %#v", status)
	}
}

func TestReviewModeCloneScopeOutsideGitFailsBeforeWriting(t *testing.T) {
	home := reviewModeHome(t)
	nonGit := t.TempDir()

	var output bytes.Buffer
	err := RunReviewMode([]string{"disable", "--cwd", nonGit, "--scope", "clone", "--json"}, &output)
	if err == nil || !strings.Contains(err.Error(), "clone-local review mode requires a Git repository") ||
		!strings.Contains(err.Error(), "--cwd") || !strings.Contains(err.Error(), "--scope global") ||
		strings.Contains(err.Error(), "fatal:") || strings.Contains(err.Error(), "git rev-parse") || strings.Contains(err.Error(), "exit code 128") {
		t.Fatalf("clone disable outside Git error = %v", err)
	}
	if !reviewtransaction.ReviewRootResolutionReportsNoRepository(err) {
		t.Fatalf("clone disable outside Git lost its typed no-repository classification: %v", err)
	}
	if _, readErr := state.Read(home); !errors.Is(readErr, os.ErrNotExist) {
		t.Fatalf("clone disable outside Git mutated global state: %v", readErr)
	}
	if entries, readErr := os.ReadDir(nonGit); readErr != nil || len(entries) != 0 {
		t.Fatalf("clone disable outside Git touched cwd: entries=%v err=%v", entries, readErr)
	}
}

func TestReviewModeRepositoryRequiredRefusalDoesNotDependOnGitStderrLanguage(t *testing.T) {
	localized := &reviewtransaction.GitCommandError{Args: []string{"rev-parse", "--show-toplevel"}, ExitCode: 128, Output: "fatal: no es un repositorio Git"}
	refusal := reviewModeRepositoryRequiredRefusal(localized)
	if refusal == nil || !reviewtransaction.ReviewRootResolutionReportsNoRepository(refusal) {
		t.Fatalf("localized no-repository error was not classified: %v", refusal)
	}
	if strings.Contains(refusal.Error(), localized.Output) {
		t.Fatalf("localized Git stderr reached the operator refusal: %v", refusal)
	}
}

func TestWriteGlobalRDDModeSerializesWithInstallStateAndPreservesFreshFields(t *testing.T) {
	home := reviewModeHome(t)
	lock, err := reviewtransaction.AcquireAuthorityFileLock(mustInstallStateLockPath(t, home))
	if err != nil {
		t.Fatalf("acquire install state lock: %v", err)
	}
	released := false
	t.Cleanup(func() {
		if !released {
			_ = lock.Release()
		}
	})

	if err := state.Write(home, state.InstallState{InstalledAgents: []string{"opencode"}}); err != nil {
		t.Fatalf("write concurrent install state: %v", err)
	}
	if err := writeGlobalRDDMode("enable"); !errors.Is(err, reviewtransaction.ErrStoreLockContended) {
		t.Fatalf("writeGlobalRDDMode while install state lock was held error = %v, want lock contention", err)
	}
	persisted, err := state.Read(home)
	if err != nil {
		t.Fatalf("read state after contended global mode write: %v", err)
	}
	if persisted.RDDMode != "" || persisted.RDDModeRecordedAt != nil {
		t.Fatalf("contended global mode write mutated state: %#v", persisted)
	}

	if err := lock.Release(); err != nil {
		t.Fatalf("release install state lock: %v", err)
	}
	released = true
	if err := writeGlobalRDDMode("enable"); err != nil {
		t.Fatalf("writeGlobalRDDMode after lock release: %v", err)
	}

	persisted, err = state.Read(home)
	if err != nil {
		t.Fatalf("read state after global mode write: %v", err)
	}
	if len(persisted.InstalledAgents) != 1 || persisted.InstalledAgents[0] != "opencode" {
		t.Fatalf("global mode write lost fresh install state: %#v", persisted.InstalledAgents)
	}
	if persisted.RDDMode != string(reviewtransaction.RDDModeOn) || persisted.RDDModeRecordedAt == nil {
		t.Fatalf("global mode write did not persist enable: %#v", persisted)
	}
}

// TestReviewModeGlobalEnableSurvivesTheOptInDefault is the upgrade-safety
// property behind making receipt-driven development opt-in. A user who
// deliberately ran `review mode enable --scope global` before the flip must
// still be reviewed after it: the enable writes an explicit "on" into user
// state, and resolution reads that explicit opinion rather than falling through
// to the now-off default. A clone that never opted in stays off.
func TestReviewModeGlobalEnableSurvivesTheOptInDefault(t *testing.T) {
	home := reviewModeHome(t)
	repo := initReviewCLIRepo(t)

	var output bytes.Buffer
	if err := RunReviewMode([]string{"status", "--cwd", repo, "--json"}, &output); err != nil {
		t.Fatalf("RunReviewMode(status) error = %v", err)
	}
	if before := decodeReviewModeResult(t, output.Bytes()); before.Status.Effective != reviewtransaction.RDDModeOff ||
		before.Status.Source != reviewtransaction.RDDModeSourceDefault {
		t.Fatalf("a clone nobody opted in was not off by default: %#v", before.Status)
	}

	output.Reset()
	if err := RunReviewMode([]string{"enable", "--cwd", repo, "--scope", "global", "--json"}, &output); err != nil {
		t.Fatalf("RunReviewMode(enable global) error = %v", err)
	}
	if result := decodeReviewModeResult(t, output.Bytes()); result.Status.Effective != reviewtransaction.RDDModeOn ||
		result.Status.Source != reviewtransaction.RDDModeSourceGlobal ||
		result.Status.Global != reviewtransaction.RDDModeOn {
		t.Fatalf("global enable result = %#v", result.Status)
	}

	persisted, err := state.Read(home)
	if err != nil {
		t.Fatalf("state.Read error = %v", err)
	}
	if persisted.RDDMode != string(reviewtransaction.RDDModeOn) || persisted.RDDModeRecordedAt == nil {
		t.Fatalf("global enable did not persist an explicit on: %#v", persisted)
	}

	// The persisted opinion, not the process that wrote it, is what survives an
	// upgrade: a later status in a different clone reads the same explicit on.
	other := initReviewCLIRepo(t)
	output.Reset()
	if err := RunReviewMode([]string{"status", "--cwd", other, "--json"}, &output); err != nil {
		t.Fatalf("RunReviewMode(status other clone) error = %v", err)
	}
	if after := decodeReviewModeResult(t, output.Bytes()); after.Status.Effective != reviewtransaction.RDDModeOn ||
		after.Status.Source != reviewtransaction.RDDModeSourceGlobal {
		t.Fatalf("an explicitly enabled user lost reviews: %#v", after.Status)
	}
}

// TestReviewModeCloneScopeDisablesOnlyThisClone needs a user who opted in
// globally: the property under test is that a clone-local off does not travel
// to a second clone, and that is only observable when something other than the
// override would have said on. Against the opt-in default both clones would
// read off for the same reason and the test would prove nothing.
func TestReviewModeCloneScopeDisablesOnlyThisClone(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)

	var output bytes.Buffer
	if err := RunReviewMode([]string{"disable", "--cwd", repo, "--scope", "clone", "--json"}, &output); err != nil {
		t.Fatalf("RunReviewMode(disable clone) error = %v", err)
	}
	result := decodeReviewModeResult(t, output.Bytes())
	if result.Status.Effective != reviewtransaction.RDDModeOff ||
		result.Status.Source != reviewtransaction.RDDModeSourceCloneLocal ||
		result.Status.Revision == "" {
		t.Fatalf("clone disable result = %#v", result.Status)
	}

	clone := filepath.Join(t.TempDir(), "clone")
	runReviewCLIGit(t, repo, "clone", "-q", repo, clone)
	output.Reset()
	if err := RunReviewMode([]string{"status", "--cwd", clone, "--json"}, &output); err != nil {
		t.Fatalf("RunReviewMode(status clone) error = %v", err)
	}
	if cloned := decodeReviewModeResult(t, output.Bytes()); cloned.Status.Effective != reviewtransaction.RDDModeOn {
		t.Fatalf("second clone inherited the override: %#v", cloned.Status)
	}
}

func TestReviewModeCloneScopeEnableIsIdempotentWhenGlobalOn(t *testing.T) {
	home := reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	if err := state.Write(home, state.InstallState{RDDMode: string(reviewtransaction.RDDModeOn)}); err != nil {
		t.Fatalf("state.Write error = %v", err)
	}

	var output bytes.Buffer
	err := RunReviewMode([]string{"enable", "--cwd", repo, "--scope", "clone", "--expected-revision", "", "--json"}, &output)
	if err != nil {
		t.Fatalf("clearing an absent clone override must succeed while global mode is on: %v", err)
	}
	if result := decodeReviewModeResult(t, output.Bytes()); result.Status.Effective != reviewtransaction.RDDModeOn ||
		result.Status.Source != reviewtransaction.RDDModeSourceGlobal || result.Status.Revision != "" {
		t.Fatalf("clone enable result = %#v", result.Status)
	}
	if _, err := os.Lstat(filepath.Join(repo, ".git", "gentle-ai")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("idempotent clone enable created repository state: %v", err)
	}
}

// TestReviewModeCloneScopeEnableMigratesLegacyRevision seeds the clone-local
// override against an explicit global "on", so the fixture opts in the same
// way: clearing the override has to land back on that global opinion, and
// against the opt-in default it would land on off and hide the migration.
func TestReviewModeCloneScopeEnableMigratesLegacyRevision(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	ctx := context.Background()
	disabled, err := reviewtransaction.SetCloneLocalRDDMode(ctx, repo, reviewtransaction.RDDModeOff, "", reviewtransaction.RDDGlobalMode{Value: "on"})
	if err != nil {
		t.Fatalf("seed clone-local override: %v", err)
	}
	current, err := reviewtransaction.CloneLocalRDDModeRecordPath(ctx, repo)
	if err != nil {
		t.Fatalf("current record path: %v", err)
	}
	legacyBytes, err := os.ReadFile(current)
	if err != nil {
		t.Fatalf("read current record: %v", err)
	}
	legacyRoot := filepath.Join(repo, ".git", "gentle-ai", "review-transactions")
	// The seeding write publishes into both locations, because the switch is
	// machine state rather than build state (#3284). This fixture is the clone
	// that only ever had the pre-relocation one, so its mirror is dropped
	// before the relocated root takes that name.
	if err := os.RemoveAll(legacyRoot); err != nil {
		t.Fatalf("drop the mirrored fixture copy: %v", err)
	}
	if err := os.Rename(filepath.Join(repo, ".git", "gentle-ai", "review-mode"), legacyRoot); err != nil {
		t.Fatalf("relocate secure legacy fixture: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(repo, ".git", "gentle-ai", "review-mode")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy fixture left a separately created private directory: %v", err)
	}
	legacy := filepath.Join(legacyRoot, "rar-authority", "v1", "rdd-mode", filepath.Base(current))

	var output bytes.Buffer
	if err := RunReviewMode([]string{"status", "--cwd", repo, "--json"}, &output); err != nil {
		t.Fatalf("legacy status: %v", err)
	}
	status := decodeReviewModeResult(t, output.Bytes()).Status
	if status.Revision != disabled.Revision || status.CloneLocal != reviewtransaction.RDDModeOff {
		t.Fatalf("legacy CLI status = %#v", status)
	}
	output.Reset()
	if err := RunReviewMode([]string{"enable", "--cwd", repo, "--scope", "clone", "--expected-revision", status.Revision, "--json"}, &output); err != nil {
		t.Fatalf("legacy CLI enable: %v", err)
	}
	migrated := decodeReviewModeResult(t, output.Bytes()).Status
	if !migrated.Enabled() || migrated.Revision == "" || migrated.Revision == status.Revision {
		t.Fatalf("migrated CLI status = %#v", migrated)
	}
	if after, err := os.ReadFile(legacy); err != nil || !bytes.Equal(after, legacyBytes) {
		t.Fatalf("legacy CLI bytes changed: err=%v", err)
	}
}

func TestReviewModeCloneScopeEnableRejectsGlobalOffWithoutLocalOverride(t *testing.T) {
	home := reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	if err := state.Write(home, state.InstallState{RDDMode: string(reviewtransaction.RDDModeOff)}); err != nil {
		t.Fatalf("state.Write error = %v", err)
	}

	var output bytes.Buffer
	err := RunReviewMode([]string{"enable", "--cwd", repo, "--scope", "clone", "--json"}, &output)
	var disabled *reviewtransaction.RDDDisabledError
	if !errors.As(err, &disabled) || !errors.Is(err, reviewtransaction.ErrRDDDisabled) ||
		disabled.Source != reviewtransaction.RDDModeSourceGlobal {
		t.Fatalf("clone enable error = %v, want global typed disabled error", err)
	}
	if !strings.Contains(err.Error(), "gentle-ai review mode enable --scope=global") {
		t.Fatalf("clone enable error does not name the global continuation: %v", err)
	}
	if result := decodeReviewModeResult(t, output.Bytes()); result.Status.Effective != reviewtransaction.RDDModeOff ||
		result.Status.Source != reviewtransaction.RDDModeSourceGlobal || result.Status.Revision != "" {
		t.Fatalf("clone enable result = %#v", result.Status)
	}
	if _, err := os.Lstat(filepath.Join(repo, ".git", "gentle-ai")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected clone enable created repository state: %v", err)
	}
}

func TestReviewModeCloneScopeEnableRejectsLegacyInheritWhileGlobalOff(t *testing.T) {
	home := reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	global := reviewtransaction.RDDGlobalMode{Value: string(reviewtransaction.RDDModeOn)}
	disabled, err := reviewtransaction.SetCloneLocalRDDMode(context.Background(), repo, reviewtransaction.RDDModeOff, "", global)
	if err != nil {
		t.Fatalf("SetCloneLocalRDDMode(off) error = %v", err)
	}
	inherited, err := reviewtransaction.SetCloneLocalRDDMode(context.Background(), repo, reviewtransaction.RDDModeUnset, disabled.Revision, global)
	if err != nil {
		t.Fatalf("SetCloneLocalRDDMode(inherit) error = %v", err)
	}
	if err := state.Write(home, state.InstallState{RDDMode: string(reviewtransaction.RDDModeOff)}); err != nil {
		t.Fatalf("state.Write error = %v", err)
	}
	record, err := reviewtransaction.CloneLocalRDDModeRecordPath(context.Background(), repo)
	if err != nil {
		t.Fatalf("CloneLocalRDDModeRecordPath error = %v", err)
	}
	before, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("read inherited record: %v", err)
	}

	var output bytes.Buffer
	err = RunReviewMode([]string{"enable", "--cwd", repo, "--scope", "clone", "--json"}, &output)
	var blocked *reviewtransaction.RDDDisabledError
	if !errors.As(err, &blocked) || blocked.Source != reviewtransaction.RDDModeSourceGlobal {
		t.Fatalf("legacy inherit clone enable error = %v, want global typed disabled error", err)
	}
	recordAfter, err := reviewtransaction.CloneLocalRDDModeRecordPath(context.Background(), repo)
	if err != nil {
		t.Fatalf("CloneLocalRDDModeRecordPath after retry error = %v", err)
	}
	after, err := os.ReadFile(recordAfter)
	if err != nil {
		t.Fatalf("read inherited record after retry: %v", err)
	}
	if recordAfter != record || !bytes.Equal(after, before) {
		t.Fatalf("legacy inherit retry published a new generation")
	}
	if result := decodeReviewModeResult(t, output.Bytes()); result.Status.Revision != inherited.Revision ||
		result.Status.Source != reviewtransaction.RDDModeSourceGlobal {
		t.Fatalf("legacy inherit clone enable result = %#v", result.Status)
	}
}

func TestReviewModeCloneScopeEnableRejectsExplicitOffWhileGlobalOff(t *testing.T) {
	home := reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	disabled, err := reviewtransaction.SetCloneLocalRDDMode(
		context.Background(), repo, reviewtransaction.RDDModeOff, "", reviewtransaction.RDDGlobalMode{Value: string(reviewtransaction.RDDModeOn)})
	if err != nil {
		t.Fatalf("SetCloneLocalRDDMode(off) error = %v", err)
	}
	if err := state.Write(home, state.InstallState{RDDMode: string(reviewtransaction.RDDModeOff)}); err != nil {
		t.Fatalf("state.Write error = %v", err)
	}
	record, err := reviewtransaction.CloneLocalRDDModeRecordPath(context.Background(), repo)
	if err != nil {
		t.Fatalf("CloneLocalRDDModeRecordPath error = %v", err)
	}
	before, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("read explicit-off record: %v", err)
	}

	var output bytes.Buffer
	err = RunReviewMode([]string{"enable", "--cwd", repo, "--scope", "clone", "--json"}, &output)
	var blocked *reviewtransaction.RDDDisabledError
	if !errors.As(err, &blocked) || !errors.Is(err, reviewtransaction.ErrRDDDisabled) ||
		blocked.Source != reviewtransaction.RDDModeSourceGlobal {
		t.Fatalf("explicit-off clone enable error = %v, want global typed disabled error", err)
	}
	if !strings.Contains(err.Error(), "gentle-ai review mode enable --scope=global") {
		t.Fatalf("explicit-off clone enable error does not name the global continuation: %v", err)
	}
	result := decodeReviewModeResult(t, output.Bytes())
	if result.Status.Effective != reviewtransaction.RDDModeOff || result.Status.CloneLocal != reviewtransaction.RDDModeOff ||
		result.Status.Revision != disabled.Revision {
		t.Fatalf("explicit-off clone enable result = %#v", result.Status)
	}
	recordAfter, err := reviewtransaction.CloneLocalRDDModeRecordPath(context.Background(), repo)
	if err != nil {
		t.Fatalf("CloneLocalRDDModeRecordPath after rejected enable error = %v", err)
	}
	after, err := os.ReadFile(recordAfter)
	if err != nil {
		t.Fatalf("read explicit-off record after rejected enable: %v", err)
	}
	if recordAfter != record || !bytes.Equal(after, before) {
		t.Fatalf("explicit-off clone enable published a new generation")
	}
}

func TestReviewModeCloneScopeDisableIsIdempotent(t *testing.T) {
	home := reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	if err := state.Write(home, state.InstallState{RDDMode: string(reviewtransaction.RDDModeOn)}); err != nil {
		t.Fatalf("state.Write error = %v", err)
	}

	var output bytes.Buffer
	if err := RunReviewMode([]string{"disable", "--cwd", repo, "--scope", "clone", "--json"}, &output); err != nil {
		t.Fatalf("first clone disable: %v", err)
	}
	first := decodeReviewModeResult(t, output.Bytes()).Status
	if first.Global != reviewtransaction.RDDModeOn || first.CloneLocal != reviewtransaction.RDDModeOff ||
		first.Source != reviewtransaction.RDDModeSourceCloneLocal {
		t.Fatalf("seeded clone disable status = %#v", first)
	}
	record, err := reviewtransaction.CloneLocalRDDModeRecordPath(context.Background(), repo)
	if err != nil {
		t.Fatalf("CloneLocalRDDModeRecordPath error = %v", err)
	}
	before, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("read disabled record: %v", err)
	}

	output.Reset()
	if err := RunReviewMode([]string{"disable", "--cwd", repo, "--scope", "clone", "--json"}, &output); err != nil {
		t.Fatalf("repeated clone disable: %v", err)
	}
	second := decodeReviewModeResult(t, output.Bytes()).Status
	recordAfter, err := reviewtransaction.CloneLocalRDDModeRecordPath(context.Background(), repo)
	if err != nil {
		t.Fatalf("CloneLocalRDDModeRecordPath after retry error = %v", err)
	}
	after, err := os.ReadFile(recordAfter)
	if err != nil {
		t.Fatalf("read disabled record after retry: %v", err)
	}
	if second.Revision != first.Revision || recordAfter != record || !bytes.Equal(after, before) {
		t.Fatalf("repeated clone disable published a new generation: first=%#v second=%#v", first, second)
	}
}

func TestReviewModeReportsUnknownPersistedModeAsDisabled(t *testing.T) {
	home := reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	if err := state.Write(home, state.InstallState{RDDMode: "sometimes"}); err != nil {
		t.Fatalf("state.Write error = %v", err)
	}

	var output bytes.Buffer
	err := RunReviewMode([]string{"status", "--cwd", repo, "--json"}, &output)
	if !errors.Is(err, reviewtransaction.ErrRDDModeUnknown) {
		t.Fatalf("unknown persisted mode error = %v, want ErrRDDModeUnknown", err)
	}
	if !strings.Contains(output.String(), string(reviewtransaction.RDDModeOff)) {
		t.Fatalf("unknown persisted mode did not report a disabled projection:\n%s", output.String())
	}
}

func TestReviewModeRejectsUnknownSubcommandAndScope(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	var output bytes.Buffer
	if err := RunReviewMode([]string{"toggle", "--cwd", repo}, &output); err == nil ||
		!strings.Contains(err.Error(), "unknown review mode command") {
		t.Fatalf("unknown subcommand error = %v", err)
	}
	if err := RunReviewMode([]string{"disable", "--cwd", repo, "--scope", "team"}, &output); err == nil ||
		!strings.Contains(err.Error(), "unknown review mode scope") {
		t.Fatalf("unknown scope error = %v", err)
	}
}

// TestReviewStartIsRejectedWhileTheKillSwitchIsOff proves a disabled START
// reports its actual refusal without manufacturing persistent authority or a
// receipt merely to exercise the negative path.
func TestReviewStartIsRejectedWhileTheKillSwitchIsOff(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "docs/guide.md", "ordinary documentation\n", 0o644)

	var modeOutput bytes.Buffer
	if err := RunReviewMode([]string{"disable", "--cwd", repo, "--scope", "clone", "--json"}, &modeOutput); err != nil {
		t.Fatalf("disable clone-local review mode: %v", err)
	}

	var startOutput bytes.Buffer
	err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-kill-switch"}, &startOutput)
	var disabled *reviewtransaction.RDDDisabledError
	if !errors.As(err, &disabled) {
		t.Fatalf("disabled review start error = %v, want *RDDDisabledError", err)
	}
	if disabled.Operation != reviewtransaction.RDDOperationStart ||
		disabled.Source != reviewtransaction.RDDModeSourceCloneLocal {
		t.Fatalf("disabled review start = %#v", disabled)
	}
	if !errors.Is(err, reviewtransaction.ErrRDDDisabled) {
		t.Fatalf("disabled review start does not unwrap to ErrRDDDisabled: %v", err)
	}
	stores, err := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	if err != nil {
		t.Fatalf("discover stores after disabled START: %v", err)
	}
	if len(stores) != 0 {
		t.Fatalf("disabled START created persistent review authority: %#v", stores)
	}
}

func TestTierZeroReviewStartNeverAsksForConsent(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	console := stubReviewConsole(t, true, "1\n")
	writeReviewStartCandidate(t, repo, "docs/guide.md", "ordinary documentation\n", 0o644)

	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-tier-zero"}, &output); err != nil {
		t.Fatalf("tier 0 start: %v\n%s", err, output.String())
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	if started.RiskLevel != reviewtransaction.RiskLow || len(started.SelectedLenses) != 0 {
		t.Fatalf("tier 0 START = %#v", started)
	}
	if console.Len() != 0 {
		t.Fatalf("tier 0 emitted a consent prompt:\n%s", console.String())
	}
	if asked, err := reviewtransaction.RDDConsentAsked(context.Background(), repo); err != nil || asked {
		t.Fatalf("tier 0 consumed the one-time question: asked=%v err=%v", asked, err)
	}
}

func TestTierOneReviewStartAsksOnceForOneConsolidatedReview(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	console := stubReviewConsole(t, true, "1\n")
	writeReviewStartCandidate(t, repo, "internal/app.go", "package internal\n", 0o644)

	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-tier-one"}, &output); err != nil {
		t.Fatalf("tier 1 start: %v\n%s", err, output.String())
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	if started.RiskLevel != reviewtransaction.RiskMedium || len(started.SelectedLenses) != 1 {
		t.Fatalf("tier 1 START = %#v", started)
	}
	assertReviewConsentPrompt(t, console.String(), "one consolidated review")
	// Accepting the review is the safe direction, so it is the only answer that
	// is persisted: later candidates are reviewed silently.
	if asked, err := reviewtransaction.RDDConsentAsked(context.Background(), repo); err != nil || !asked {
		t.Fatalf("accepting the review did not record the answer: asked=%v err=%v", asked, err)
	}

	console.Reset()
	writeReviewStartCandidate(t, repo, "internal/second.go", "package internal\n", 0o644)
	output.Reset()
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-tier-one-second"}, &output); err != nil {
		t.Fatalf("second tier 1 start: %v\n%s", err, output.String())
	}
	if console.Len() != 0 {
		t.Fatalf("an answered question was asked again:\n%s", console.String())
	}
}

func TestTierTwoReviewStartAsksOnceNamingTheTriggeringEvidence(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	console := stubReviewConsole(t, true, "1\n")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-tier-two"}, &output); err != nil {
		t.Fatalf("tier 2 start: %v\n%s", err, output.String())
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	if started.RiskLevel != reviewtransaction.RiskHigh || len(started.SelectedLenses) != 4 {
		t.Fatalf("tier 2 START = %#v", started)
	}
	prompt := assertReviewConsentPrompt(t, console.String(), "scripts/deploy.sh")
	if !strings.Contains(prompt, "shell") {
		t.Fatalf("tier 2 prompt does not name the triggering evidence:\n%s", prompt)
	}

	console.Reset()
	writeReviewStartCandidate(t, repo, "scripts/release.sh", "echo release\n", 0o644)
	output.Reset()
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-tier-two-second"}, &output); err != nil {
		t.Fatalf("second tier 2 start: %v\n%s", err, output.String())
	}
	if console.Len() != 0 {
		t.Fatalf("an answered question was asked again:\n%s", console.String())
	}
}

// TestReviewConsentNeverAskAgainIsNoLongerAnOfferedAnswer keeps guarding what
// the old third option guarded — that a permanent disable is observable through
// review mode — by proving the keystroke no longer reaches it. Turning the
// safety net off for good now costs a deliberate command.
func TestReviewConsentNeverAskAgainIsNoLongerAnOfferedAnswer(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	console := stubReviewConsole(t, true, "3\n")
	writeReviewStartCandidate(t, repo, "internal/app.go", "package internal\n", 0o644)

	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-never"}, &output); err != nil {
		t.Fatalf("an unoffered answer must fail safe by reviewing: %v\n%s", err, output.String())
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	if len(started.SelectedLenses) != 1 {
		t.Fatalf("an unoffered answer did not review the candidate: %#v", started)
	}

	var modeOutput bytes.Buffer
	if err := RunReviewMode([]string{"status", "--cwd", repo, "--json"}, &modeOutput); err != nil {
		t.Fatalf("review mode status: %v", err)
	}
	status := decodeReviewModeResult(t, modeOutput.Bytes()).Status
	if status.Effective != reviewtransaction.RDDModeOn || status.CloneLocal != reviewtransaction.RDDModeUnset {
		t.Fatalf("an unoffered answer disabled receipt-driven development: %#v", status)
	}
	if !strings.Contains(console.String(), "did not recognize") {
		t.Fatalf("an unoffered answer was not reported to the user:\n%s", console.String())
	}
}

// TestReviewConsentUnrecognizedAnswerReviewsAndAsksAgain proves the fail-safe
// direction: an answer nobody offered reviews the candidate and leaves the
// question unanswered, so the next candidate can still ask.
func TestReviewConsentUnrecognizedAnswerReviewsAndAsksAgain(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	console := stubReviewConsole(t, true, "maybe later\n")
	writeReviewStartCandidate(t, repo, "internal/app.go", "package internal\n", 0o644)

	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-unknown"}, &output); err != nil {
		t.Fatalf("unrecognized answer start: %v\n%s", err, output.String())
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	if len(started.SelectedLenses) != 1 {
		t.Fatalf("unrecognized answer did not review the candidate: %#v", started)
	}
	if asked, err := reviewtransaction.RDDConsentAsked(context.Background(), repo); err != nil || asked {
		t.Fatalf("an unrecognized answer consumed the question: asked=%v err=%v", asked, err)
	}

	console.Reset()
	writeReviewStartCandidate(t, repo, "internal/second.go", "package internal\n", 0o644)
	output.Reset()
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-unknown-second"}, &output); err != nil {
		t.Fatalf("second start after an unrecognized answer: %v\n%s", err, output.String())
	}
	assertReviewConsentPrompt(t, console.String(), "one consolidated review")
}

// TestReviewConsentNotNowIsNotPersisted pins the asymmetric latch and the
// non-error decline: "not now" is a reported user choice, never a failure.
// Declining applies to this candidate only, because today's README says
// nothing about tomorrow's migration.
func TestReviewConsentNotNowIsNotPersisted(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	console := stubReviewConsole(t, true, "2\n")
	writeReviewStartCandidate(t, repo, "internal/app.go", "package internal\n", 0o644)

	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-not-now"}, &output); err != nil {
		t.Fatalf("declining the review must not be an error: %v\n%s", err, output.String())
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	if started.Consent != ReviewStartConsentDeclinedThisCandidate {
		t.Fatalf("declined START consent = %q, want %q", started.Consent, ReviewStartConsentDeclinedThisCandidate)
	}
	if started.LensesRequired || len(started.SelectedLenses) != 0 || started.LineageID != "" {
		t.Fatalf("declined START still selected a review: %#v", started)
	}
	if !strings.Contains(console.String(), "at your request") {
		t.Fatalf("declining did not report the skip on the console:\n%s", console.String())
	}
	if asked, err := reviewtransaction.RDDConsentAsked(context.Background(), repo); err != nil || asked {
		t.Fatalf("not-now recorded the answer: asked=%v err=%v", asked, err)
	}
	if store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "review-not-now"); err == nil {
		if _, loadErr := store.Load(); !errors.Is(loadErr, os.ErrNotExist) {
			t.Fatalf("declined START persisted review authority: %v", loadErr)
		}
	}

	var modeOutput bytes.Buffer
	if err := RunReviewMode([]string{"status", "--cwd", repo, "--json"}, &modeOutput); err != nil {
		t.Fatalf("review mode status: %v", err)
	}
	if status := decodeReviewModeResult(t, modeOutput.Bytes()).Status; status.Effective != reviewtransaction.RDDModeOn {
		t.Fatalf("not-now persisted a disabled mode: %#v", status)
	}

	console.Reset()
	writeReviewStartCandidate(t, repo, "internal/second.go", "package internal\n", 0o644)
	output.Reset()
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-not-now-second"}, &output); err != nil {
		t.Fatalf("the next candidate after not-now must ask again without an error: %v\n%s", err, output.String())
	}
	assertReviewConsentPrompt(t, console.String(), "one consolidated review")
	var second ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &second)
	if second.Consent != ReviewStartConsentDeclinedThisCandidate {
		t.Fatalf("second declined START consent = %q, want %q", second.Consent, ReviewStartConsentDeclinedThisCandidate)
	}
}

func TestNonInteractiveReviewStartNeverBlocksOnConsent(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	console := stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-non-interactive"}, &output); err != nil {
		t.Fatalf("non-interactive start: %v\n%s", err, output.String())
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	if len(started.SelectedLenses) != 4 {
		t.Fatalf("non-interactive start did not review the candidate: %#v", started)
	}
	notice := console.String()
	if !strings.Contains(notice, "review mode disable") {
		t.Fatalf("non-interactive notice is not discoverable:\n%s", notice)
	}
	if asked, err := reviewtransaction.RDDConsentAsked(context.Background(), repo); err != nil || asked {
		t.Fatalf("a non-interactive run consumed the one-time question: asked=%v err=%v", asked, err)
	}
}

// TestNonInteractiveReviewStartNoticeShownOnlyOnce covers issue #1848: a
// tester driving many flows in the same non-interactive clone saw the
// "no terminal to answer on" notice on every single start. The first
// occurrence must never be suppressed (it is the only way the user learns
// the tool reviewed without asking); later occurrences in the same clone
// carry no new information and must be rate-limited to once. This must not
// reuse the RDDConsentAsked latch: TestNonInteractiveReviewStartNeverBlocksOnConsent
// (above) establishes that a non-interactive run must never consume the
// one-time consent question, so a later interactive session in the same
// clone can still be asked.
func TestNonInteractiveReviewStartNoticeShownOnlyOnce(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	console := stubReviewConsole(t, false, "")

	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)
	var first bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-notice-once-a"}, &first); err != nil {
		t.Fatalf("first non-interactive start: %v\n%s", err, first.String())
	}
	if !strings.Contains(console.String(), reviewConsentSkippedNotice) {
		t.Fatalf("the first non-interactive start must show the notice:\n%s", console.String())
	}

	console.Reset()
	writeReviewStartCandidate(t, repo, "scripts/deploy2.sh", "echo deploy2\n", 0o644)
	var second bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-notice-once-b"}, &second); err != nil {
		t.Fatalf("second non-interactive start: %v\n%s", err, second.String())
	}
	if strings.Contains(console.String(), reviewConsentSkippedNotice) {
		t.Fatalf("a repeated non-interactive start must not repeat the notice:\n%s", console.String())
	}

	if asked, err := reviewtransaction.RDDConsentAsked(context.Background(), repo); err != nil || asked {
		t.Fatalf("notice rate-limiting must not consume the one-time consent question: asked=%v err=%v", asked, err)
	}
}

// reviewConsentChoicePattern matches one numbered answer the question offers.
// The count is asserted, because turning reviews off for good must never be
// reachable by pressing a number in a hurried moment.
var reviewConsentChoicePattern = regexp.MustCompile(`(?m)^\s*\d+\)\s`)

// assertReviewConsentPrompt checks the shared shape of the question: the value
// framing, exactly the two offered answers, the deliberate off path named as a
// trailing line rather than a choice, and the tier-specific reason. It also
// rejects internal vocabulary, because the question is for a human.
func assertReviewConsentPrompt(t *testing.T, prompt, reason string) string {
	t.Helper()
	for _, want := range []string{
		reason,
		"takes a bit longer",
		"substantially safer",
		"1) Run the review now",
		"2) Not now, just this once",
		"gentle-ai review mode disable",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("consent prompt missing %q:\n%s", want, prompt)
		}
	}
	if choices := reviewConsentChoicePattern.FindAllString(prompt, -1); len(choices) != 2 {
		t.Fatalf("consent prompt offers %d numbered choices, want exactly 2:\n%s", len(choices), prompt)
	}
	for _, forbidden := range []string{"Never ask again", "never ask again", "3)"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("consent prompt still offers a permanent disable %q:\n%s", forbidden, prompt)
		}
	}
	for _, forbidden := range []string{"gentle-ai.", "sha256:", "lineage", "schema", "contract", "lens"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("consent prompt leaked internal vocabulary %q:\n%s", forbidden, prompt)
		}
	}
	return prompt
}

// stubReviewConsole replaces the console seam so the one-time question can be
// driven without a terminal. It returns the buffer the question is written to.
func stubReviewConsole(t *testing.T, interactive bool, answer string) *bytes.Buffer {
	t.Helper()
	previous := reviewConsole
	buffer := &bytes.Buffer{}
	reviewConsole = func() reviewConsentSession {
		return reviewConsentSession{Interactive: interactive, Input: strings.NewReader(answer), Output: buffer}
	}
	t.Cleanup(func() { reviewConsole = previous })
	return buffer
}

func reviewModeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// reviewEnabledHome is reviewModeHome for a user who opted in. Receipt-driven
// development is off until someone explicitly enables it, so a test whose
// subject is the review lifecycle -- rather than the switch itself -- has to
// opt in the way a real user does before a review will start at all. It writes
// the same explicit global "on" that `gentle-ai review mode enable` persists,
// rather than reaching past the switch, so these fixtures keep exercising the
// resolution path they are meant to run through.
//
// The opinion lives in the user's home directory, which is process-wide state
// reached through t.Setenv. Go forbids t.Setenv in a test that also calls
// t.Parallel, so a test that opts in cannot be parallel: there is no
// repository-scoped way to assert "on" (a clone may only ever assert "off").
func reviewEnabledHome(t *testing.T) string {
	t.Helper()
	home := reviewModeHome(t)
	recordedAt := time.Now().UTC()
	if err := state.Write(home, state.InstallState{
		RDDMode:           string(reviewtransaction.RDDModeOn),
		RDDModeRecordedAt: &recordedAt,
	}); err != nil {
		t.Fatalf("enable review mode for this test: %v", err)
	}
	return home
}

func decodeReviewModeResult(t *testing.T, payload []byte) ReviewModeResult {
	t.Helper()
	var result ReviewModeResult
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("decode review mode result: %v\n%s", err, payload)
	}
	return result
}
