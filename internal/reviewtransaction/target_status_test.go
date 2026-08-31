package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTargetStatusDecisionFailsClosedBeforeAnAdapterCanBroadenRecovery(t *testing.T) {
	base := TargetStatusResult{
		Applicability: TargetApplicabilityCurrent, Action: TargetStatusActionRecover,
		ActionDisposition: RecoveryScopeChanged, TargetIdentity: "sha256:target",
		Projection:          TargetProjectionStatus{Kind: TargetBaseDiff, Projection: ProjectionWorkspace, BaseTree: "next-base"},
		authorityTargetKind: TargetCurrentChanges, authorityProjection: ProjectionWorkspace,
	}
	decision := projectTargetStatusDecision(base)
	if decision.Decision.RecoverySelector != nil {
		t.Fatalf("non-approved cross-kind recovery selector = %#v, want fail-closed", decision.Decision.RecoverySelector)
	}
	if decision.Decision.CandidateRelation != TargetApplicabilityCurrent || decision.Decision.SemanticTransition != TargetStatusActionRecover ||
		decision.Decision.TargetIdentity != base.TargetIdentity {
		t.Fatalf("decision lost core classification: %#v", decision.Decision)
	}
}

func TestAssessTargetStatusClassifiesAllApplicabilityStates(t *testing.T) {
	requireSnapshotGit(t)
	request := TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}}

	t.Run("current target fresh start", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
		state := newCompactTestState(t, repo, "review-current")
		state, store := startReviewingCompactAuthority(t, repo, state)
		record, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}

		got, err := AssessTargetStatus(context.Background(), repo, request)
		if err != nil {
			t.Fatal(err)
		}
		if got.Applicability != TargetApplicabilityCurrent || got.State != StateReviewing {
			t.Fatalf("status = %#v", got)
		}
		if got.LineageID != state.LineageID || got.Generation != state.Generation || got.Revision != record.Revision {
			t.Fatalf("authority identity = %#v", got)
		}
		if got.OriginalChangedLines != state.OriginalChangedLines || got.Tier != state.RiskLevel || got.CorrectionBudget != state.CorrectionBudget {
			t.Fatalf("frozen review inputs = %#v", got)
		}
		if got.TargetIdentity != state.InitialSnapshot.Identity || got.Projection.CurrentCandidateTree != state.CurrentSnapshot.CandidateTree {
			t.Fatalf("projection = %#v", got.Projection)
		}
	})

	t.Run("unrelated", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "first candidate\n")
		_, _ = startReviewingCompactAuthority(t, repo, newCompactTestState(t, repo, "review-old"))
		writeSnapshotFile(t, repo, "tracked.txt", "different candidate\n")
		got, err := AssessTargetStatus(context.Background(), repo, request)
		if err != nil {
			t.Fatal(err)
		}
		if got.Applicability != TargetApplicabilityUnrelated || got.Action != TargetStatusActionStart || got.LineageID != "" {
			t.Fatalf("status = %#v", got)
		}
	})

	t.Run("ambiguous", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
		_, unrelated := startReviewingCompactAuthority(t, repo, newCompactTestState(t, repo, "review-b"))
		_, _ = startReviewingCompactAuthority(t, repo, newCompactTestState(t, repo, "review-a"))
		got, err := AssessTargetStatus(context.Background(), repo, request)
		if err != nil {
			t.Fatal(err)
		}
		if got.Applicability != TargetApplicabilityAmbiguous || got.Action != TargetStatusActionSelectLineage || !reflect.DeepEqual(got.CandidateLineageIDs, []string{"review-a", "review-b"}) {
			t.Fatalf("status = %#v", got)
		}
		selected := request
		selected.LineageID = "review-a"
		resolved, err := AssessTargetStatus(context.Background(), repo, selected)
		if err != nil || resolved.Applicability != TargetApplicabilityCurrent || resolved.LineageID != "review-a" {
			t.Fatalf("selected status = %#v, err = %v", resolved, err)
		}
		if err := os.WriteFile(unrelated.StatePath(), []byte("{\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		resolved, err = AssessTargetStatus(context.Background(), repo, selected)
		if err != nil || resolved.Applicability != TargetApplicabilityCurrent || resolved.LineageID != "review-a" {
			t.Fatalf("selected status with corrupt unrelated authority = %#v, err = %v", resolved, err)
		}
		selected.LineageID = "review-b"
		if corrupt, err := AssessTargetStatus(context.Background(), repo, selected); err != nil || corrupt.Applicability != TargetApplicabilityCorrupted {
			t.Fatalf("selected corrupt status = %#v, err = %v", corrupt, err)
		}
	})

	// Corrupted is a verdict about the lineage under assessment, so it is
	// reached by naming that lineage. A selector-free assessment over the
	// same store answers about the live target instead, which is the whole
	// point: unrelated work is not corrupted because history is.
	t.Run("corrupted", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
		_, store := startReviewingCompactAuthority(t, repo, newCompactTestState(t, repo, "review-corrupt"))
		if err := os.WriteFile(store.StatePath(), []byte("{\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		unrelated, err := AssessTargetStatus(context.Background(), repo, request)
		if err != nil {
			t.Fatal(err)
		}
		if unrelated.Applicability == TargetApplicabilityCorrupted {
			t.Fatalf("a corrupt unrelated entry made the live target corrupted: %#v", unrelated)
		}
		corruptRequest := request
		corruptRequest.LineageID = "review-corrupt"
		got, err := AssessTargetStatus(context.Background(), repo, corruptRequest)
		if err != nil {
			t.Fatal(err)
		}
		if got.Applicability != TargetApplicabilityCorrupted || got.Action != TargetStatusActionRepairAuthority || got.Replayability != ReplayabilityManualActionRequired {
			t.Fatalf("status = %#v", got)
		}
		payload, _ := json.Marshal(got)
		if strings.Contains(string(payload), repo) || strings.Contains(string(payload), "review-state.json") {
			t.Fatalf("status exposes authority filesystem details: %s", payload)
		}
	})
}

func TestEscalatedChangedTargetWithChangedScopeRecovers(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := accountingOnlyEscalatedState(t, repo, "escalated-changed-scope-status")
	_, record := persistEscalatedRecoveryFixture(t, repo, state)
	writeSnapshotFile(t, repo, "tracked.txt", "changed recovery target\n")
	writeSnapshotFile(t, repo, "added.txt", "added recovery scope\n")

	status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{"added.txt"}}, LineageID: state.LineageID,
	})
	if err != nil || status.Applicability != TargetApplicabilityCurrent || status.State != StateEscalated ||
		status.Action != TargetStatusActionRecover || status.ActionDisposition != RecoveryEscalated ||
		status.LineageID != state.LineageID || status.Revision != record.Revision {
		t.Fatalf("changed-target escalated scope recovery = %#v, %v", status, err)
	}
}

