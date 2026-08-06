package reviewtransaction

// Wave 5 (Gate Cutover), Slice 3, task 4.2: the 5 named per-gate deny
// goldens (#2222/#2239 supersession-evidence naming convention) proving
// attachGateVerdictRelation's real wiring: EvaluateCompactGate -- the exact
// function runReviewFacadeValidate calls for a discovered compact receipt,
// and the exact function the Slice 1 gate-boundary-matrix harness drives
// through the real binary -- now populates Relation/Next on a "changed"
// relation denial (candidate-or-paths-mismatch, or PrePR's base-mismatch
// variant), and that Next names a runnable operation.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// gateVerdictWorkspaceChangedFixture builds a workspace (current-changes)
// approved receipt over tracked.txt, then drifts the candidate by changing
// tracked.txt's own content. Since #2394 an undeclared untracked addition is
// not review scope at all, so drifting a declared path is the only way to
// reach the candidate-or-paths-mismatch check this fixture targets. Suitable
// for post-apply and pre-commit.
func gateVerdictWorkspaceChangedFixture(t *testing.T, gate GateKind, stage bool) (string, CompactReceipt, NativeGateRequestInput) {
	t.Helper()
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "approved candidate\n")
	lineage := "gate-verdict-changed-" + string(gate)
	state, _, receipt := approvedCompactCurrentChangesFixture(t, repo, lineage, []string{})
	writeSnapshotFile(t, repo, "tracked.txt", "drifted after approval\n")
	if stage {
		gitSnapshot(t, repo, "add", "tracked.txt")
	}
	return repo, receipt, NativeGateRequestInput{Gate: gate, LineageID: state.LineageID}
}

// gateVerdictCommittedChangedFixture builds a committed (exact-revision)
// approved receipt with a configured publication remote, then adds a
// further commit -- suitable for pre-push, which requires an actual
// delivered commit range and a resolvable upstream.
func gateVerdictCommittedChangedFixture(t *testing.T, gate GateKind) (string, CompactReceipt, NativeGateRequestInput) {
	t.Helper()
	repo := initSnapshotRepo(t)
	branch := currentBranch(context.Background(), repo)
	configurePublicationRemote(t, repo, branch)
	lineage := "gate-verdict-changed-committed-" + string(gate)
	state, _, receipt := approvedCompactRevisionFixture(t, repo, lineage)
	writeSnapshotFile(t, repo, "outside.txt", "outside frozen scope\n")
	gitSnapshot(t, repo, "add", "outside.txt")
	gitSnapshot(t, repo, "commit", "-m", "drift after approval")
	return repo, receipt, NativeGateRequestInput{Gate: gate, LineageID: state.LineageID, BaseRef: "origin/" + branch}
}

