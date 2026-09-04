package sddstatus

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// #3872, #3879, and #3884 share one root class: a compact ledger answer that
// does not name its runnable exit. Settle collapsed five causes into one
// generic invalid_continuation text, and complete carried no exit at all.

func TestSettleWithoutActiveAttemptNamesAcquire(t *testing.T) {
	store := mustRuntimeStore(t, initRuntimeLedgerRepo(t), "settle-nothing-active")
	result, err := store.Settle(context.Background(), compactSettleFixture("idle-settle", runtimeTestHash('1'), AttemptPassed, ""))
	if err != nil {
		t.Fatal(err)
	}
	assertSettleBlockedExit(t, result, "nothing to settle", "`gentle-ai sdd-attempt acquire --cwd <repo> --change <change>")
}

func TestCompactSettleStaleUntrackedInventoryReportsNativeRecovery(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	for _, path := range []string{"selected-a.txt", "selected-z.txt"} {
		if err := os.WriteFile(filepath.Join(repo, path), []byte("selected\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	store := mustRuntimeStore(t, repo, "compact-stale-untracked-recovery")
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "stale-recovery-begin", WorkUnit: "recover stale inventory",
		EvidenceGoal: "report the compact recovery contract", MaxAttempts: 2, MaxChangedLines: 20,
		IntendedUntracked: []string{"selected-a.txt", "selected-z.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	staleInventory := currentUntrackedInventoryDigest(t, repo)
	if err := os.WriteFile(filepath.Join(repo, "born.txt"), []byte("born\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	currentInventory := currentUntrackedInventoryDigest(t, repo)
	if currentInventory == staleInventory {
		t.Fatalf("test setup did not make the eligible inventory stale: %q", currentInventory)
	}
	before, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	beforeRecords := countRuntimeRecords(t, store.Dir)

	selection := []string{"born.txt", "selected-a.txt", "selected-z.txt"}
	result, err := store.Settle(context.Background(), CompactSettleRequest{
		Token: started.Revision, RequestID: "stale-recovery-settle", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('a'), Diagnosis: "stale inventory needs a native recovery delta",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "process scan clean",
		IntendedUntracked: &selection, ExpectedUntrackedInventory: staleInventory,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != CompactStateBlocked || result.Reason != CompactBlockUndeclaredUntracked || result.Token != "" {
		t.Fatalf("stale compact settle = %#v, want blocked(undeclared_untracked) without a receipt token", result)
	}
	after, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || after.ActiveAttempt == nil || after.ActiveAttempt.Outcome != AttemptRunning || len(after.Attempts) != len(before.Attempts) || countRuntimeRecords(t, store.Dir) != beforeRecords {
		t.Fatalf("stale compact settle mutated attempt authority: status=%#v records=%d, want revision=%q and records=%d", after, countRuntimeRecords(t, store.Dir), before.Revision, beforeRecords)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	recoveryJSON, ok := document["recovery"]
	if !ok {
		t.Fatalf("compact stale-inventory refusal is missing the required recovery delta: %s", encoded)
	}
	var recovery map[string]json.RawMessage
	if err := json.Unmarshal(recoveryJSON, &recovery); err != nil {
		t.Fatal(err)
	}
	if len(recovery) != 2 {
		t.Fatalf("recovery fields = %v, want only expected_untracked_inventory and retained_intended_untracked", recovery)
	}
	var typed struct {
		ExpectedUntrackedInventory string   `json:"expected_untracked_inventory"`
		RetainedIntendedUntracked  []string `json:"retained_intended_untracked"`
	}
	if err := json.Unmarshal(recoveryJSON, &typed); err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(typed.ExpectedUntrackedInventory) || typed.ExpectedUntrackedInventory != currentInventory {
		t.Fatalf("recovery expected_untracked_inventory = %q, want current canonical digest %q", typed.ExpectedUntrackedInventory, currentInventory)
	}
	wantRetained := []string{"selected-a.txt", "selected-z.txt"}
	if !slices.Equal(typed.RetainedIntendedUntracked, wantRetained) {
		t.Fatalf("recovery retained_intended_untracked = %q, want canonical mandatory selection floor %q", typed.RetainedIntendedUntracked, wantRetained)
	}

	// The refusal wrote no receipt, so the same request ID remains available.
	// A second inventory change must replace rather than reuse the recovery
	// digest, and the corrected retry retains the original selected floor.
	if err := os.WriteFile(filepath.Join(repo, "later.txt"), []byte("later\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := store.Settle(context.Background(), CompactSettleRequest{
		Token: started.Revision, RequestID: "stale-recovery-settle", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('a'), Diagnosis: "stale inventory needs a native recovery delta",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "process scan clean",
		IntendedUntracked: &selection, ExpectedUntrackedInventory: typed.ExpectedUntrackedInventory,
	})
	if err != nil {
		t.Fatal(err)
	}
	currentInventory = currentUntrackedInventoryDigest(t, repo)
	if second.State != CompactStateBlocked || second.Recovery == nil || second.Recovery.ExpectedUntrackedInventory != currentInventory ||
		!slices.Equal(second.Recovery.RetainedIntendedUntracked, wantRetained) {
		t.Fatalf("second stale compact settle = %#v, want current recovery retaining %q", second, wantRetained)
	}
	corrected := []string{"born.txt", "later.txt", "selected-a.txt", "selected-z.txt"}
	settled, err := store.Settle(context.Background(), CompactSettleRequest{
		Token: started.Revision, RequestID: "stale-recovery-settle", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('a'), Diagnosis: "stale inventory needs a native recovery delta",
		HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "process scan clean",
		IntendedUntracked: &corrected, ExpectedUntrackedInventory: second.Recovery.ExpectedUntrackedInventory,
	})
	if err != nil || settled.State != CompactStateComplete {
		t.Fatalf("same-ID corrected retry = %#v, err=%v", settled, err)
	}
}

func TestSettlePassedWithoutRemediatesNamesTheChainEvidence(t *testing.T) {
	store := seedUnremediatedFailure(t, "settle-owes-remediation")
	attempt := acquireRegressionAttempt(t, store, "b-acquire", "sdd-remediate", "correct failed verification A", "", 2)
	result, err := store.Settle(context.Background(), compactSettleFixture("b-settle", attempt.Token, AttemptPassed, ""))
	if err != nil {
		t.Fatal(err)
	}
	assertSettleBlockedExit(t, result, "this passed settle was refused", regressionFailedEvidence, "--remediates-evidence-revision")
}

func TestSettleRemediatesMismatchNamesTheChainEvidence(t *testing.T) {
	other := runtimeTestHash('c')
	store := seedUnremediatedFailure(t, "settle-remediates-mismatch")
	attempt := acquireRegressionAttempt(t, store, "b-acquire", "sdd-remediate", "correct failed verification A", "", 2)
	result, err := store.Settle(context.Background(), compactSettleFixture("b-settle", attempt.Token, AttemptPassed, other))
	if err != nil {
		t.Fatal(err)
	}
	assertSettleBlockedExit(t, result, "not "+other, "--remediates-evidence-revision "+regressionFailedEvidence)

	fresh := mustRuntimeStore(t, initRuntimeLedgerRepo(t), "settle-remediates-nothing")
	attempt = acquireRegressionAttempt(t, fresh, "a-acquire", "sdd-apply", "apply the change", "", 2)
	result, err = fresh.Settle(context.Background(), compactSettleFixture("a-settle", attempt.Token, AttemptPassed, other))
	if err != nil {
		t.Fatal(err)
	}
	assertSettleBlockedExit(t, result, "names nothing", other, "without that flag")
}

// The changed-line and older-binary preconditions cannot be reached through
// the CLI (a finish that exceeds its budget never completes), so they are
// driven through the pure readiness predicate on a constructed ledger state.
func TestCompleteExitNamesEachSuccessorPrecondition(t *testing.T) {
	objective := &RuntimeObjective{ID: runtimeTestHash('o'), WorkUnit: "slice-1"}
	passed := RuntimeAttempt{
		ObjectiveID: objective.ID, Outcome: AttemptPassed,
		FinishCandidateIdentity: runtimeTestHash('i'), FinishCandidateTree: strings.Repeat("a", 40),
	}
	exceeded, unbound := passed, passed
	exceeded.ChangedLineBudgetExceeded = true
	unbound.FinishCandidateIdentity, unbound.FinishCandidateTree = "", ""
	for _, tt := range []struct {
		name     string
		last     RuntimeAttempt
		workUnit string
		want     []string
	}{
		{name: "no work unit", last: passed, want: []string{"(slice-1) is complete", "--work-unit \"<a different label>\""}},
		{name: "same label", last: passed, workUnit: "slice-1", want: []string{"--work-unit \"slice-1\" restates the completed objective; choose a different label"}},
		{name: "budget exceeded", last: exceeded, workUnit: "slice-2", want: []string{"exceeded its changed-line budget", "`gentle-ai sdd-attempt reset --cwd <repo> --change <change>"}},
		{name: "older binary", last: unbound, workUnit: "slice-2", want: []string{"no finish candidate identity", "older binary", "`gentle-ai sdd-attempt reset --cwd <repo> --change <change>"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			status := RuntimeStatus{Complete: true, Objective: objective, Attempts: []RuntimeAttempt{tt.last}}
			result, terminal := runtimeReadiness(runtimeReadinessInput{Status: status, Request: BeginAttemptRequest{WorkUnit: tt.workUnit}})
			if !terminal || result.State != CompactStateComplete || result.Exit == "" || result.Detail != result.Exit {
				t.Fatalf("readiness = %#v terminal=%v, want complete with a named exit", result, terminal)
			}
			for _, want := range tt.want {
				if !strings.Contains(result.Exit, want) {
					t.Fatalf("complete exit does not name %q:\n%s", want, result.Exit)
				}
			}
		})
	}
}

func seedUnremediatedFailure(t *testing.T, change string) RuntimeStore {
	t.Helper()
	store := mustRuntimeStore(t, initRuntimeLedgerRepo(t), change)
	failed := acquireRegressionAttempt(t, store, "a-acquire", "sdd-verify", "record failed verification A", "", 1)
	settleRegressionAttempt(t, store, "a-settle", failed.Token, AttemptFailed, regressionFailedEvidence, "")
	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: status.Revision, RequestID: "a-reset", Reason: "maintainer authorizes remediation B", Actor: "maintainer",
	}); err != nil {
		t.Fatalf("authorized reset: %v", err)
	}
	return store
}

func compactSettleFixture(requestID, token string, outcome AttemptOutcome, remediates string) CompactSettleRequest {
	return CompactSettleRequest{
		RequestID: requestID, Token: token, Outcome: outcome, EvidenceRevision: regressionPassedEvidence,
		Diagnosis: "settle exit fixture " + requestID, HarnessDisposition: HarnessReused,
		CleanupEvidence: "fixture has no external resources", ProcessEvidence: "fixture process scan found no descendants",
		RemediatesEvidenceRevision: remediates,
	}
}

func assertSettleBlockedExit(t *testing.T, result CompactAttemptResult, wants ...string) {
	t.Helper()
	if result.State != CompactStateBlocked || result.Reason != CompactBlockInvalidContinuation || result.Detail != result.Exit || result.Recovery != nil {
		t.Fatalf("settle = %#v, want blocked(invalid_continuation) with detail mirroring exit", result)
	}
	for _, want := range append(wants, "`gentle-ai sdd-attempt status --cwd <repo> --change <change>`") {
		if !strings.Contains(result.Exit, want) {
			t.Fatalf("settle exit does not name %q:\n%s", want, result.Exit)
		}
	}
}
