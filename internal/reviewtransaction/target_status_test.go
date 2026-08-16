package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
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

func TestAssessTargetStatusDerivesReceiptTruthWithoutMutation(t *testing.T) {
	requireSnapshotGit(t)
	tests := []struct {
		name              string
		prepare           func(t *testing.T, repo, lineage string) CompactStore
		wantApplicability TargetApplicability
		wantAction        TargetStatusAction
		wantReplay        Replayability
		wantIdentity      bool
	}{
		{
			name: "fresh reviewing receipt is expected missing",
			prepare: func(t *testing.T, repo, lineage string) CompactStore {
				return storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, lineage))
			},
			wantApplicability: TargetApplicabilityCurrent,
			wantAction:        TargetStatusActionFinalize,
			wantReplay:        ReplayabilityNotReplayable,
		},
		{
			name: "approved derived receipt is published",
			prepare: func(t *testing.T, repo, lineage string) CompactStore {
				gitSnapshot(t, repo, "mv", "tracked.txt", "tracked.md")
				gitSnapshot(t, repo, "commit", "-am", "low-risk base")
				writeSnapshotFile(t, repo, "tracked.md", "next candidate\n")
				_, store, _ := approvedCompactCurrentChangesFixture(t, repo, lineage, []string{})
				payload, err := os.ReadFile(store.ReceiptPath())
				if err != nil {
					t.Fatal(err)
				}
				payload = bytes.Replace(payload, []byte(`"selected_lenses": []`), []byte(`"selected_lenses": null`), 1)
				if err := os.WriteFile(store.ReceiptPath(), payload, 0o644); err != nil {
					t.Fatal(err)
				}
				return store
			},
			wantApplicability: TargetApplicabilityCurrent,
			wantAction:        TargetStatusActionValidate,
			wantReplay:        ReplayabilityNotReplayable,
			wantIdentity:      true,
		},
		{
			name: "approved authority with absent receipt is replayable",
			prepare: func(t *testing.T, repo, lineage string) CompactStore {
				_, store, _ := approvedCompactCurrentChangesFixture(t, repo, lineage, []string{})
				if err := os.Remove(store.ReceiptPath()); err != nil {
					t.Fatal(err)
				}
				return store
			},
			wantApplicability: TargetApplicabilityCurrent,
			wantAction:        TargetStatusActionFinalize,
			wantReplay:        ReplayabilityExactReplaySafe,
		},
		{
			name: "wrong or corrupt receipt fails closed",
			prepare: func(t *testing.T, repo, lineage string) CompactStore {
				_, store, _ := approvedCompactCurrentChangesFixture(t, repo, lineage, []string{})
				if err := os.WriteFile(store.ReceiptPath(), []byte("{\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return store
			},
			wantApplicability: TargetApplicabilityCorrupted,
			wantAction:        TargetStatusActionRepairAuthority,
			wantReplay:        ReplayabilityManualActionRequired,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
			lineage := "receipt-truth-" + strings.ReplaceAll(strings.Fields(tt.name)[0], "_", "-")
			store := tt.prepare(t, repo, lineage)
			authorityRoot, _, err := reviewAuthorityRoot(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			before := authorityBytes(t, authorityRoot)
			request := TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: lineage}
			first, err := AssessTargetStatus(context.Background(), repo, request)
			if err != nil {
				t.Fatal(err)
			}
			second, err := AssessTargetStatus(context.Background(), repo, request)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) || first.Applicability != tt.wantApplicability || first.Action != tt.wantAction || first.Replayability != tt.wantReplay {
				t.Fatalf("receipt status = %#v, second %#v", first, second)
			}
			if tt.wantIdentity {
				payload, err := os.ReadFile(store.ReceiptPath())
				if err != nil {
					t.Fatal(err)
				}
				sum := sha256.Sum256(payload)
				want := "sha256:" + hex.EncodeToString(sum[:])
				if first.ReceiptIdentity != want {
					t.Fatalf("receipt identity = %q, want %q", first.ReceiptIdentity, want)
				}
			} else if first.ReceiptIdentity != "" {
				t.Fatalf("unsafe receipt identity = %q", first.ReceiptIdentity)
			}
			if after := authorityBytes(t, authorityRoot); !reflect.DeepEqual(before, after) {
				t.Fatalf("receipt status mutated authority: before=%v after=%v", before, after)
			}
		})
	}
}

func TestAssessTargetStatusClassifiesAllApplicabilityStates(t *testing.T) {
	requireSnapshotGit(t)
	request := TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}}

	t.Run("current target fresh start", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
		state := newCompactTestState(t, repo, "review-current")
		store := storeCompactStartAuthority(t, repo, state)
		record, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}

		got, err := AssessTargetStatus(context.Background(), repo, request)
		if err != nil {
			t.Fatal(err)
		}
		if got.Applicability != TargetApplicabilityCurrent || got.State != StateReviewing || got.Action != TargetStatusActionFinalize {
			t.Fatalf("status = %#v", got)
		}
		if got.LineageID != state.LineageID || got.Generation != state.Generation || got.Revision != record.Revision || got.ReceiptIdentity != "" {
			t.Fatalf("authority identity = %#v", got)
		}
		if got.OriginalChangedLines != state.OriginalChangedLines || got.Tier != state.RiskLevel || got.CorrectionBudget != state.CorrectionBudget {
			t.Fatalf("frozen review inputs = %#v", got)
		}
		if got.TargetIdentity != state.InitialSnapshot.Identity || got.Projection.CurrentCandidateTree != state.CurrentSnapshot.CandidateTree {
			t.Fatalf("projection = %#v", got.Projection)
		}
		if _, err := os.Stat(store.ReceiptPath()); !os.IsNotExist(err) {
			t.Fatalf("fresh START receipt stat error = %v, want not-exist", err)
		}
	})

	t.Run("unrelated", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "first candidate\n")
		storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, "review-old"))
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
		unrelated := storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, "review-b"))
		storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, "review-a"))
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
		store := storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, "review-corrupt"))
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

func TestAssessTargetStatusRecognizesAuthorizedCorrection(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	state := newCompactTestState(t, repo, "review-correction-resume")
	store := storeCompactStartAuthority(t, repo, state)
	record, _ := store.Load()
	finding := Finding{ID: "R3-001", Lens: "reliability", Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "wrong value", ProofRefs: []string{"differential failure"}}
	if err := state.CompleteReview(CompactReviewInput{LensResults: []LensResult{{Lens: LensReliability, Findings: []Finding{finding}, Evidence: []string{"reviewed"}}}, Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk"}}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	revision, _ := store.Replace(record.Revision, "review/complete-review", state)
	if err := state.BeginCorrection(2); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/begin-fix", state); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfixed\n")
	got, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: state.LineageID})
	if err != nil || got.Applicability != TargetApplicabilityCurrent || got.State != StateCorrectionRequired || got.Action != TargetStatusActionFinalize {
		t.Fatalf("authorized correction status = %#v, err = %v", got, err)
	}
}