// TestAccountingOnlyEscalationStatusOffersRecoveryInsteadOfDeadEndStop proves
// receiptless status keeps the evidence-bound RecoveryEscalated routing for an
// escalation whose review and correction regression both passed while only
// correction accounting exceeded its budget. STATUS must offer that
// continuation without materializing a receipt.
func TestAccountingOnlyEscalationStatusOffersRecoveryInsteadOfDeadEndStop(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := accountingOnlyEscalatedState(t, repo, "accounting-only-status-dead-end")
	persistEscalatedRecoveryFixture(t, repo, state)

	target := Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}
	status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: target, LineageID: state.LineageID})
	if err != nil {
		t.Fatal(err)
	}
	if status.Action != TargetStatusActionRecover || status.ActionDisposition != RecoveryEscalated {
		t.Fatalf("accounting-only escalation with an unchanged target = %#v, want an offered evidence-bound recovery continuation", status)
	}
	if status.Decision.RecoverySelector != nil || !status.Decision.SelectorFreeAccountingOnlyRecovery {
		t.Fatalf("accounting-only escalation decision = %#v, want an explicitly authorized selector-free recovery", status.Decision)
	}
}

// TestFailedCriteriaEscalationStatusStillStopsWithUnchangedTarget proves the
// accounting-only routing fix stays scoped: an escalation caused by a failed
// original-review or correction-regression criterion, not by accounting
// alone, must never match compactAccountingOnlyEscalation and must keep
// stopping the operator when its target has not changed.
func TestFailedCriteriaEscalationStatusStillStopsWithUnchangedTarget(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	state := newCompactTestState(t, repo, "failed-criteria-status-dead-end")
	state, store := startReviewingCompactAuthority(t, repo, state)
	if state.CorrectionBudget < 2 || len(state.SelectedLenses) != 1 {
		t.Fatalf("fixture risk/budget = %q/%d", state.RiskLevel, state.CorrectionBudget)
	}
	finding := Finding{
		ID: "R3-001", Lens: "reliability", Location: "tracked.txt:5", Severity: "CRITICAL",
		Claim: "candidate retains the wrong value", ProofRefs: []string{"candidate-only differential failure"},
		EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalIntroduced,
	}
	state, _ = captureAndCompleteCompactReview(t, store, state, CompactReviewInput{
		LensResults:     []LensResult{{Lens: state.SelectedLenses[0], Findings: []Finding{finding}, Evidence: []string{"reviewed exact candidate tree"}}},
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk"}},
		RefuterOutcomes: []EvidenceResult{},
	})
	if err := state.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfixed\n")
	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetFixDiff, Projection: state.InitialSnapshot.Projection, BaseRef: state.CurrentSnapshot.CandidateTree,
		IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := bindTargetedValidationForTest(ScopedValidationResult{
		LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: true},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: false},
	}, fix)
	if err := state.CompleteCorrection(fix, 1, validation); err != nil {
		t.Fatal(err)
	}
	if state.State != StateEscalated {
		t.Fatalf("fixture state = %q, want escalated", state.State)
	}
	if compactAccountingOnlyEscalation(state) {
		t.Fatalf("failed-criteria escalation must never match the accounting-only predicate: %#v", state)
	}
	_, record := persistEscalatedRecoveryFixture(t, repo, state)

	status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: state.LineageID,
	})
	if err != nil || status.Action != TargetStatusActionStop || status.ActionDisposition != "" || status.Revision != record.Revision {
		t.Fatalf("failed-criteria escalation status with an unchanged target = %#v, err=%v, want a terminal stop", status, err)
	}
}

// TestAccountingOnlyEscalationRecoveryStillRequiresMaintainerAuthorization
// proves the offered continuation still routes through the native
// maintainer-authorization gate; STATUS names a disposition, it never
// bypasses the authorization the recovery edge demands.
func TestAccountingOnlyEscalationRecoveryStillRequiresMaintainerAuthorization(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := accountingOnlyEscalatedState(t, repo, "accounting-only-auth-required")
	_, record := persistEscalatedRecoveryFixture(t, repo, state)
	successor := recoveredEvidenceSuccessor(t, repo, state, "accounting-only-auth-required-successor")
	const actor, reason = "maintainer@example.com", "recover accounting-only escalation"
	_, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
		PredecessorLineageID: state.LineageID, ExpectedPredecessorRevision: record.Revision,
		Successor: successor, Disposition: RecoveryEscalated, Reason: reason, Actor: actor,
		MaintainerAuthorization: "wrong-authorization",
	})
	if err == nil || !errors.Is(err, ErrCompactRecoveryAuthorizationInexact) {
		t.Fatalf("accounting-only recovery without exact maintainer authorization = %v, want authorization error", err)
	}
}

func TestCompactTargetStatusUsesExactCurrentCandidateAndLiveProjection(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	writeSnapshotFile(t, repo, "new.txt", "initial\n")
	builder := SnapshotBuilder{Repo: repo}
	initial, _ := builder.Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{"new.txt"}})
	writeSnapshotFile(t, repo, "new.txt", "corrected\n")
	live, _ := builder.Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{"new.txt"}})
	fix, _ := builder.Build(context.Background(), Target{Kind: TargetFixDiff, BaseRef: initial.CandidateTree, IntendedUntracked: []string{"new.txt"}, LedgerIDs: []string{"R1-001"}})
	state := CompactState{InitialSnapshot: initial, CurrentSnapshot: fix, GenesisPaths: initial.Paths}
	if !compactLiveTargetMatchesSnapshot(context.Background(), repo, state, live, true) {
		t.Fatal("terminal correction did not match the exact current candidate")
	}
	wrongProof := state
	wrongProof.CurrentSnapshot.IntendedUntrackedProof = initial.IntendedUntrackedProof
	if !compactLiveTargetMatchesSnapshot(context.Background(), repo, wrongProof, live, true) {
		t.Fatal("terminal correction rejected an exact candidate for a stale intended-untracked proof")
	}
	projection := targetProjectionFromCompact(state, targetProjectionFromSnapshot(live))
	if projection.Kind != live.Kind || projection.PathsDigest != live.PathsDigest || projection.IntendedUntrackedProof != live.IntendedUntrackedProof || projection.CurrentSnapshotIdentity != live.Identity || projection.InitialSnapshotIdentity != initial.Identity {
		t.Fatalf("corrected projection = %#v, live = %#v", projection, live)
	}
}