// gateVerdictReleaseChangedFixture builds a committed approved receipt with
// minimal (structurally complete, content-arbitrary) release evidence
// artifacts -- buildCompactGateRequest requires all five release artifact
// paths just to construct a release target, before the
// candidate-or-paths-mismatch check this fixture targets is ever reached.
func gateVerdictReleaseChangedFixture(t *testing.T) (string, CompactReceipt, NativeGateRequestInput) {
	t.Helper()
	repo := initSnapshotRepo(t)
	lineage := "gate-verdict-changed-release"
	state, _, receipt := approvedCompactRevisionFixture(t, repo, lineage)
	writeSnapshotFile(t, repo, "outside.txt", "outside frozen scope\n")
	gitSnapshot(t, repo, "add", "outside.txt")
	gitSnapshot(t, repo, "commit", "-m", "drift after approval")
	dir := t.TempDir()
	input := NativeGateRequestInput{Gate: GateRelease, LineageID: state.LineageID}
	for path, target := range map[string]*string{
		"configuration": &input.ReleaseConfiguration, "generated": &input.ReleaseGenerated,
		"provenance": &input.ReleaseProvenance, "boundary": &input.ReleasePublicationBoundary,
		"freshness": &input.ReleaseEvidenceFreshness,
	} {
		full := filepath.Join(dir, path)
		if err := os.WriteFile(full, []byte(path+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		*target = full
	}
	return repo, receipt, input
}

// assertChangedRelationDenialCarriesNextStep asserts the "changed" relation
// classification landed (Relation field) and that Next carries the exact
// expected shape. Post-apply/pre-commit/pre-push/release fixtures here keep
// BaseRelationshipValid true, so gateVerdict's relation table alone decides
// Next ("review start"/"candidate_changed"). Pre-PR and release ALSO carry
// the absorbed N2 precondition (BaseRelationshipValid gated to those two
// gates): when a fixture's drift genuinely breaks BaseRelationshipValid too,
// gateVerdict's precondition check reports THAT reason instead
// ("base_relationship_invalid", no Transition) -- itself still "never a
// bare denial" (ReasonCode is always present), and callers pass the
// precondition-shaped want* explicitly for that case.
func assertChangedRelationDenialCarriesNextStep(t *testing.T, evaluation NativeGateEvaluation, wantResult GateResult, wantTransition, wantReasonCode string) {
	t.Helper()
	if evaluation.Result != wantResult {
		t.Fatalf("gate verdict result = %#v, want %q", evaluation, wantResult)
	}
	if evaluation.Relation != ShadowRelationChanged {
		t.Fatalf("gate verdict relation = %q, want %q: %#v", evaluation.Relation, ShadowRelationChanged, evaluation)
	}
	if evaluation.Next == nil {
		t.Fatalf("gate verdict denial carries no Next step: %#v", evaluation)
	}
	if evaluation.Next.Transition != wantTransition {
		t.Fatalf("gate verdict Next.Transition = %q, want %q", evaluation.Next.Transition, wantTransition)
	}
	if evaluation.Next.ReasonCode != wantReasonCode {
		t.Fatalf("gate verdict Next.ReasonCode = %q, want %q", evaluation.Next.ReasonCode, wantReasonCode)
	}
	// "Never a bare denial": SOMETHING is always readable, transition or not.
	if evaluation.Next.Transition == "" && evaluation.Next.ReasonCode == "" {
		t.Fatal("gate verdict denial carries neither a Transition nor a ReasonCode")
	}
	// Runnability (Wave 4 CRITICAL-A lesson): a non-empty Transition must be a
	// real operation runReviewCommand's dispatch table
	// (internal/cli/review_facade.go) actually routes, not a name that
	// merely looks plausible.
	if evaluation.Next.Transition != "" && !gateVerdictNextStepIsRunnableOperation(evaluation.Next.Transition) {
		t.Fatalf("gate verdict Next.Transition %q is not a dispatched review operation", evaluation.Next.Transition)
	}
}

// gateVerdictNextStepIsRunnableOperation mirrors runReviewCommand's exact
// dispatch vocabulary (internal/cli/review_facade.go's runReviewCommand
// switch) so this package's test can verify runnability without importing
// internal/cli (which would create an import cycle: internal/cli already
// imports internal/reviewtransaction).
func gateVerdictNextStepIsRunnableOperation(transition string) bool {
	switch transition {
	case "review start", "review finalize", "review validate", "review status", "review repair",
		"review invalidate", "review abandon", "review recover", "review retry-final-verification":
		return true
	default:
		return false
	}
}

func TestPostApplyGate_Deny_ChangedRelationCarriesNextStep(t *testing.T) {
	repo, receipt, input := gateVerdictWorkspaceChangedFixture(t, GatePostApply, false)
	evaluation := EvaluateCompactGate(context.Background(), repo, receipt, input)
	assertChangedRelationDenialCarriesNextStep(t, evaluation, GateScopeChanged, "review start", "candidate_changed")
}

func TestPreCommitGate_Deny_ChangedRelationCarriesNextStep(t *testing.T) {
	repo, receipt, input := gateVerdictWorkspaceChangedFixture(t, GatePreCommit, true)
	evaluation := EvaluateCompactGate(context.Background(), repo, receipt, input)
	assertChangedRelationDenialCarriesNextStep(t, evaluation, GateScopeChanged, "review start", "candidate_changed")
}

func TestPrePushGate_Deny_ChangedRelationCarriesNextStep(t *testing.T) {
	repo, receipt, input := gateVerdictCommittedChangedFixture(t, GatePrePush)
	evaluation := EvaluateCompactGate(context.Background(), repo, receipt, input)
	assertChangedRelationDenialCarriesNextStep(t, evaluation, GateScopeChanged, "review start", "candidate_changed")
}

// TestReleaseGate_Deny_ChangedRelationCarriesNextStep: release is one of
// the two absorbed-N2 gates (BaseRelationshipValid gated to pre-pr/release).
// This fixture's drift breaks BOTH the candidate-or-paths match AND
// BaseRelationshipValid, so gateVerdict's precondition check -- which the
// absorbed N2 tests (gate_verdict_test.go) prove runs BEFORE the relation
// table -- reports the precondition's own reason, not the relation table's.
// Both are still "changed" relation and still never a bare denial.
func TestReleaseGate_Deny_ChangedRelationCarriesNextStep(t *testing.T) {
	repo, receipt, input := gateVerdictReleaseChangedFixture(t)
	evaluation := EvaluateCompactGate(context.Background(), repo, receipt, input)
	assertChangedRelationDenialCarriesNextStep(t, evaluation, GateScopeChanged, "", "base_relationship_invalid")
}

// TestPrePRGate_Deny_BaseMismatchDeniesWithoutComposition is the PrePR
// variant: a base-mismatch denial (not candidate-or-paths-mismatch),
// reached because GatePrePR is not strictBinding and this fixture's
// snapshot.BaseTree diverges from the receipt's own BaseTree. Called
// directly against EvaluateCompactGate -- not through
// runReviewFacadeValidate's funnel. Wave 5 Slice 5 deleted
// EvaluateCompactPrePRChain and its two funnel call sites entirely
// (TestPrePRComposition_ZeroCallers, internal/cli, proves it by
// call-absence), so "without composition" now holds for every pre-PR call
// path, not just this direct one -- this test's own construction (calling
// EvaluateCompactGate directly) simply predates that deletion and stays
// correct unchanged.
func TestPrePRGate_Deny_BaseMismatchDeniesWithoutComposition(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "approved candidate\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "approved candidate")
	lineage := "gate-verdict-changed-pre-pr"
	state, _, receipt := approvedCompactRevisionFixture(t, repo, lineage)
	base := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD^"))
	branch := currentBranch(context.Background(), repo)
	remote := configurePublicationRemote(t, repo, branch)
	gitSnapshot(t, repo, "--git-dir", remote, "branch", "reviewed-base", base)

	evaluation := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{
		Gate: GatePrePR, LineageID: state.LineageID, BaseRef: "origin/" + branch,
	})
	if evaluation.Result != GateInvalidated {
		t.Fatalf("pre-pr base-mismatch evaluation = %#v, want invalidated", evaluation)
	}
	if evaluation.Context.Denial == nil || evaluation.Context.Denial.Code != "base-mismatch" {
		t.Fatalf("pre-pr denial code = %#v, want base-mismatch", evaluation.Context.Denial)
	}
	// Pre-PR is the other absorbed-N2 gate: this fixture's base-mismatch IS
	// precisely BaseRelationshipValid=false, so the precondition's own
	// reason reports, exactly as it does for release above.
	assertChangedRelationDenialCarriesNextStep(t, evaluation, GateInvalidated, "", "base_relationship_invalid")
}