func TestHistoricalFailedValidatorRequiresChangedTargetRecovery(t *testing.T) {
	repo, state, record, before := historicalFailedValidatorFixture(t, "historical-status")
	target := Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}
	requested := newCompactTestState(t, repo, "historical-status-new")
	started, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
	if err != nil || started.Action != CompactStartBlocked || started.Record.Revision != record.Revision {
		t.Fatalf("same-target historical START = %#v, %v", started, err)
	}
	status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: target, LineageID: state.LineageID})
	if err != nil || status.Applicability != TargetApplicabilityCurrent || status.Action != TargetStatusActionStop ||
		status.Replayability != ReplayabilityManualActionRequired || status.LineageID != state.LineageID || status.Revision != record.Revision {
		t.Fatalf("same-target historical status = %#v, %v", status, err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "changed recovery target\n")
	requested = newCompactTestState(t, repo, "historical-status-changed")
	started, err = StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
	status, statusErr := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: target, LineageID: state.LineageID})
	if err != nil || statusErr != nil || started.Action != CompactStartRecover || status.Action != TargetStatusActionRecover || status.Replayability != ReplayabilityManualActionRequired {
		t.Fatalf("changed-target recovery: START=%#v status=%#v errors=%v/%v", started, status, err, statusErr)
	}
	store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	after, _ := os.ReadFile(store.StatePath())
	if !bytes.Equal(before, after) {
		t.Fatal("START/status migrated historical authority")
	}

	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfixed\n")
	forecast := 1
	state.ProposedCorrectionLines = &forecast
	record, before, _ = makeCompactRecord(state)
	if err := os.WriteFile(store.StatePath(), before, 0o644); err != nil {
		t.Fatal(err)
	}
	status, statusErr = AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: target, LineageID: state.LineageID})
	if statusErr != nil || status.Action != TargetStatusActionStop || status.Revision != record.Revision {
		t.Fatalf("same-target forecasted historical status = %#v, %v", status, statusErr)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "changed forecasted recovery target\n")
	requested = newCompactTestState(t, repo, "historical-status-forecasted")
	started, err = StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
	status, statusErr = AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: target, LineageID: state.LineageID})
	if err != nil || statusErr != nil || started.Action != CompactStartRecover || status.Action != TargetStatusActionRecover || status.ActionDisposition != RecoveryEscalated {
		t.Fatalf("changed forecasted historical recovery: START=%#v status=%#v errors=%v/%v", started, status, err, statusErr)
	}
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
// the routing dead end: an escalated authority whose original review and
// correction regression both passed, and whose target has not changed since
// escalation, is a candidate the native store accepts via the evidence-bound
// RecoveryEscalated edge. STATUS must offer that continuation instead of
// stopping the operator on a target that never changed.
func TestAccountingOnlyEscalationStatusOffersRecoveryInsteadOfDeadEndStop(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := accountingOnlyEscalatedState(t, repo, "accounting-only-status-dead-end")
	_, record := persistEscalatedRecoveryFixture(t, repo, state)

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

	successor := recoveredEvidenceSuccessor(t, repo, state, "accounting-only-status-dead-end-successor")
	const actor, reason = "maintainer@example.com", "recover accounting-only escalation with an unchanged target"
	authorization := compactRecoveryAuthorizationBinding(state.LineageID, record.Revision, successor.InitialSnapshot.Identity, actor, reason)
	recovered, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
		PredecessorLineageID: state.LineageID, ExpectedPredecessorRevision: record.Revision,
		Successor: successor, Disposition: RecoveryEscalated, Reason: reason, Actor: actor,
		MaintainerAuthorization: authorization,
	})
	if err != nil {
		t.Fatalf("evidence-bound recovery: %v", err)
	}
	if err := validateCompactRecoveryEdge(record, recovered.State); err != nil {
		t.Fatalf("directly-constructed evidence-bound escalated recovery successor did not validate: %v", err)
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
	if state.CorrectionBudget < 2 || len(state.SelectedLenses) != 1 {
		t.Fatalf("fixture risk/budget = %q/%d", state.RiskLevel, state.CorrectionBudget)
	}
	finding := Finding{
		ID: "R3-001", Lens: "reliability", Location: "tracked.txt:5", Severity: "CRITICAL",
		Claim: "candidate retains the wrong value", ProofRefs: []string{"candidate-only differential failure"},
	}
	if err := state.CompleteReview(CompactReviewInput{
		LensResults:     []LensResult{{Lens: state.SelectedLenses[0], Findings: []Finding{finding}, Evidence: []string{"reviewed exact candidate tree"}}},
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk"}},
		RefuterOutcomes: []EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
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

func TestCorrectionScopeExpansionGuidesStatusAndStartToRecovery(t *testing.T) {
	repo, predecessor, _, _ := correctionScopeRecoveryFixture(t, "review-correction-expansion")
	writeSnapshotFile(t, repo, "process_helper.go", "package processhelper\n")
	target := Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{"process_helper.go"}}
	status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: target, LineageID: predecessor.LineageID})
	if err != nil || status.Applicability != TargetApplicabilityCurrent || status.State != StateCorrectionRequired ||
		status.Action != TargetStatusActionRecover || status.Replayability != ReplayabilityManualActionRequired {
		t.Fatalf("expanded correction status = %#v, %v", status, err)
	}
	requested := newCompactStartStateForTarget(t, repo, "review-correction-expansion-new", target)
	started, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
	if err != nil || started.Action != CompactStartAction("recover") || started.Record.State.LineageID != predecessor.LineageID {
		t.Fatalf("expanded correction start = %#v, %v", started, err)
	}
	requestedStore, _ := CompactAuthoritativeStore(context.Background(), repo, requested.LineageID)
	if _, err := os.Stat(requestedStore.StatePath()); !os.IsNotExist(err) {
		t.Fatalf("start published an unauthorized successor: %v", err)
	}
}