func TestAssessTargetStatusReconstructsAfterRestartWithoutMutation(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "review-restart")
	state, _ = startReviewingCompactAuthority(t, repo, state)
	authorityRoot, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	before := authorityBytes(t, authorityRoot)
	request := TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}}
	first, err := AssessTargetStatus(context.Background(), repo, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AssessTargetStatus(context.Background(), repo, request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || first.Projection.InitialSnapshotIdentity != state.InitialSnapshot.Identity || first.Projection.CurrentSnapshotIdentity != state.CurrentSnapshot.Identity {
		t.Fatalf("restart reconstruction differs: first=%#v second=%#v", first, second)
	}
	if after := authorityBytes(t, authorityRoot); !reflect.DeepEqual(before, after) {
		t.Fatalf("read-only status mutated authority: before=%v after=%v", before, after)
	}
}

func TestAssessTargetStatusIgnoresUnrelatedValidLegacyHistory(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	head := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	legacySnapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetExactRevision, Revision: head})
	if err != nil {
		t.Fatal(err)
	}
	storeLegacyReviewingStatus(t, repo, "legacy-history", legacySnapshot)
	writeSnapshotFile(t, repo, "tracked.txt", "compact candidate\n")
	compact := newCompactTestState(t, repo, "review-current")
	compact, _ = startReviewingCompactAuthority(t, repo, compact)

	got, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Applicability != TargetApplicabilityCurrent || got.AuthorityVersion != AuthorityVersionCompact || got.LineageID != compact.LineageID {
		t.Fatalf("status = %#v", got)
	}
}

func TestAssessTargetStatusKeepsExplicitCompactLineageCurrentWithInvalidLegacyInventory(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	head := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	legacySnapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetExactRevision, Revision: head})
	if err != nil {
		t.Fatal(err)
	}
	legacyLineage := "legacy-invalid-history"
	storeLegacyReviewingStatus(t, repo, legacyLineage, legacySnapshot)
	legacyStore, err := AuthoritativeStore(context.Background(), repo, legacyLineage)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyStore.Dir, "HEAD"), []byte("not-a-revision\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeSnapshotFile(t, repo, "tracked.txt", "compact candidate\n")
	compact := newCompactTestState(t, repo, "review-explicit-current")
	compact, _ = startReviewingCompactAuthority(t, repo, compact)
	authorityRoot, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	before := authorityBytes(t, authorityRoot)
	request := TargetStatusRequest{
		Target:    Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}},
		LineageID: compact.LineageID,
	}
	got, err := AssessTargetStatus(context.Background(), repo, request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Applicability != TargetApplicabilityCurrent || got.AuthorityVersion != AuthorityVersionCompact ||
		got.LineageID != compact.LineageID {
		t.Fatalf("explicit compact status = %#v", got)
	}

	unscoped := request
	unscoped.LineageID = ""
	global, err := AssessTargetStatus(context.Background(), repo, unscoped)
	if err != nil {
		t.Fatal(err)
	}
	if global.Applicability != TargetApplicabilityCurrent || global.AuthorityVersion != AuthorityVersionCompact ||
		global.LineageID != compact.LineageID {
		t.Fatalf("ordinary status did not keep the compact authority exclusive: %#v", global)
	}
	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	invalidLegacyEvidence := false
	for _, entry := range report.Entries {
		invalidLegacyEvidence = invalidLegacyEvidence || entry.LineageID == legacyLineage && entry.Status == AuthorityStatusInvalid && len(entry.Problems) > 0
	}
	if !invalidLegacyEvidence {
		t.Fatalf("invalid legacy inventory diagnostics = %#v", report)
	}
	if after := authorityBytes(t, authorityRoot); !reflect.DeepEqual(before, after) {
		t.Fatal("target status or inventory mutated authority")
	}
}

func TestAssessTargetStatusLeavesNonTerminalLegacyForManualCompatibilityWithoutMutation(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "legacy candidate\n")
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	lineage := "legacy-read-only-status"
	storeLegacyReviewingStatus(t, repo, lineage, snapshot)
	authorityRoot, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	before := authorityBytes(t, authorityRoot)
	request := TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: lineage}
	for attempt := 0; attempt < 2; attempt++ {
		got, err := AssessTargetStatus(context.Background(), repo, request)
		if err != nil {
			t.Fatal(err)
		}
		if got.Applicability != TargetApplicabilityUnrelated || got.AuthorityVersion != "" ||
			got.Action != TargetStatusActionStart || got.Replayability != ReplayabilityNotReplayable || got.State != "" {
			t.Fatalf("ordinary status consulted legacy authority: %#v", got)
		}
		if after := authorityBytes(t, authorityRoot); !reflect.DeepEqual(after, before) {
			t.Fatalf("attempt %d mutated legacy authority", attempt+1)
		}
	}
}

// TestAssessTargetStatusReportsPluralStaleLineagesAsUnrelatedWithOptionalRecovery
// is the positive proof for organic-dx Phase 3e: zero EXACTLY governing
// candidates plus 2+ stale (scope-changed) lineages must never decide
// anything by itself. It reports the identical "nothing governs, start
// fresh" shape a genuinely empty candidate set already reports, not
// ambiguity/select_lineage. The stale lineages stay listed in
// CandidateLineageIDs purely so recovering one of them remains a
// discoverable OPTION, never a required disambiguation chore forced by
// history alone.
func TestAssessTargetStatusReportsPluralStaleLineagesAsUnrelatedWithOptionalRecovery(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "reviewed candidate\n")
	writeApprovedTargetStatusHistory(t, repo, 2)
	gitSnapshot(t, repo, "add", "-A")
	gitSnapshot(t, repo, "commit", "-m", "deliver reviewed candidate")
	writeSnapshotFile(t, repo, "tracked.txt", "related follow-up\n")

	got, err := AssessTargetStatus(context.Background(), repo, targetStatusCurrentChangesRequest())
	if err != nil {
		t.Fatal(err)
	}
	if got.Applicability != TargetApplicabilityUnrelated || got.Action != TargetStatusActionStart ||
		got.Replayability != ReplayabilityNotReplayable || got.LineageID != "" {
		t.Fatalf("plural stale status = %#v", got)
	}
	if !equalStrings(got.CandidateLineageIDs, []string{"status-history-000", "status-history-001"}) {
		t.Fatalf("plural stale candidate lineages = %#v", got.CandidateLineageIDs)
	}
}

