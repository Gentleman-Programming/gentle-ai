package reviewtransaction

import (
	"testing"
)

func accountingOnlyEscalatedState(t *testing.T, repo, lineage string) CompactState {
	t.Helper()
	state, fix := pendingCompactCorrection(t, repo, lineage)
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := bindTargetedValidationForTest(ScopedValidationResult{
		LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: true},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true},
	}, fix)
	if err := state.CompleteCorrection(fix, 2, validation); err != nil {
		t.Fatal(err)
	}

	state.State = StateEscalated
	view, err := state.CompactReviewView()
	if err != nil {
		t.Fatal(err)
	}
	// Re-capture the historical legacy-policy role values through the canonical
	// capture boundary. The old policy's phase differs from the current floor-two
	// phase, so keeping the old entries would be an invalid synthetic authority.
	legacy := state
	legacy.State, legacy.CurrentSnapshot = StateReviewing, legacy.InitialSnapshot
	legacy.OriginalChangedLines, legacy.CorrectionBudget, legacy.CorrectionBudgetPolicy = 1, 1, ""
	legacy.FixFindingIDs = []string{}
	legacy.ProposedCorrectionLines, legacy.ActualCorrectionLines = nil, nil
	legacy.FixDeltaHash, legacy.OriginalCriteria, legacy.CorrectionRegression, legacy.EvidenceHash = EmptyFixDeltaHash, nil, nil, ""
	legacy.CorrectionAttempts, legacy.CumulativeCorrectionLines = nil, 0
	legacy.AdmittedRoleResults = []CompactAdmittedRoleResult{}
	legacy.CapturePhaseEpoch = 0
	phase, err := deriveCompactCapturePhaseRevision(legacy)
	if err != nil {
		t.Fatal(err)
	}
	legacy.CapturePhaseRevision = phase
	store, err := CompactAuthoritativeStore(t.Context(), repo, legacy.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	writeCompactFixtureRecord(t, store, legacy)
	for order := range legacy.SelectedLenses {
		captureCompactLens(t, store, legacy, order, view.LensResults[order].Findings...)
	}
	captured, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	state.OriginalChangedLines, state.CorrectionBudget, state.CorrectionBudgetPolicy = 1, 1, ""
	state.AdmittedRoleResults = captured.State.AdmittedRoleResults
	state.CapturePhaseRevision, state.CapturePhaseEpoch = captured.State.CapturePhaseRevision, captured.State.CapturePhaseEpoch
	if err := state.Validate(); err != nil {
		t.Fatal(err)
	}
	return state
}

// TestCompactEscalationAccountingBudgetExceeded pins that an accounting-only
// escalation (both correction criteria passed, cumulative correction lines
// exceed the frozen budget) reports Cause=budget_exceeded with Remaining
// clamped at 0 even though Spent > Total.
func TestCompactEscalationAccountingBudgetExceeded(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := accountingOnlyEscalatedState(t, repo, "escalation-accounting-budget")
	accounting := state.EscalationAccounting()
	if accounting.Cause != CompactEscalationCauseBudgetExceeded {
		t.Fatalf("Cause = %q, want %q", accounting.Cause, CompactEscalationCauseBudgetExceeded)
	}
	if accounting.Total != state.CorrectionBudget {
		t.Fatalf("Total = %d, want frozen budget %d", accounting.Total, state.CorrectionBudget)
	}
	if accounting.Spent != state.CumulativeCorrectionLines {
		t.Fatalf("Spent = %d, want cumulative lines %d", accounting.Spent, state.CumulativeCorrectionLines)
	}
	if accounting.Spent <= accounting.Total {
		t.Fatalf("fixture is not over budget: spent %d, total %d", accounting.Spent, accounting.Total)
	}
	if accounting.Remaining != 0 {
		t.Fatalf("Remaining = %d, want clamped 0 when spent %d exceeds total %d", accounting.Remaining, accounting.Spent, accounting.Total)
	}
}