func TestCorrectionScopeExpansionPrioritizesPendingFinalize(t *testing.T) {
	repo, predecessor, store, record := correctionScopeRecoveryFixture(t, "review-correction-pending")
	request := finalizeAttemptTestRequest(predecessor.LineageID, record.Revision, "evidence")
	request.CandidateDigest = FinalizeAttemptValueDigest("candidate", predecessor.CurrentSnapshot)
	request.RequestDigest = FinalizeAttemptRequestDigest(request)
	if _, _, err := store.BeginFinalizeAttempt(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "process_helper.go", "package processhelper\n")
	target := Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{"process_helper.go"}}
	status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: target, LineageID: predecessor.LineageID})
	if err != nil || status.Action != TargetStatusActionReconcileFinalize || status.Replayability != ReplayabilityStatusRequired {
		t.Fatalf("pending finalize with expanded correction scope = %#v, %v", status, err)
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
	storeCompactStartAuthority(t, repo, state)
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
	storeCompactStartAuthority(t, repo, compact)

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
	storeCompactStartAuthority(t, repo, compact)
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
		got.LineageID != compact.LineageID || got.Action != TargetStatusActionFinalize {
		t.Fatalf("explicit compact status = %#v", got)
	}

	unscoped := request
	unscoped.LineageID = ""
	global, err := AssessTargetStatus(context.Background(), repo, unscoped)
	if err != nil {
		t.Fatal(err)
	}
	if global.Applicability != TargetApplicabilityCorrupted || global.Action != TargetStatusActionRepairAuthority {
		t.Fatalf("unscoped invalid inventory did not fail closed: %#v", global)
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

func TestAssessTargetStatusStopsApplicableNonTerminalLegacyWithoutMutation(t *testing.T) {
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
		if got.Applicability != TargetApplicabilityCurrent || got.AuthorityVersion != AuthorityVersionLegacy ||
			got.Action != TargetStatusActionStop || got.Replayability != ReplayabilityManualActionRequired || got.State != StateReviewing {
			t.Fatalf("legacy status = %#v", got)
		}
		if after := authorityBytes(t, authorityRoot); !reflect.DeepEqual(after, before) {
			t.Fatalf("attempt %d mutated legacy authority", attempt+1)
		}
	}
}

func TestAssessTargetStatusValidatesApplicableApprovedLegacyReceiptWithoutMutation(t *testing.T) {
	requireSnapshotGit(t)
	tests := []struct {
		name              string
		mutateReceipt     func(t *testing.T, path string, receipt Receipt)
		wantApplicability TargetApplicability
		wantAction        TargetStatusAction
		wantReplay        Replayability
		wantIdentity      bool
	}{
		{
			name:              "approved receipt is present and valid",
			wantApplicability: TargetApplicabilityCurrent,
			wantAction:        TargetStatusActionValidate,
			wantReplay:        ReplayabilityNotReplayable,
			wantIdentity:      true,
		},
		{
			name: "approved receipt is missing",
			mutateReceipt: func(t *testing.T, path string, _ Receipt) {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			},
			wantApplicability: TargetApplicabilityCorrupted,
			wantAction:        TargetStatusActionRepairAuthority,
			wantReplay:        ReplayabilityManualActionRequired,
		},
		{
			name: "approved receipt is corrupt",
			mutateReceipt: func(t *testing.T, path string, _ Receipt) {
				if err := os.WriteFile(path, []byte("{\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantApplicability: TargetApplicabilityCorrupted,
			wantAction:        TargetStatusActionRepairAuthority,
			wantReplay:        ReplayabilityManualActionRequired,
		},
		{
			name: "approved receipt belongs to different authority",
			mutateReceipt: func(t *testing.T, path string, receipt Receipt) {
				receipt.PolicyHash = hash("b")
				if err := WriteReceiptAtomic(path, receipt); err != nil {
					t.Fatal(err)
				}
			},
			wantApplicability: TargetApplicabilityCorrupted,
			wantAction:        TargetStatusActionRepairAuthority,
			wantReplay:        ReplayabilityManualActionRequired,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			writeSnapshotFile(t, repo, "tracked.txt", "legacy candidate\n")
			lineage := "legacy-approved-" + strings.ReplaceAll(strings.Fields(tt.name)[2], "_", "-")
			transaction, receipt, _ := nativeGateFixture(t, repo, lineage)
			store, err := AuthoritativeStore(context.Background(), repo, lineage)
			if err != nil {
				t.Fatal(err)
			}
			appendApprovedStoreChain(t, store, transaction)
			receiptPath := filepath.Join(store.Dir, "artifacts", "receipt.json")
			if err := WriteReceiptAtomic(receiptPath, receipt); err != nil {
				t.Fatal(err)
			}
			if tt.mutateReceipt != nil {
				tt.mutateReceipt(t, receiptPath, receipt)
			}
			authorityRoot, _, err := reviewAuthorityRoot(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			before := authorityBytes(t, authorityRoot)
			request := TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: lineage}
			first, err := AssessTargetStatus(context.Background(), repo, request)
			if err != nil {
				t.Fatal(err)
			}
			second, err := AssessTargetStatus(context.Background(), repo, request)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) || first.Applicability != tt.wantApplicability ||
				first.Action != tt.wantAction || first.Replayability != tt.wantReplay {
				t.Fatalf("legacy receipt status = %#v, second %#v", first, second)
			}
			if tt.wantIdentity {
				payload, err := os.ReadFile(receiptPath)
				if err != nil {
					t.Fatal(err)
				}
				sum := sha256.Sum256(payload)
				want := "sha256:" + hex.EncodeToString(sum[:])
				if first.ReceiptIdentity != want || first.AuthorityVersion != AuthorityVersionLegacy || first.State != StateApproved {
					t.Fatalf("approved legacy receipt status = %#v, want identity %q", first, want)
				}
			} else if first.ReceiptIdentity != "" || first.Action == TargetStatusActionFinalize || first.Replayability == ReplayabilityExactReplaySafe {
				t.Fatalf("invalid legacy receipt exposed compact replay semantics: %#v", first)
			}
			if after := authorityBytes(t, authorityRoot); !reflect.DeepEqual(before, after) {
				t.Fatalf("legacy receipt status mutated authority: before=%v after=%v", before, after)
			}
		})
	}
}

func TestAssessTargetStatusMatchesCorrectedLegacyDelivery(t *testing.T) {
	repo := initSnapshotRepo(t)
	fixture := correctedBundleFixture(t, repo, "legacy-corrected-status")
	receiptPath := filepath.Join(fixture.Store.Dir, "artifacts", "receipt.json")
	if err := WriteReceiptAtomic(receiptPath, fixture.Receipt); err != nil {
		t.Fatal(err)
	}
	base := strings.SplitN(fixture.Request.Target.Revision, "..", 2)[0]
	got, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: Target{Kind: TargetBaseDiff, BaseRef: base, IntendedUntracked: fixture.Transaction.Snapshot.IntendedUntracked}, LineageID: fixture.Transaction.LineageID})
	if err != nil || got.Applicability != TargetApplicabilityCurrent || got.State != StateApproved || got.Action != TargetStatusActionValidate || got.Projection.Kind != TargetBaseDiff {
		t.Fatalf("corrected legacy status = %#v, err = %v", got, err)
	}
	writeSnapshotFile(t, repo, "delivery.txt", "mismatched delivery\n")
	gitSnapshot(t, repo, "add", "delivery.txt")
	gitSnapshot(t, repo, "commit", "-m", "mismatched delivery")
	if mismatch, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: Target{Kind: TargetBaseDiff, BaseRef: base, IntendedUntracked: fixture.Transaction.Snapshot.IntendedUntracked}, LineageID: fixture.Transaction.LineageID}); err != nil || mismatch.Applicability != TargetApplicabilityUnrelated {
		t.Fatalf("mismatched corrected legacy status = %#v, err = %v", mismatch, err)
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
	storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, "review-exact-b"))
	storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, "review-exact-a"))

	got, err := AssessTargetStatus(context.Background(), repo, targetStatusCurrentChangesRequest())
	if err != nil {
		t.Fatal(err)
	}
	if got.Applicability != TargetApplicabilityAmbiguous || got.Action != TargetStatusActionSelectLineage ||
		!equalStrings(got.CandidateLineageIDs, []string{"review-exact-a", "review-exact-b"}) {
		t.Fatalf("mixed exact+stale status = %#v", got)
	}
}