// TestAssessTargetStatusKeepsSingleStaleLineageUnrelatedWithoutCandidateListing
// is negative proof #2 for organic-dx Phase 3e: exactly ONE stale
// (scope-changed) lineage must remain byte-identical to today's shape — the
// correction targets 2+ stale lineages only.
func TestAssessTargetStatusKeepsSingleStaleLineageUnrelatedWithoutCandidateListing(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "reviewed candidate\n")
	writeApprovedTargetStatusHistory(t, repo, 1)
	gitSnapshot(t, repo, "add", "-A")
	gitSnapshot(t, repo, "commit", "-m", "deliver reviewed candidate")
	writeSnapshotFile(t, repo, "tracked.txt", "related follow-up\n")

	got, err := AssessTargetStatus(context.Background(), repo, targetStatusCurrentChangesRequest())
	if err != nil {
		t.Fatal(err)
	}
	if got.Applicability != TargetApplicabilityUnrelated || got.Action != TargetStatusActionStart ||
		got.Replayability != ReplayabilityNotReplayable || got.LineageID != "" || len(got.CandidateLineageIDs) != 0 {
		t.Fatalf("single stale status = %#v", got)
	}
}

// TestAssessTargetStatusKeepsExactAmbiguityWhenStaleLineagesAlsoExist is
// negative proof #1 for organic-dx Phase 3e: 2+ EXACTLY governing candidates
// remain ambiguous/select_lineage even when stale lineages exist alongside
// them. That is present-tense competing authority — real damage the fix must
// never mask.
func TestAssessTargetStatusKeepsExactAmbiguityWhenStaleLineagesAlsoExist(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "reviewed candidate\n")
	writeApprovedTargetStatusHistory(t, repo, 2)
	gitSnapshot(t, repo, "add", "-A")
	gitSnapshot(t, repo, "commit", "-m", "deliver reviewed candidate")
	writeSnapshotFile(t, repo, "tracked.txt", "different candidate\n")
	_, _ = startReviewingCompactAuthority(t, repo, newCompactTestState(t, repo, "review-exact-b"))
	_, _ = startReviewingCompactAuthority(t, repo, newCompactTestState(t, repo, "review-exact-a"))

	got, err := AssessTargetStatus(context.Background(), repo, targetStatusCurrentChangesRequest())
	if err != nil {
		t.Fatal(err)
	}
	if got.Applicability != TargetApplicabilityAmbiguous || got.Action != TargetStatusActionSelectLineage ||
		!equalStrings(got.CandidateLineageIDs, []string{"review-exact-a", "review-exact-b"}) {
		t.Fatalf("mixed exact+stale status = %#v", got)
	}
}

func TestAssessTargetStatusKeepsCompactExclusiveBesideSameLineageLegacyAuthority(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "review-collision")
	state, _ = startReviewingCompactAuthority(t, repo, state)
	storeLegacyReviewingStatus(t, repo, state.LineageID, state.InitialSnapshot)

	got, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: state.LineageID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Applicability != TargetApplicabilityCurrent || got.AuthorityVersion != AuthorityVersionCompact ||
		got.LineageID != state.LineageID {
		t.Fatalf("ordinary status did not keep compact authority exclusive: %#v", got)
	}
}