// TestCompactEscalationAccountingOriginalCriteriaFailed pins that an
// escalation driven by a failed original-criteria check (within budget)
// reports Cause=original_criteria_failed with a positive Remaining.
func TestCompactEscalationAccountingOriginalCriteriaFailed(t *testing.T) {
	repo := initSnapshotRepo(t)
	state, fix := pendingCompactCorrection(t, repo, "escalation-accounting-original-criteria")
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := bindTargetedValidationForTest(ScopedValidationResult{
		LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: false},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true},
	}, fix)
	if err := state.CompleteCorrection(fix, 1, validation); err != nil {
		t.Fatal(err)
	}
	if state.State != StateEscalated {
		t.Fatalf("fixture state = %q, want escalated", state.State)
	}
	accounting := state.EscalationAccounting()
	if accounting.Cause != CompactEscalationCauseOriginalCriteriaFailed {
		t.Fatalf("Cause = %q, want %q", accounting.Cause, CompactEscalationCauseOriginalCriteriaFailed)
	}
	if accounting.Spent != 1 || accounting.Total != state.CorrectionBudget || accounting.Remaining != accounting.Total-accounting.Spent {
		t.Fatalf("accounting = %#v, want spent 1, total %d, remaining %d", accounting, state.CorrectionBudget, state.CorrectionBudget-1)
	}
	if accounting.Remaining <= 0 {
		t.Fatalf("fixture must stay within budget: %#v", accounting)
	}
}

// TestCompactEscalationAccountingCorrectionRegressionFailed pins that an
// escalation driven only by a failed correction-regression check (within
// budget, original criteria passed) reports
// Cause=correction_regression_failed.
func TestCompactEscalationAccountingCorrectionRegressionFailed(t *testing.T) {
	repo := initSnapshotRepo(t)
	state, fix := pendingCompactCorrection(t, repo, "escalation-accounting-regression")
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
	accounting := state.EscalationAccounting()
	if accounting.Cause != CompactEscalationCauseCorrectionRegressionFailed {
		t.Fatalf("Cause = %q, want %q", accounting.Cause, CompactEscalationCauseCorrectionRegressionFailed)
	}
}

// TestBeginCorrectionRefusesForecastBeyondTheFrozenBudget proves that a
// pre-edit forecast never escalates or consumes authority. The caller must
// choose a plan within the bound before changing the candidate.
func TestBeginCorrectionRefusesForecastBeyondTheFrozenBudget(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nwrong\n")
	state, _, _ := correctionRequiredAuthorityFixture(t, repo, "escalation-accounting-forecast")
	proposed := state.CorrectionBudget + 1
	if err := state.BeginCorrection(proposed); !IsCorrectionBudgetExceeded(err) {
		t.Fatalf("over-budget correction forecast error = %v, want CorrectionBudgetExceededError", err)
	}
	if state.State != StateCorrectionRequired || state.ProposedCorrectionLines != nil ||
		state.ActualCorrectionLines != nil || state.CumulativeCorrectionLines != 0 {
		t.Fatalf("over-budget forecast mutated correction authority: %#v", state)
	}
}

// TestCompactEscalationAccountingNotEscalatedHasNoCause pins that a
// non-escalated compact state (e.g. a fresh pending correction) reports a
// zero-value Cause while Spent/Remaining/Total still reflect the current
// budget bookkeeping.
func TestCompactEscalationAccountingNotEscalatedHasNoCause(t *testing.T) {
	repo := initSnapshotRepo(t)
	state, _ := pendingCompactCorrection(t, repo, "escalation-accounting-not-escalated")
	accounting := state.EscalationAccounting()
	if accounting.Cause != "" {
		t.Fatalf("Cause = %q, want empty for a non-escalated state", accounting.Cause)
	}
	if accounting.Spent != state.CumulativeCorrectionLines || accounting.Total != state.CorrectionBudget {
		t.Fatalf("accounting = %#v, want spent %d, total %d", accounting, state.CumulativeCorrectionLines, state.CorrectionBudget)
	}
}