// TestAssessTargetStatusReportsAmbiguousApprovedStagedScopeExpansion is
// UNCHANGED by organic-dx Phase 3e and stays here as a negative-proof
// companion: both "staged-status-first" and "staged-status-second" are
// independently eligible for `compactApprovedStagedScopeRecovery` (a real,
// per-candidate recovery match, not the generic scope-changed classifier),
// so both land in `candidates`, not `scopeChangedCandidates`. That is 2+
// EXACTLY governing/recoverable candidates competing for the same staged
// scope-expansion recovery slot — present-tense authority damage — and must
// stay ambiguous/select_lineage exactly as before. (Initially mis-suspected
// as an instance of the stale-only bug during Phase 3e; empirically verified
// via RED/GREEN to be a structurally different, legitimate bucket, so this
// test is intentionally left untouched.)
func TestAssessTargetStatusReportsAmbiguousApprovedStagedScopeExpansion(t *testing.T) {
	repo, base, predecessor, _, _ := approvedBaseDiffScopeRecoveryFixture(t, "staged-status-first")
	peer := predecessor
	peer.LineageID = "staged-status-second"
	writeTerminalTargetStatusAuthority(t, repo, peer)
	stageStagedScopeExtra(t, repo)

	got, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: Target{
		Kind: TargetBaseWorkspaceOverlay, Projection: ProjectionStaged,
		BaseRef: base, IntendedUntracked: []string{},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Applicability != TargetApplicabilityAmbiguous || got.Action != TargetStatusActionSelectLineage ||
		!equalStrings(got.CandidateLineageIDs, []string{"staged-status-first", "staged-status-second"}) {
		t.Fatalf("ambiguous staged scope status = %#v", got)
	}
}

// TestAssessTargetStatusReportsPluralStaleStagedOverlayLineagesAsUnrelatedWithStop
// is the positive proof that organic-dx Phase 3e's fix composes correctly
// with the live overlay+staged safety stop. Unlike
// TestAssessTargetStatusReportsAmbiguousApprovedStagedScopeExpansion (which
// queries status against the SAME base the review used, matching
// compactApprovedStagedScopeRecoveryShape's per-candidate recovery match),
// this queries against a base that already advanced past the reviewed diff
// (its immutable review content was already committed to HEAD before the
// review even started, matching approvedBaseDiffScopeRecoveryFixture's own
// construction). That breaks the recovery shape's
// `next.BaseTree == initial.BaseTree` requirement, so both lineages fall
// through to the generic scope-changed classifier instead — the actual bug
// this phase fixes. The overlay+staged safety stop
// (TargetStatusActionStop / ReplayabilityManualActionRequired) still
// applies: it is the identical live-projection safety check the
// zero-candidate branch already performs, unrelated to lineage history.
// Both stale lineages stay listed as optional recovery candidates.
func TestAssessTargetStatusReportsPluralStaleStagedOverlayLineagesAsUnrelatedWithStop(t *testing.T) {
	repo, _, predecessor, _, _ := approvedBaseDiffScopeRecoveryFixture(t, "staged-overlay-stale-first")
	peer := predecessor
	peer.LineageID = "staged-overlay-stale-second"
	writeTerminalTargetStatusAuthority(t, repo, peer)
	reviewedBase := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	writeSnapshotFile(t, repo, "docs/candidate.md", "# Candidate\nexpanded again\n")
	gitSnapshot(t, repo, "add", "docs/candidate.md")

	got, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: Target{
		Kind: TargetBaseWorkspaceOverlay, Projection: ProjectionStaged,
		BaseRef: reviewedBase, IntendedUntracked: []string{},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Applicability != TargetApplicabilityUnrelated || got.Action != TargetStatusActionStop ||
		got.Replayability != ReplayabilityManualActionRequired ||
		!equalStrings(got.CandidateLineageIDs, []string{"staged-overlay-stale-first", "staged-overlay-stale-second"}) {
		t.Fatalf("plural stale staged overlay status = %#v", got)
	}
}

func TestAssessTargetStatusFailsClosedForMixedSameLineageAuthority(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "review-collision")
	storeCompactStartAuthority(t, repo, state)
	storeLegacyReviewingStatus(t, repo, state.LineageID, state.InitialSnapshot)

	got, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: state.LineageID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Applicability != TargetApplicabilityCorrupted || got.Action != TargetStatusActionRepairAuthority {
		t.Fatalf("status = %#v", got)
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
	t.Run("historical failed validator with a changed target recovers as escalated", func(t *testing.T) {
		repo, state, _, _ := historicalFailedValidatorFixture(t, "disposition-historical")
		writeSnapshotFile(t, repo, "tracked.txt", "changed recovery target\n")
		status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
			Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: state.LineageID,
		})
		if err != nil || status.Action != TargetStatusActionRecover || status.ActionDisposition != RecoveryEscalated {
			t.Fatalf("historical failed validator status = %#v, %v", status, err)
		}
	})
	t.Run("invalidated authority recovers as invalidated", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
		state := newCompactTestState(t, repo, "disposition-invalidated")
		store := storeCompactStartAuthority(t, repo, state)
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
func TestNonRecoveryStatusOmitsDisposition(t *testing.T) {
	requireSnapshotGit(t)
	t.Run("reviewing routes to finalize without a disposition", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
		state := newCompactTestState(t, repo, "disposition-reviewing")
		storeCompactStartAuthority(t, repo, state)
		status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
			Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: state.LineageID,
		})
		if err != nil || status.Action != TargetStatusActionFinalize || status.ActionDisposition != "" {
			t.Fatalf("reviewing status = %#v, %v", status, err)
		}
	})
	t.Run("authorized correction resume routes to finalize without a disposition", func(t *testing.T) {
		repo, predecessor, _, _ := correctionScopeRecoveryFixture(t, "disposition-resume")
		status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
			Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: predecessor.LineageID,
		})
		if err != nil || status.State != StateCorrectionRequired || status.Action != TargetStatusActionFinalize ||
			status.ActionDisposition != "" {
			t.Fatalf("correction resume status = %#v, %v", status, err)
		}
	})
	t.Run("historical failed validator with an unchanged target stops without a disposition", func(t *testing.T) {
		repo, state, _, _ := historicalFailedValidatorFixture(t, "disposition-historical-stop")
		status, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
			Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: state.LineageID,
		})
		if err != nil || status.Action != TargetStatusActionStop || status.ActionDisposition != "" {
			t.Fatalf("unchanged historical status = %#v, %v", status, err)
		}
	})
}

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
	receiptBefore, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}

	got, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Applicability != TargetApplicabilityUnrelated || got.Action != TargetStatusActionStart ||
		got.LineageID != "" || got.ReceiptIdentity != "" {
		t.Fatalf("committed corrected history status = %#v", got)
	}
	stateAfter, stateErr := os.ReadFile(store.StatePath())
	receiptAfter, receiptErr := os.ReadFile(store.ReceiptPath())
	if stateErr != nil || receiptErr != nil || !bytes.Equal(stateBefore, stateAfter) || !bytes.Equal(receiptBefore, receiptAfter) {
		t.Fatalf("status changed authority or receipt bytes: stateErr=%v receiptErr=%v", stateErr, receiptErr)
	}
	if after := authorityBytes(t, authorityRoot); !reflect.DeepEqual(before, after) {
		t.Fatalf("status mutated committed corrected history: before=%v after=%v", before, after)
	}
}