func storeLegacyReviewingStatus(t *testing.T, repo, lineage string, snapshot Snapshot) {
	t.Helper()
	risk, lines, err := (SnapshotBuilder{Repo: repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses := []string{}
	if risk == RiskMedium {
		lenses = []string{LensReliability}
	} else if risk == RiskHigh {
		lenses = append([]string(nil), supportedLenses...)
	}
	tx, err := NewTransaction(Start{LineageID: lineage, Mode: ModeOrdinaryBounded, Generation: 1, Snapshot: snapshot, PolicyHash: hash("1"), RiskLevel: risk, SelectedLenses: lenses, OriginalChangedLines: &lines})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	store, err := AuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append("", Record{Operation: "review/start", Transaction: *tx}); err != nil {
		t.Fatal(err)
	}
}

func TestTargetStatusReopenedLensesAdvertiseOnlyQuarantinedRecapture(t *testing.T) {
	repo, store, state := highRiskCaptureAuthority(t, "status-reopen-recapture")
	for order := range state.SelectedLenses {
		captureCompactLens(t, store, state, order)
	}
	record := requireCompactRoleCount(t, store, 4)
	if err := store.CaptureAdmittedRefuterResult(t.Context(), CompactAdmittedRefuterResultRequest{ExpectedRevision: record.State.CapturePhaseRevision, TargetIdentity: record.State.InitialSnapshot.Identity, RequestHash: hash("d"), Payload: []byte(`{"results":[]}`)}); err != nil {
		t.Fatal(err)
	}
	beforeReopen := requireCompactRoleCount(t, store, 5)
	record = reopenOneCapturedLens(t, repo, store, beforeReopen, LensRisk)
	for order, lens := range record.State.SelectedLenses {
		entry, captured, lookupErr := record.State.ActiveAdmittedLensResult(order)
		if lookupErr != nil {
			t.Fatalf("status active capture slot %q at order %d: %v", lens, order, lookupErr)
		}
		if captured != (lens != LensRisk) {
			t.Fatalf("status capture slot %q at order %d = %t, want %t", lens, order, captured, lens != LensRisk)
		}
		if captured && entry.CapturePhaseRevision != beforeReopen.State.CapturePhaseRevision {
			t.Fatalf("retained active slot %q phase = %q, want immutable phase %q", lens, entry.CapturePhaseRevision, beforeReopen.State.CapturePhaseRevision)
		}
	}
	status, err := AssessTargetStatus(t.Context(), repo, TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: record.State.LineageID})
	if err != nil || status.Applicability != TargetApplicabilityCurrent || status.State != StateReviewing {
		t.Fatalf("reopened reviewing status = %#v, %v", status, err)
	}
}

func TestTargetStatusResultHasNoAuthorityPathFields(t *testing.T) {
	typeOf := reflect.TypeOf(TargetStatusResult{})
	for index := 0; index < typeOf.NumField(); index++ {
		if strings.Contains(strings.ToLower(typeOf.Field(index).Name), "path") || strings.Contains(strings.ToLower(typeOf.Field(index).Name), "dir") {
			t.Fatalf("authority path-like field exposed: %s", typeOf.Field(index).Name)
		}
	}
	_ = filepath.Separator
}

func TestCompactAuthorityLockFailuresAreOperational(t *testing.T) {
	for _, err := range []error{
		&AuthorityLockTimeoutError{Timeout: 2 * time.Second},
		&AuthorityLockCancelledError{Cause: context.Canceled},
	} {
		if !IsCompactAuthorityOperationalFailure(err) {
			t.Fatalf("authority lock failure classified as semantic corruption: %T", err)
		}
	}
}

// TestAssessTargetStatusTreatsChangedCanonicalRejectedValidatorAsUnrelated
// proves a candidate-bound rejected validator does not govern a later normal
// candidate edit.
func TestAssessTargetStatusTreatsChangedCanonicalRejectedValidatorAsUnrelated(t *testing.T) {
	repo, state := persistCanonicalRejectedValidatorAuthority(t, "status-canonical-rejection")
	unchanged, err := AssessTargetStatus(context.Background(), repo, targetStatusCurrentChangesRequest())
	if err != nil || unchanged.Applicability != TargetApplicabilityCurrent || unchanged.Action != TargetStatusActionStop {
		t.Fatalf("unchanged canonical rejected validator status = %#v, err=%v", unchanged, err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "normal candidate after rejection\n")

	status, err := AssessTargetStatus(context.Background(), repo, targetStatusCurrentChangesRequest())
	if err != nil || status.Applicability != TargetApplicabilityUnrelated || status.Action != TargetStatusActionStart ||
		status.ActionDisposition != "" || status.LineageID != "" || state.State != StateEscalated {
		t.Fatalf("changed canonical rejected validator status = %#v, state=%#v, err=%v", status, state, err)
	}
}

func persistCanonicalRejectedValidatorAuthority(t *testing.T, lineage string) (string, CompactState) {
	t.Helper()
	repo, state, revision, store := targetedValidationRequestFixture(t, lineage, true)
	request, err := BuildTargetedValidationRequest(context.Background(), repo, state, revision)
	if err != nil {
		t.Fatal(err)
	}
	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetFixDiff, Projection: state.InitialSnapshot.Projection, BaseRef: state.CurrentSnapshot.CandidateTree,
		IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	evidence := CompactTargetedValidatorEvidence{
		TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: request.CorrectionTargetIdentity,
		OriginalCriteria:     CompactTargetedValidatorCheckEvidence{Evidence: []string{"the corrected candidate still fails the original criterion"}},
		CorrectionRegression: CompactTargetedValidatorCheckEvidence{Passed: true, Evidence: []string{"the correction introduced no unrelated regression"}},
		FollowUps:            []FollowUp{},
	}
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := ScopedValidationResult{
		LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: evidence.FollowUps,
		OriginalCriteria:              ValidationCheck{EvidenceHash: compactTargetedValidatorEvidenceHashForDomain("original-criteria", evidence.OriginalCriteria), FixDeltaHash: fixHash},
		CorrectionRegression:          ValidationCheck{EvidenceHash: compactTargetedValidatorEvidenceHashForDomain("correction-regression", evidence.CorrectionRegression), FixDeltaHash: fixHash, Passed: true},
		TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: request.CorrectionTargetIdentity,
	}
	payload, err := json.Marshal(compactAdmittedTargetedValidatorValue{Outcome: "failed", Evidence: &evidence})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CaptureAdmittedTargetedValidatorResult(context.Background(), CompactAdmittedTargetedValidatorResultRequest{
		ExpectedRequest: request, Payload: payload, Evidence: &evidence, Validation: &validation,
		Complete: func(next *CompactState) error { return next.CompleteCorrectionVerification(fix, 1, validation) },
	}); err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return repo, record.State
}

// TestRecoveryStatusNamesTheDispositionRecoveryAccepts pins issue #1469 Case B:
// every `recover` recommendation must name the --disposition that
// ValidateCompactRecovery actually accepts, so an operator never has to guess
// and never lands on the "recovery requires an invalidated predecessor" dead
// end. It does not change which action any state is routed to.
func TestRecoveryStatusNamesTheDispositionRecoveryAccepts(t *testing.T) {
	requireSnapshotGit(t)
	t.Run("correction-required genesis expansion recovers as scope_changed", func(t *testing.T) {
		repo, predecessor, _, _ := correctionScopeRecoveryFixture(t, "disposition-expansion")
		writeSnapshotFile(t, repo, "process_helper.go", "package processhelper\n")
		target := Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{"process_helper.go"}}
		status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: target, LineageID: predecessor.LineageID})
		if err != nil || status.State != StateCorrectionRequired || status.Action != TargetStatusActionRecover ||
			status.ActionDisposition != RecoveryScopeChanged {
			t.Fatalf("expansion status = %#v, %v", status, err)
		}
	})
	t.Run("correction-required pure contraction recovers as scope_changed", func(t *testing.T) {
		repo, predecessor, _, _ := correctionContractionRecoveryFixture(t, "disposition-contraction")
		writeSnapshotFile(t, repo, "deleted.txt", "delete me\n")
		status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
			Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: predecessor.LineageID,
		})
		if err != nil || status.State != StateCorrectionRequired || status.Action != TargetStatusActionRecover ||
			status.ActionDisposition != RecoveryScopeChanged {
			t.Fatalf("contraction status = %#v, %v", status, err)
		}
	})
	t.Run("changed historical failed validator starts unrelated review", func(t *testing.T) {
		repo, _, _ := persistHistoricalFailedValidatorAuthority(t, "disposition-historical")
		writeSnapshotFile(t, repo, "tracked.txt", "changed normal candidate\n")
		status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
			Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}},
		})
		if err != nil || status.Applicability != TargetApplicabilityUnrelated || status.Action != TargetStatusActionStart ||
			status.ActionDisposition != "" || status.LineageID != "" {
			t.Fatalf("changed historical failed validator status = %#v, %v", status, err)
		}
	})
	t.Run("invalidated authority recovers as invalidated", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
		state := newCompactTestState(t, repo, "disposition-invalidated")
		state, store := startReviewingCompactAuthority(t, repo, state)
		record, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		if err := state.Invalidate("scope drift"); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Replace(record.Revision, "review/invalidate", state); err != nil {
			t.Fatal(err)
		}
		status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
			Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: state.LineageID,
		})
		if err != nil || status.State != StateInvalidated || status.Action != TargetStatusActionRecover ||
			status.ActionDisposition != RecoveryInvalidated {
			t.Fatalf("invalidated status = %#v, %v", status, err)
		}
	})
}

// TestNonRecoveryStatusOmitsDisposition proves the field stays unset wherever
// no recovery disposition applies, so it can never be read as authorization.
// TestCorrectionRecoveryDispositionMirrorsRecoveryRules keeps the guidance
// helper aligned with the predicates that authorize each recovery. If
// classifyCompactCorrectionTarget says recover, a disposition must be named.
func TestCorrectionRecoveryDispositionMirrorsRecoveryRules(t *testing.T) {
	requireSnapshotGit(t)
	for _, tt := range []struct {
		name    string
		fixture func(*testing.T, string) (string, CompactState, CompactStore, CompactRecord)
		mutate  func(*testing.T, string)
		want    RecoveryDisposition
	}{
		{name: "expansion", fixture: correctionScopeRecoveryFixture, want: RecoveryScopeChanged,
			mutate: func(t *testing.T, repo string) {
				writeSnapshotFile(t, repo, "process_helper.go", "package processhelper\n")
			}},
		{name: "contraction", fixture: correctionContractionRecoveryFixture, want: RecoveryScopeChanged,
			mutate: func(t *testing.T, repo string) { writeSnapshotFile(t, repo, "deleted.txt", "delete me\n") }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo, predecessor, _, _ := tt.fixture(t, "mirror-"+tt.name)
			tt.mutate(t, repo)
			untracked := []string{}
			if tt.name == "expansion" {
				untracked = []string{"process_helper.go"}
			}
			live, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: untracked})
			if err != nil {
				t.Fatal(err)
			}
			requested := predecessor
			requested.InitialSnapshot = live
			if claim, err := classifyCompactCorrectionTarget(context.Background(), repo, predecessor, requested, false); err != nil || claim != compactCorrectionTargetRecover {
				t.Fatalf("classification = %v, %v, want recover", claim, err)
			}
			if got := compactCorrectionRecoveryDisposition(predecessor, live); got != tt.want {
				t.Fatalf("disposition = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAssessTargetStatusTreatsCommittedCorrectedIntendedHistoryAsUnrelated(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	state := correctedCompactTestStateWithIntended(t, repo, "status-committed-correction", []string{"new.txt"})
	store := writeTerminalTargetStatusAuthority(t, repo, state)

	gitSnapshot(t, repo, "add", "-A")
	gitSnapshot(t, repo, "commit", "-m", "deliver corrected candidate")
	if headTree := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD^{tree}")); headTree != state.CurrentSnapshot.CandidateTree {
		t.Fatalf("delivered HEAD tree = %q, want reviewed candidate %q", headTree, state.CurrentSnapshot.CandidateTree)
	}

	authorityRoot, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	before := authorityBytes(t, authorityRoot)
	stateBefore, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	got, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Applicability != TargetApplicabilityUnrelated || got.Action != TargetStatusActionStart || got.LineageID != "" {
		t.Fatalf("committed corrected history status = %#v", got)
	}
	stateAfter, stateErr := os.ReadFile(store.StatePath())
	if stateErr != nil || !bytes.Equal(stateBefore, stateAfter) {
		t.Fatalf("status changed authority state bytes: stateErr=%v", stateErr)
	}
	if after := authorityBytes(t, authorityRoot); !reflect.DeepEqual(before, after) {
		t.Fatalf("status mutated committed corrected history: before=%v after=%v", before, after)
	}
}

func TestAssessTargetStatusTreatsMissingHistoricalIntendedPathAsNonApplicable(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "historical.txt", "reviewed historical content\n")
	state := receiptFreeLastEventClosedCompactState(t, repo, "status-missing-intended", []string{"historical.txt"})
	writeTerminalTargetStatusAuthority(t, repo, state)
	gitSnapshot(t, repo, "add", "-A")
	gitSnapshot(t, repo, "commit", "-m", "deliver reviewed candidate")
	if headTree := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD^{tree}")); headTree != state.CurrentSnapshot.CandidateTree {
		t.Fatalf("delivered HEAD tree = %q, want reviewed candidate %q", headTree, state.CurrentSnapshot.CandidateTree)
	}
	gitSnapshot(t, repo, "rm", "historical.txt")
	gitSnapshot(t, repo, "commit", "-m", "remove delivered historical path")

	authorityRoot, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	before := authorityBytes(t, authorityRoot)

	for _, tt := range []struct {
		name   string
		target Target
		setup  func()
	}{
		{name: "clean follow-up", target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}},
		{name: "disjoint follow-up", target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{"next.txt"}}, setup: func() {
			writeSnapshotFile(t, repo, "next.txt", "next slice\n")
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}
			got, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: tt.target})
			if err != nil {
				t.Fatal(err)
			}
			if got.Applicability != TargetApplicabilityUnrelated || got.Action != TargetStatusActionStart || got.LineageID != "" {
				t.Fatalf("missing historical intended path status = %#v", got)
			}
		})
	}
	if after := authorityBytes(t, authorityRoot); !reflect.DeepEqual(before, after) {
		t.Fatalf("status mutated missing-path history: before=%v after=%v", before, after)
	}
}

func TestAssessTargetStatusGitSubprocessCountIsHistoryIndependent(t *testing.T) {
	requireSnapshotGit(t)
	originalCommand := gitCommandContext
	t.Cleanup(func() { gitCommandContext = originalCommand })

	sizes := []int{1, 10, 100}
	counts := make([]int, 0, len(sizes))
	for _, size := range sizes {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "reviewed candidate\n")
		writeApprovedTargetStatusHistory(t, repo, size)
		gitSnapshot(t, repo, "add", "-A")
		gitSnapshot(t, repo, "commit", "-m", "deliver reviewed candidate")
		writeSnapshotFile(t, repo, "tracked.txt", "related follow-up\n")

		count := 0
		gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
			count++
			return originalCommand(ctx, name, args...)
		}
		_, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
			Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}},
		})
		gitCommandContext = originalCommand
		if err != nil {
			t.Fatalf("AssessTargetStatus() for %d histories: %v", size, err)
		}
		counts = append(counts, count)
	}

	t.Logf("AssessTargetStatus Git subprocess counts for N=1/10/100: %v", counts)
	if counts[0] != counts[1] || counts[1] != counts[2] || counts[0] > 20 {
		t.Fatalf("Git subprocess counts = %v, want equal request-scoped counts no greater than 20", counts)
	}
}