func TestAssessTargetStatusTreatsMissingHistoricalIntendedPathAsNonApplicable(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "historical.txt", "reviewed historical content\n")
	state, store, _ := approvedCompactCurrentChangesFixture(t, repo, "status-missing-intended", []string{"historical.txt"})
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
	receiptBefore, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}

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
	receiptAfter, err := os.ReadFile(store.ReceiptPath())
	if err != nil || !bytes.Equal(receiptBefore, receiptAfter) {
		t.Fatalf("status changed historical receipt bytes: err=%v", err)
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
		store := storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, "status-filesystem-error"))
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
	t.Run("pending finalize journal reconciles", func(t *testing.T) {
		fixture := newCompactReviewerCaptureFixture(t, "frozen-pending-finalize")
		request := finalizeAttemptTestRequest(fixture.state.LineageID, fixture.request.ExpectedRevision, "evidence")
		request.CandidateDigest = FinalizeAttemptValueDigest("candidate", fixture.state.CurrentSnapshot)
		request.RequestDigest = FinalizeAttemptRequestDigest(request)
		if _, _, err := fixture.store.BeginFinalizeAttempt(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(fixture.store.repo, "internal", "a.go"), []byte("package internal\n\nfunc Value() int { return 3 }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		status, err := AssessTargetStatus(context.Background(), fixture.store.repo, TargetStatusRequest{
			Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: fixture.state.LineageID,
		})
		if err != nil || status.Action != TargetStatusActionReconcileFinalize || status.Replayability != ReplayabilityStatusRequired {
			t.Fatalf("pending frozen finalize status = %#v, %v", status, err)
		}
	})
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
	t.Run("fully occupied undrifted candidate finalizes", func(t *testing.T) {
		fixture := newCompactReviewerCaptureFixture(t, "frozen-complete-undrifted")
		if _, err := fixture.store.CaptureAdmittedReviewerResult(context.Background(), fixture.request); err != nil {
			t.Fatal(err)
		}
		status, err := AssessTargetStatus(context.Background(), fixture.store.repo, TargetStatusRequest{
			Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: fixture.state.LineageID,
		})
		if err != nil || status.Applicability != TargetApplicabilityCurrent || status.LineageID != fixture.state.LineageID ||
			status.Action != TargetStatusActionFinalize || status.Replayability != ReplayabilityNotReplayable {
			t.Fatalf("fully occupied undrifted frozen status = %#v, %v", status, err)
		}
	})

	t.Run("semantic frozen evidence", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
		store := storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, "frozen-semantic"))
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
		store := storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, "frozen-not-reviewing"))
		record, _ := store.Load()
		record.State.State = StateInvalidated
		if eligible, _, err := explicitReviewingCompactCandidate(context.Background(), repo, targetStatusCandidate{compact: &record}); eligible || err != nil {
			t.Fatalf("non-reviewing candidate = eligible %v error %v", eligible, err)
		}
	})

	t.Run("superseded selected lineage", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
		predecessor, store, _ := approvedCompactRevisionFixture(t, repo, "frozen-superseded")
		record, _ := store.Load()
		writeSnapshotFile(t, repo, "tracked.txt", "drifted candidate\n")
		successor := newCompactTestState(t, repo, "frozen-superseded-next")
		successor.Generation = predecessor.Generation + 1
		if _, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: record.Revision, Successor: successor, Disposition: RecoveryScopeChanged, Reason: "supersede frozen review", Actor: "maintainer"}); err != nil {
			t.Fatal(err)
		}
		got, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: predecessor.LineageID})
		if err != nil || got.Applicability != TargetApplicabilityUnrelated || got.Action != TargetStatusActionStart {
			t.Fatalf("superseded frozen status = %#v, %v", got, err)
		}
	})
}

func TestAssessTargetStatusStillCorruptsMalformedAuthorityGraph(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "reviewed candidate\n")
	predecessor, predecessorStore, _ := approvedCompactCurrentChangesFixture(t, repo, "status-graph-predecessor", []string{})
	predecessorRecord, err := predecessorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "successor candidate\n")
	successor := newCompactTestState(t, repo, "status-graph-successor")
	successor.Generation = predecessor.Generation + 1
	if _, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: predecessorRecord.Revision,
		Successor: successor, Disposition: RecoveryScopeChanged, Reason: "scope changed", Actor: "maintainer",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(predecessorStore.Dir); err != nil {
		t.Fatal(err)
	}

	// The successor whose predecessor is gone is the entry that is corrupted,
	// and naming it says so. The live target inherits nothing from it and is
	// assessed on its own terms.
	scoped := targetStatusCurrentChangesRequest()
	scoped.LineageID = successor.LineageID
	got, err := AssessTargetStatus(context.Background(), repo, scoped)
	if err != nil {
		t.Fatal(err)
	}
	if got.Applicability != TargetApplicabilityCorrupted || got.Action != TargetStatusActionRepairAuthority ||
		got.Replayability != ReplayabilityManualActionRequired {
		t.Fatalf("malformed graph status = %#v", got)
	}
	unrelated, err := AssessTargetStatus(context.Background(), repo, targetStatusCurrentChangesRequest())
	if err != nil {
		t.Fatal(err)
	}
	if unrelated.Applicability == TargetApplicabilityCorrupted {
		t.Fatalf("a dangling successor made the live target corrupted: %#v", unrelated)
	}
}

func TestAssessTargetStatusIgnoresReceiptlessUnrelatedLegacyHistory(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "legacy candidate\n")
	transaction, _, _ := nativeGateFixture(t, repo, "legacy-receiptless-history")
	store, err := AuthoritativeStore(context.Background(), repo, transaction.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	appendApprovedStoreChain(t, store, transaction)
	receiptPath := filepath.Join(store.Dir, "artifacts", "receipt.json")
	if _, err := os.Stat(receiptPath); !os.IsNotExist(err) {
		t.Fatalf("receiptless legacy fixture stat = %v, want not-exist", err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "unrelated candidate\n")

	authorityRoot, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	before := authorityBytes(t, authorityRoot)
	got, err := AssessTargetStatus(context.Background(), repo, targetStatusCurrentChangesRequest())
	if err != nil {
		t.Fatal(err)
	}
	if got.Applicability != TargetApplicabilityUnrelated || got.Action != TargetStatusActionStart ||
		got.LineageID != "" || got.ReceiptIdentity != "" {
		t.Fatalf("receiptless unrelated legacy status = %#v", got)
	}
	if _, err := os.Stat(receiptPath); !os.IsNotExist(err) {
		t.Fatalf("status materialized a legacy receipt: %v", err)
	}
	if after := authorityBytes(t, authorityRoot); !reflect.DeepEqual(before, after) {
		t.Fatalf("status mutated receiptless legacy history: before=%v after=%v", before, after)
	}
}

func TestAssessTargetStatusReadsCompactPublicationCoherently(t *testing.T) {
	requireSnapshotGit(t)
	for _, receiptFirst := range []bool{false, true} {
		name := "state then receipt"
		if receiptFirst {
			name = "receipt then state"
		}
		t.Run(name, func(t *testing.T) {
			repo, store, initial, reviewed, terminal, receipt := compactStatusPublicationFixture(t, "status-publication-"+map[bool]string{false: "state-first", true: "receipt-first"}[receiptFirst])
			originalHook := targetStatusCompactAuthorityReadHook
			t.Cleanup(func() { targetStatusCompactAuthorityReadHook = originalHook })

			startPublication := make(chan struct{})
			publicationDone := make(chan error, 1)
			go func() {
				<-startPublication
				publicationDone <- publishCompactStatusTerminal(store, initial, reviewed, terminal, receipt, receiptFirst)
			}()
			var once sync.Once
			var publicationErr error
			var expectedState, expectedReceipt []byte
			targetStatusCompactAuthorityReadHook = func(lineage, phase string, attempt int) {
				if lineage != terminal.LineageID || phase != "after-state" || attempt != 0 {
					return
				}
				once.Do(func() {
					close(startPublication)
					publicationErr = <-publicationDone
					if publicationErr == nil {
						expectedState, _ = os.ReadFile(store.StatePath())
						expectedReceipt, _ = os.ReadFile(store.ReceiptPath())
					}
				})
			}

			got, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
				Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: terminal.LineageID,
			})
			if publicationErr != nil {
				t.Fatalf("publish terminal authority: %v", publicationErr)
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Applicability != TargetApplicabilityCurrent || got.State != StateApproved ||
				got.Action != TargetStatusActionValidate || got.ReceiptIdentity == "" {
				t.Fatalf("coherent compact publication status = %#v", got)
			}
			stateAfter, stateErr := os.ReadFile(store.StatePath())
			receiptAfter, receiptErr := os.ReadFile(store.ReceiptPath())
			if stateErr != nil || receiptErr != nil || !bytes.Equal(expectedState, stateAfter) || !bytes.Equal(expectedReceipt, receiptAfter) {
				t.Fatalf("status mutated published authority: stateErr=%v receiptErr=%v", stateErr, receiptErr)
			}
		})
	}
}