func TestAssessTargetStatusPropagatesOperationalAuthorityFailures(t *testing.T) {
	requireSnapshotGit(t)

	t.Run("git exit 73", func(t *testing.T) {
		repo := targetStatusOperationalFailureFixture(t, "status-git-exit")
		writeSnapshotFile(t, repo, "tracked.txt", "drifted candidate\n")
		originalCommand := gitCommandContext
		t.Cleanup(func() { gitCommandContext = originalCommand })
		t.Setenv("GENTLE_AI_TARGET_STATUS_GIT_HELPER", "exit73")
		gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
			if gitInvocationContains(args, "--git-common-dir") {
				return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTargetStatusGitHelperProcess$", "--")
			}
			return originalCommand(ctx, name, args...)
		}
		request := targetStatusCurrentChangesRequest()
		request.LineageID = "status-git-exit"
		got, err := AssessTargetStatus(context.Background(), repo, request)
		var commandErr *GitCommandError
		if !errors.As(err, &commandErr) || commandErr.ExitCode != 73 || got.Applicability == TargetApplicabilityCorrupted {
			t.Fatalf("Git exit status = %#v, error = %T %v", got, err, err)
		}
	})

	t.Run("git timeout", func(t *testing.T) {
		repo := targetStatusOperationalFailureFixture(t, "status-git-timeout")
		originalCommand, originalTimeout, originalWait := gitCommandContext, LocalGitCommandTimeout, gitCommandWaitDelay
		t.Cleanup(func() {
			gitCommandContext, LocalGitCommandTimeout, gitCommandWaitDelay = originalCommand, originalTimeout, originalWait
		})
		t.Setenv("GENTLE_AI_TARGET_STATUS_GIT_HELPER", "sleep")
		LocalGitCommandTimeout, gitCommandWaitDelay = 25*time.Millisecond, 10*time.Millisecond
		gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
			if gitInvocationContains(args, "--git-common-dir") {
				return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTargetStatusGitHelperProcess$", "--")
			}
			return originalCommand(ctx, name, args...)
		}
		got, err := AssessTargetStatus(context.Background(), repo, targetStatusCurrentChangesRequest())
		var timeout *GitCommandTimeoutError
		if !errors.As(err, &timeout) || got.Applicability == TargetApplicabilityCorrupted {
			t.Fatalf("Git timeout status = %#v, error = %T %v", got, err, err)
		}
	})

	t.Run("git process control", func(t *testing.T) {
		repo := targetStatusOperationalFailureFixture(t, "status-git-control")
		originalStarter := gitProcessTreeStarter
		t.Cleanup(func() { gitProcessTreeStarter = originalStarter })
		cause := errors.New("job object assignment denied")
		gitProcessTreeStarter = func(command *exec.Cmd) (func() error, error) {
			if gitInvocationContains(command.Args, "--git-common-dir") {
				if err := command.Start(); err != nil {
					return nil, err
				}
				return func() error { return command.Process.Kill() }, cause
			}
			return originalStarter(command)
		}
		got, err := AssessTargetStatus(context.Background(), repo, targetStatusCurrentChangesRequest())
		var control *GitProcessControlError
		if !errors.As(err, &control) || !errors.Is(err, cause) || got.Applicability == TargetApplicabilityCorrupted {
			t.Fatalf("Git process-control status = %#v, error = %T %v", got, err, err)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		repo := targetStatusOperationalFailureFixture(t, "status-context-cancel")
		request := targetStatusCurrentChangesRequest()
		live, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), request.Target)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		got, err := assessTargetStatusSnapshot(ctx, repo, request, live)
		if !errors.Is(err, context.Canceled) || got.Applicability == TargetApplicabilityCorrupted {
			t.Fatalf("canceled status = %#v, error = %T %v", got, err, err)
		}
	})

	t.Run("context deadline", func(t *testing.T) {
		repo := targetStatusOperationalFailureFixture(t, "status-context-deadline")
		request := targetStatusCurrentChangesRequest()
		live, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), request.Target)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		got, err := assessTargetStatusSnapshot(ctx, repo, request, live)
		if !errors.Is(err, context.DeadlineExceeded) || got.Applicability == TargetApplicabilityCorrupted {
			t.Fatalf("expired status = %#v, error = %T %v", got, err, err)
		}
	})

	t.Run("non-not-exist filesystem failure", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
		_, store := startReviewingCompactAuthority(t, repo, newCompactTestState(t, repo, "status-filesystem-error"))
		if err := os.Remove(store.StatePath()); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(store.StatePath(), 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := AssessTargetStatus(context.Background(), repo, targetStatusCurrentChangesRequest())
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) || errors.Is(err, os.ErrNotExist) || got.Applicability == TargetApplicabilityCorrupted {
			t.Fatalf("filesystem status = %#v, error = %T %v", got, err, err)
		}
	})
}

func TestExplicitReviewingStatusRejectsSemanticAndIneligibleFrozenCandidates(t *testing.T) {
	t.Run("fully occupied drifted candidate stops", func(t *testing.T) {
		fixture := newCompactReviewerCaptureFixture(t, "frozen-complete")
		if _, err := fixture.store.CaptureAdmittedReviewerResult(context.Background(), fixture.request); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(fixture.store.repo, "internal", "a.go"), []byte("package internal\n\nfunc Value() int { return 3 }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		status, err := AssessTargetStatus(context.Background(), fixture.store.repo, TargetStatusRequest{
			Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: fixture.state.LineageID,
		})
		if err != nil || status.Applicability != TargetApplicabilityCurrent || status.LineageID != fixture.state.LineageID ||
			status.Action != TargetStatusActionStop || status.Replayability != ReplayabilityManualActionRequired {
			t.Fatalf("fully occupied frozen status = %#v, %v", status, err)
		}
	})
	t.Run("semantic frozen evidence", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
		_, store := startReviewingCompactAuthority(t, repo, newCompactTestState(t, repo, "frozen-semantic"))
		record, _ := store.Load()
		record.State.InitialSnapshot.Paths = []string{"missing.txt"}
		eligible, _, err := explicitReviewingCompactCandidate(context.Background(), repo, targetStatusCandidate{compact: &record})
		status, statusErr := targetStatusFailure(TargetStatusResult{}, err)
		if eligible || statusErr != nil || status.Applicability != TargetApplicabilityCorrupted || status.Action != TargetStatusActionRepairAuthority {
			t.Fatalf("semantic frozen evidence = eligible %v status %#v error %v", eligible, status, statusErr)
		}
	})

	t.Run("non-reviewing selected lineage", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
		_, store := startReviewingCompactAuthority(t, repo, newCompactTestState(t, repo, "frozen-not-reviewing"))
		record, _ := store.Load()
		record.State.State = StateInvalidated
		if eligible, _, err := explicitReviewingCompactCandidate(context.Background(), repo, targetStatusCandidate{compact: &record}); eligible || err != nil {
			t.Fatalf("non-reviewing candidate = eligible %v error %v", eligible, err)
		}
	})

}