func TestAssessTargetStatusBindsCompactArtifactPublicationCoherently(t *testing.T) {
	requireSnapshotGit(t)
	tests := []struct {
		name                  string
		phase                 string
		initialReceipt        string
		publishedReceipt      string
		initialJournal        string
		publishedJournal      string
		wantAction            TargetStatusAction
		wantReplayability     Replayability
		wantReceiptPayloadKey string
	}{
		{
			name: "missing receipt and journal publish between artifact reads", phase: "after-first-receipt",
			initialReceipt: "missing", publishedReceipt: "canonical",
			initialJournal: "missing", publishedJournal: "completed",
			wantAction: TargetStatusActionValidate, wantReplayability: ReplayabilityNotReplayable,
			wantReceiptPayloadKey: "canonical",
		},
		{
			name: "missing receipt publishes between artifact reads", phase: "after-first-receipt",
			initialReceipt: "missing", publishedReceipt: "canonical",
			initialJournal: "completed", publishedJournal: "completed",
			wantAction: TargetStatusActionValidate, wantReplayability: ReplayabilityNotReplayable,
			wantReceiptPayloadKey: "canonical",
		},
		{
			name: "present receipt disappears between artifact reads", phase: "after-first-receipt",
			initialReceipt: "canonical", publishedReceipt: "missing",
			initialJournal: "completed", publishedJournal: "completed",
			wantAction: TargetStatusActionFinalize, wantReplayability: ReplayabilityExactReplaySafe,
			wantReceiptPayloadKey: "missing",
		},
		{
			name: "present receipt is replaced between artifact reads", phase: "after-first-receipt",
			initialReceipt: "canonical", publishedReceipt: "alternate",
			initialJournal: "completed", publishedJournal: "completed",
			wantAction: TargetStatusActionValidate, wantReplayability: ReplayabilityNotReplayable,
			wantReceiptPayloadKey: "alternate",
		},
		{
			name: "missing journal becomes pending between artifact observations", phase: "after-artifacts",
			initialReceipt: "canonical", publishedReceipt: "canonical",
			initialJournal: "missing", publishedJournal: "pending",
			wantAction: TargetStatusActionReconcileFinalize, wantReplayability: ReplayabilityStatusRequired,
			wantReceiptPayloadKey: "canonical",
		},
		{
			name: "missing journal completes between artifact observations", phase: "after-artifacts",
			initialReceipt: "canonical", publishedReceipt: "canonical",
			initialJournal: "missing", publishedJournal: "completed",
			wantAction: TargetStatusActionValidate, wantReplayability: ReplayabilityNotReplayable,
			wantReceiptPayloadKey: "canonical",
		},
		{
			name: "pending journal disappears between artifact observations", phase: "after-artifacts",
			initialReceipt: "canonical", publishedReceipt: "canonical",
			initialJournal: "pending", publishedJournal: "missing",
			wantAction: TargetStatusActionValidate, wantReplayability: ReplayabilityNotReplayable,
			wantReceiptPayloadKey: "canonical",
		},
		{
			name: "pending journal completes between artifact observations", phase: "after-artifacts",
			initialReceipt: "canonical", publishedReceipt: "canonical",
			initialJournal: "pending", publishedJournal: "completed",
			wantAction: TargetStatusActionValidate, wantReplayability: ReplayabilityNotReplayable,
			wantReceiptPayloadKey: "canonical",
		},
		{
			name: "completed journal is replaced between artifact observations", phase: "after-artifacts",
			initialReceipt: "canonical", publishedReceipt: "canonical",
			initialJournal: "completed", publishedJournal: "replacement",
			wantAction: TargetStatusActionValidate, wantReplayability: ReplayabilityNotReplayable,
			wantReceiptPayloadKey: "canonical",
		},
		{
			name: "completed journal disappears between artifact observations", phase: "after-artifacts",
			initialReceipt: "canonical", publishedReceipt: "canonical",
			initialJournal: "completed", publishedJournal: "missing",
			wantAction: TargetStatusActionValidate, wantReplayability: ReplayabilityNotReplayable,
			wantReceiptPayloadKey: "canonical",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCompactStatusStableArtifactFixture(t, "status-artifact-"+strings.ReplaceAll(tt.name, " ", "-"))
			if err := replaceCompactStatusArtifact(fixture.store.ReceiptPath(), fixture.receiptPayload(tt.initialReceipt)); err != nil {
				t.Fatal(err)
			}
			if err := replaceCompactStatusArtifact(fixture.store.FinalizeAttemptJournalPath(), fixture.journalPayload(tt.initialJournal)); err != nil {
				t.Fatal(err)
			}

			originalHook := targetStatusCompactAuthorityReadHook
			t.Cleanup(func() { targetStatusCompactAuthorityReadHook = originalHook })
			publish := make(chan struct{})
			published := make(chan error, 1)
			go func() {
				<-publish
				err := replaceCompactStatusArtifact(fixture.store.ReceiptPath(), fixture.receiptPayload(tt.publishedReceipt))
				if err == nil {
					err = replaceCompactStatusArtifact(fixture.store.FinalizeAttemptJournalPath(), fixture.journalPayload(tt.publishedJournal))
				}
				published <- err
			}()
			attempts := 0
			var once sync.Once
			var publicationErr error
			targetStatusCompactAuthorityReadHook = func(lineage, phase string, _ int) {
				if lineage != fixture.state.LineageID {
					return
				}
				if phase == "after-state" {
					attempts++
				}
				if phase == tt.phase {
					once.Do(func() {
						close(publish)
						publicationErr = <-published
					})
				}
			}

			got, err := AssessTargetStatus(context.Background(), fixture.repo, TargetStatusRequest{
				Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: fixture.state.LineageID,
			})
			if publicationErr != nil {
				t.Fatalf("publish compact status artifacts: %v", publicationErr)
			}
			if err != nil {
				t.Fatal(err)
			}
			wantReceipt := fixture.receiptPayload(tt.wantReceiptPayloadKey)
			wantIdentity := compactStatusArtifactIdentity(wantReceipt)
			if got.Applicability != TargetApplicabilityCurrent || got.State != StateApproved ||
				got.Action != tt.wantAction || got.Replayability != tt.wantReplayability ||
				got.ReceiptIdentity != wantIdentity {
				t.Fatalf("artifact transition status = %#v, want action=%s replayability=%s receipt=%q", got, tt.wantAction, tt.wantReplayability, wantIdentity)
			}
			if attempts != 2 {
				t.Fatalf("artifact transition stable-read attempts = %d, want 2", attempts)
			}
			assertCompactStatusArtifact(t, fixture.store.ReceiptPath(), fixture.receiptPayload(tt.publishedReceipt))
			assertCompactStatusArtifact(t, fixture.store.FinalizeAttemptJournalPath(), fixture.journalPayload(tt.publishedJournal))
		})
	}
}

func TestAssessTargetStatusBoundsUnstableCompactArtifactReads(t *testing.T) {
	requireSnapshotGit(t)
	fixture := newCompactStatusStableArtifactFixture(t, "status-artifact-churn")
	if err := replaceCompactStatusArtifact(fixture.store.ReceiptPath(), fixture.receiptPayload("canonical")); err != nil {
		t.Fatal(err)
	}
	if err := replaceCompactStatusArtifact(fixture.store.FinalizeAttemptJournalPath(), fixture.journalPayload("completed")); err != nil {
		t.Fatal(err)
	}
	originalHook := targetStatusCompactAuthorityReadHook
	t.Cleanup(func() { targetStatusCompactAuthorityReadHook = originalHook })
	writes := 0
	var hookErr error
	targetStatusCompactAuthorityReadHook = func(lineage, phase string, _ int) {
		if lineage != fixture.state.LineageID || phase != "after-first-receipt" || hookErr != nil {
			return
		}
		writes++
		key := "alternate"
		if writes%2 == 0 {
			key = "canonical"
		}
		hookErr = replaceCompactStatusArtifact(fixture.store.ReceiptPath(), fixture.receiptPayload(key))
	}

	got, err := AssessTargetStatus(context.Background(), fixture.repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: fixture.state.LineageID,
	})
	if hookErr != nil {
		t.Fatal(hookErr)
	}
	if !errors.Is(err, ErrConcurrentUpdate) || got.Applicability == TargetApplicabilityCorrupted {
		t.Fatalf("unstable artifact status = %#v, error = %T %v", got, err, err)
	}
	if writes != 3 {
		t.Fatalf("unstable artifact attempts = %d, want bounded 3", writes)
	}
}

func TestAssessTargetStatusStopsSecondArtifactReadOnContextCancellation(t *testing.T) {
	requireSnapshotGit(t)
	fixture := newCompactStatusStableArtifactFixture(t, "status-second-artifact-cancel")
	if err := replaceCompactStatusArtifact(fixture.store.ReceiptPath(), fixture.receiptPayload("canonical")); err != nil {
		t.Fatal(err)
	}
	if err := replaceCompactStatusArtifact(fixture.store.FinalizeAttemptJournalPath(), fixture.journalPayload("completed")); err != nil {
		t.Fatal(err)
	}
	originalHook := targetStatusCompactAuthorityReadHook
	t.Cleanup(func() { targetStatusCompactAuthorityReadHook = originalHook })
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	targetStatusCompactAuthorityReadHook = func(lineage, phase string, _ int) {
		if lineage == fixture.state.LineageID && phase == "after-second-receipt" {
			calls++
			cancel()
		}
	}

	got, err := AssessTargetStatus(ctx, fixture.repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: fixture.state.LineageID,
	})
	if !errors.Is(err, context.Canceled) || got.Applicability == TargetApplicabilityCorrupted {
		t.Fatalf("canceled second artifact read status = %#v, error = %T %v", got, err, err)
	}
	if calls != 1 {
		t.Fatalf("canceled second artifact read calls = %d, want 1", calls)
	}
}

func TestAssessTargetStatusStopsSecondArtifactReadOnDeadline(t *testing.T) {
	requireSnapshotGit(t)
	fixture := newCompactStatusStableArtifactFixture(t, "status-second-artifact-deadline")
	if err := replaceCompactStatusArtifact(fixture.store.ReceiptPath(), fixture.receiptPayload("canonical")); err != nil {
		t.Fatal(err)
	}
	if err := replaceCompactStatusArtifact(fixture.store.FinalizeAttemptJournalPath(), fixture.journalPayload("completed")); err != nil {
		t.Fatal(err)
	}
	originalHook := targetStatusCompactAuthorityReadHook
	t.Cleanup(func() { targetStatusCompactAuthorityReadHook = originalHook })
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	calls := 0
	targetStatusCompactAuthorityReadHook = func(lineage, phase string, _ int) {
		if lineage == fixture.state.LineageID && phase == "after-second-receipt" {
			calls++
			<-ctx.Done()
		}
	}

	got, err := AssessTargetStatus(ctx, fixture.repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: fixture.state.LineageID,
	})
	if !errors.Is(err, context.DeadlineExceeded) || got.Applicability == TargetApplicabilityCorrupted {
		t.Fatalf("deadline second artifact read status = %#v, error = %T %v", got, err, err)
	}
	// The same deadline covers Git-backed snapshot setup, so a slow runner may
	// exhaust it before the first authority observation. No second read may start.
	if calls > 1 {
		t.Fatalf("deadline second artifact read calls = %d, want at most 1", calls)
	}
}

func TestAssessTargetStatusBoundsUnstableCompactAuthorityReads(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "status-authority-churn")
	store := storeCompactStartAuthority(t, repo, state)
	_, originalPayload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	alternate := state
	alternate.PolicyHash = hash("2")
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
	storeCompactStartAuthority(t, repo, state)
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
	storeCompactStartAuthority(t, repo, state)
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

func TestCompactCorrectionTargetStartStatusParity(t *testing.T) {
	requireSnapshotGit(t)
	scopeFixture := func(t *testing.T, lineage string) (string, CompactState) {
		repo, state, _, _ := correctionScopeRecoveryFixture(t, lineage)
		return repo, state
	}
	contractionFixture := func(t *testing.T, lineage string) (string, CompactState) {
		repo, state, _, _ := correctionContractionRecoveryFixture(t, lineage)
		return repo, state
	}
	historicalFixture := func(t *testing.T, lineage string) (string, CompactState) {
		repo, state, _, _ := historicalFailedValidatorFixture(t, lineage)
		return repo, state
	}
	tests := []struct {
		name        string
		fixture     func(*testing.T, string) (string, CompactState)
		mutate      func(*testing.T, string) []string
		prepare     func(*testing.T, *CompactState)
		want        compactCorrectionTargetClaim
		operational bool
	}{
		{name: "same", fixture: scopeFixture, want: compactCorrectionTargetResume},
		{name: "contraction", fixture: contractionFixture, mutate: func(t *testing.T, repo string) []string {
			writeSnapshotFile(t, repo, "deleted.txt", "delete me\n")
			return []string{}
		}, want: compactCorrectionTargetRecover},
		{name: "expansion", fixture: scopeFixture, mutate: func(t *testing.T, repo string) []string {
			writeSnapshotFile(t, repo, "outside.go", "package outside\n")
			return []string{"outside.go"}
		}, want: compactCorrectionTargetRecover},
		{name: "overlap", fixture: contractionFixture, mutate: func(t *testing.T, repo string) []string {
			writeSnapshotFile(t, repo, "deleted.txt", "delete me\n")
			writeSnapshotFile(t, repo, "outside.go", "package outside\n")
			return []string{"outside.go"}
		}, want: compactCorrectionTargetRecover},
		{name: "unrelated", fixture: scopeFixture, mutate: func(t *testing.T, repo string) []string {
			writeSnapshotFile(t, repo, "tracked.txt", "base\n")
			writeSnapshotFile(t, repo, "outside.go", "package outside\n")
			return []string{"outside.go"}
		}, want: compactCorrectionTargetUnclaimed},
		{name: "historical exhausted unchanged", fixture: historicalFixture, want: compactCorrectionTargetBlocked},
		{name: "historical exhausted changed", fixture: historicalFixture, mutate: func(t *testing.T, repo string) []string {
			writeSnapshotFile(t, repo, "tracked.txt", "changed exhausted target\n")
			return []string{}
		}, want: compactCorrectionTargetRecover},
		{name: "operational error", fixture: scopeFixture, prepare: func(t *testing.T, state *CompactState) {
			if err := state.BeginCorrection(1); err != nil {
				t.Fatal(err)
			}
		}, mutate: func(t *testing.T, repo string) []string {
			writeSnapshotFile(t, repo, "tracked.txt", "candidate requiring correction\n")
			return []string{}
		}, operational: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, existing := tt.fixture(t, "parity-"+strings.ReplaceAll(tt.name, " ", "-"))
			if tt.prepare != nil {
				tt.prepare(t, &existing)
			}
			intended := []string{}
			if tt.mutate != nil {
				intended = tt.mutate(t, repo)
			}
			live, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
				Kind: TargetCurrentChanges, IntendedUntracked: intended,
			})
			if err != nil {
				t.Fatal(err)
			}
			requested := existing
			requested.InitialSnapshot = live

			originalCommand := gitCommandContext
			if tt.operational {
				t.Setenv("GENTLE_AI_TARGET_STATUS_GIT_HELPER", "exit73")
				gitCommandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
					return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTargetStatusGitHelperProcess$", "--")
				}
			}
			startClaim, startErr := classifyCompactCorrectionTargetForStart(context.Background(), repo, existing, requested)
			statusClaim, statusErr := classifyCompactCorrectionTargetForStatus(context.Background(), repo, existing, live)
			gitCommandContext = originalCommand

			if tt.operational {
				var startGit, statusGit *GitCommandError
				if !errors.As(startErr, &startGit) || !errors.As(statusErr, &statusGit) ||
					startGit.ExitCode != 73 || statusGit.ExitCode != 73 {
					t.Fatalf("operational parity: start=(%v,%T %v) status=(%v,%T %v)", startClaim, startErr, startErr, statusClaim, statusErr, statusErr)
				}
				return
			}
			if startErr != nil || statusErr != nil || startClaim != tt.want || statusClaim != tt.want {
				t.Fatalf("claim parity: start=(%v,%v) status=(%v,%v), want %v", startClaim, startErr, statusClaim, statusErr, tt.want)
			}
		})
	}
}