func TestAssessTargetStatusBoundsUnstableCompactAuthorityReads(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "status-authority-churn")
	state, store := startReviewingCompactAuthority(t, repo, state)
	_, originalPayload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	alternate := state
	alternate.ReviewerContextLevel = ReviewerContextLevelProviderCommand
	_, alternatePayload, err := makeCompactRecord(alternate)
	if err != nil {
		t.Fatal(err)
	}
	originalHook := targetStatusCompactAuthorityReadHook
	t.Cleanup(func() { targetStatusCompactAuthorityReadHook = originalHook })
	writes := 0
	var hookErr error
	targetStatusCompactAuthorityReadHook = func(lineage, phase string, _ int) {
		if lineage != state.LineageID || phase != "after-state" || hookErr != nil {
			return
		}
		writes++
		payload := alternatePayload
		if writes%2 == 0 {
			payload = originalPayload
		}
		hookErr = os.WriteFile(store.StatePath(), payload, 0o644)
	}

	got, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: state.LineageID,
	})
	if hookErr != nil {
		t.Fatal(hookErr)
	}
	if !errors.Is(err, ErrConcurrentUpdate) || got.Applicability == TargetApplicabilityCorrupted {
		t.Fatalf("unstable authority status = %#v, error = %T %v", got, err, err)
	}
	if writes != 3 {
		t.Fatalf("unstable authority attempts = %d, want bounded 3", writes)
	}
}

func TestAssessTargetStatusStopsStableReadOnContextCancellation(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "status-authority-cancel")
	state, _ = startReviewingCompactAuthority(t, repo, state)
	originalHook := targetStatusCompactAuthorityReadHook
	t.Cleanup(func() { targetStatusCompactAuthorityReadHook = originalHook })
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	targetStatusCompactAuthorityReadHook = func(lineage, phase string, _ int) {
		if lineage == state.LineageID && phase == "after-state" {
			calls++
			cancel()
		}
	}

	got, err := AssessTargetStatus(ctx, repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: state.LineageID,
	})
	if !errors.Is(err, context.Canceled) || got.Applicability == TargetApplicabilityCorrupted {
		t.Fatalf("canceled coherent read status = %#v, error = %T %v", got, err, err)
	}
	if calls != 1 {
		t.Fatalf("canceled coherent read attempts = %d, want 1", calls)
	}
}

func TestAssessTargetStatusStopsStableReadOnDeadline(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "status-authority-deadline")
	state, _ = startReviewingCompactAuthority(t, repo, state)
	originalHook := targetStatusCompactAuthorityReadHook
	t.Cleanup(func() { targetStatusCompactAuthorityReadHook = originalHook })
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	calls := 0
	targetStatusCompactAuthorityReadHook = func(lineage, phase string, _ int) {
		if lineage == state.LineageID && phase == "after-state" {
			calls++
			<-ctx.Done()
		}
	}

	got, err := AssessTargetStatus(ctx, repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: state.LineageID,
	})
	if !errors.Is(err, context.DeadlineExceeded) || got.Applicability == TargetApplicabilityCorrupted {
		t.Fatalf("deadline coherent read status = %#v, error = %T %v", got, err, err)
	}
	// The same deadline covers Git-backed snapshot setup, so a slow runner may
	// exhaust it before the first authority observation. No second attempt may start.
	if calls > 1 {
		t.Fatalf("deadline coherent read attempts = %d, want at most 1", calls)
	}
}

func TestTargetStatusGitHelperProcess(t *testing.T) {
	switch os.Getenv("GENTLE_AI_TARGET_STATUS_GIT_HELPER") {
	case "exit73":
		os.Exit(73)
	case "sleep":
		time.Sleep(10 * time.Second)
	}
}

func persistEscalatedRecoveryFixture(t *testing.T, repo string, state CompactState) (CompactStore, CompactRecord) {
	t.Helper()
	if state.State != StateEscalated {
		t.Fatalf("escalated recovery fixture state = %q, want %q", state.State, StateEscalated)
	}
	if state.LineageID == "" {
		t.Fatal("escalated recovery fixture requires a lineage ID")
	}
	store := writeTerminalTargetStatusAuthority(t, repo, state)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return store, record
}

func recoveredEvidenceSuccessor(t *testing.T, repo string, predecessor CompactState, lineage string) CompactState {
	t.Helper()
	successor := newCompactTestState(t, repo, lineage)
	successor.Generation = predecessor.Generation + 1
	return successor
}

func writeTerminalTargetStatusAuthority(t *testing.T, repo string, state CompactState) CompactStore {
	t.Helper()
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	_, payload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	return store
}

func writeApprovedTargetStatusHistory(t *testing.T, repo string, count int) {
	t.Helper()
	for index := 0; index < count; index++ {
		lineage := fmt.Sprintf("status-history-%03d", index)
		state := newCompactTestState(t, repo, lineage)
		state, store := startReviewingCompactAuthority(t, repo, state)
		started, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		results := make([]LensResult, len(state.SelectedLenses))
		for index, lens := range state.SelectedLenses {
			results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
		}
		state, started = captureAndCompleteCompactReview(t, store, state, CompactReviewInput{LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{}})
		if false {
			t.Fatal(err)
		}
		if err := state.CloseCleanReviewOnLastEvent(); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Replace(started.Revision, "review/complete-review", state); err != nil {
			t.Fatal(err)
		}
	}
}

func targetStatusOperationalFailureFixture(t *testing.T, lineage string) string {
	t.Helper()
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	_, _ = startReviewingCompactAuthority(t, repo, newCompactTestState(t, repo, lineage))
	return repo
}

func targetStatusCurrentChangesRequest() TargetStatusRequest {
	return TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}}
}

func gitInvocationContains(args []string, sequence ...string) bool {
	if len(sequence) == 0 || len(args) < len(sequence) {
		return false
	}
	for start := 0; start <= len(args)-len(sequence); start++ {
		match := true
		for index := range sequence {
			match = match && args[start+index] == sequence[index]
		}
		if match {
			return true
		}
	}
	return false
}