func compactStatusPublicationFixture(t *testing.T, lineage string) (string, CompactStore, CompactRecord, CompactState, CompactState, CompactReceipt) {
	t.Helper()
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, lineage)
	store := storeCompactStartAuthority(t, repo, state)
	initial, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
	}
	if err := state.CompleteReview(CompactReviewInput{
		LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
	reviewed := state
	if err := state.CompleteVerification([]byte("verified\n"), true); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	return repo, store, initial, reviewed, state, receipt
}

type compactStatusStableArtifactFixture struct {
	repo               string
	store              CompactStore
	state              CompactState
	receiptCanonical   []byte
	receiptAlternate   []byte
	journalPending     []byte
	journalCompleted   []byte
	journalReplacement []byte
}

func newCompactStatusStableArtifactFixture(t *testing.T, lineage string) compactStatusStableArtifactFixture {
	t.Helper()
	repo, store, initial, reviewed, terminal, receipt := compactStatusPublicationFixture(t, lineage)
	revision, err := store.Replace(initial.Revision, "review/complete-review", reviewed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-verification", terminal); err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	canonical = append(canonical, '\n')
	alternate, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	alternate = append(alternate, '\n')

	request := finalizeAttemptTestRequest(lineage, record.Revision, "status artifact pending")
	request.CandidateDigest = FinalizeAttemptValueDigest("candidate", terminal.CurrentSnapshot)
	request.RequestDigest = FinalizeAttemptRequestDigest(request)
	pendingAttempt := FinalizeAttempt{Request: request, Transitions: []FinalizeAttemptTransition{}}
	pending, err := marshalFinalizeAttemptJournal(finalizeAttemptJournal{
		Schema: finalizeAttemptJournalSchema, Attempts: []FinalizeAttempt{pendingAttempt},
	})
	if err != nil {
		t.Fatal(err)
	}
	completedAttempt := pendingAttempt
	completedAttempt.ReceiptPublished = true
	completedAttempt.Completed = true
	completed, err := marshalFinalizeAttemptJournal(finalizeAttemptJournal{
		Schema: finalizeAttemptJournalSchema, Attempts: []FinalizeAttempt{completedAttempt},
	})
	if err != nil {
		t.Fatal(err)
	}
	replacementRequest := finalizeAttemptTestRequest(lineage, record.Revision, "status artifact replacement")
	replacementRequest.CandidateDigest = FinalizeAttemptValueDigest("candidate", terminal.CurrentSnapshot)
	replacementRequest.RequestDigest = FinalizeAttemptRequestDigest(replacementRequest)
	replacementAttempt := FinalizeAttempt{
		Request: replacementRequest, Transitions: []FinalizeAttemptTransition{}, ReceiptPublished: true, Completed: true,
	}
	replacement, err := marshalFinalizeAttemptJournal(finalizeAttemptJournal{
		Schema: finalizeAttemptJournalSchema, Attempts: []FinalizeAttempt{replacementAttempt},
	})
	if err != nil {
		t.Fatal(err)
	}
	return compactStatusStableArtifactFixture{
		repo: repo, store: store, state: terminal,
		receiptCanonical: canonical, receiptAlternate: alternate,
		journalPending: pending, journalCompleted: completed, journalReplacement: replacement,
	}
}

func (fixture compactStatusStableArtifactFixture) receiptPayload(key string) []byte {
	switch key {
	case "missing":
		return nil
	case "canonical":
		return fixture.receiptCanonical
	case "alternate":
		return fixture.receiptAlternate
	default:
		panic("unknown receipt payload key: " + key)
	}
}

func (fixture compactStatusStableArtifactFixture) journalPayload(key string) []byte {
	switch key {
	case "missing":
		return nil
	case "pending":
		return fixture.journalPending
	case "completed":
		return fixture.journalCompleted
	case "replacement":
		return fixture.journalReplacement
	default:
		panic("unknown journal payload key: " + key)
	}
}

func replaceCompactStatusArtifact(path string, payload []byte) error {
	if payload == nil {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	return writeAtomic(path, payload, 0o644)
}

func compactStatusArtifactIdentity(payload []byte) string {
	if payload == nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func assertCompactStatusArtifact(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if want == nil {
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("artifact %q exists after status: %v", path, err)
		}
		return
	}
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("artifact %q changed after status: err=%v got=%q want=%q", path, err, got, want)
	}
}

func publishCompactStatusTerminal(store CompactStore, initial CompactRecord, reviewed, terminal CompactState, receipt CompactReceipt, receiptFirst bool) error {
	if receiptFirst {
		if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
			return err
		}
	}
	revision, err := store.Replace(initial.Revision, "review/complete-review", reviewed)
	if err != nil {
		return err
	}
	if _, err := store.Replace(revision, "review/complete-verification", terminal); err != nil {
		return err
	}
	if !receiptFirst {
		return WriteCompactReceiptAtomic(store.ReceiptPath(), receipt)
	}
	return nil
}

func TestTargetStatusGitHelperProcess(t *testing.T) {
	switch os.Getenv("GENTLE_AI_TARGET_STATUS_GIT_HELPER") {
	case "exit73":
		os.Exit(73)
	case "sleep":
		time.Sleep(10 * time.Second)
	}
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
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	return store
}

func writeApprovedTargetStatusHistory(t *testing.T, repo string, count int) {
	t.Helper()
	state := newCompactTestState(t, repo, "status-history-template")
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
	}
	if err := state.CompleteReview(CompactReviewInput{LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("verified\n"), true); err != nil {
		t.Fatal(err)
	}
	root, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < count; index++ {
		candidate := state
		candidate.LineageID = fmt.Sprintf("status-history-%03d", index)
		_, payload, err := makeCompactRecord(candidate)
		if err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, "v2", candidate.LineageID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, compactStateFileName), payload, 0o644); err != nil {
			t.Fatal(err)
		}
		receipt, err := candidate.Receipt()
		if err != nil {
			t.Fatal(err)
		}
		if err := WriteCompactReceiptAtomic(filepath.Join(dir, compactReceiptFileName), receipt); err != nil {
			t.Fatal(err)
		}
	}
}

func targetStatusOperationalFailureFixture(t *testing.T, lineage string) string {
	t.Helper()
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	storeCompactStartAuthority(t, repo, newCompactTestState(t, repo, lineage))
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
