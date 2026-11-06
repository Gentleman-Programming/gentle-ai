package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// closeCommittedBaseDiffLineageIntoCorrectionRequired starts a committed-range
// (base-diff, committed-only) review on a committed candidate.go with no
// intended untracked selection, captures one CRITICAL finding on the first
// lens and clean results on the rest, and returns the lineage sitting in
// correction_required with no correction plan captured yet.
func closeCommittedBaseDiffLineageIntoCorrectionRequired(t *testing.T, repo, lineage string) {
	t.Helper()
	const baseRef = "frozen-base"
	runReviewCLIGit(t, repo, "branch", baseRef, "HEAD")
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	runReviewCLIGit(t, repo, "add", "candidate.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "candidate")

	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{
		"--cwd", repo, "--lineage", lineage, "--base-ref", baseRef, "--committed-only",
	})
	if err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &started)
	if len(started.SelectedLenses) == 0 {
		t.Fatalf("committed base-diff START selected no lenses: %#v", started)
	}
	for order := range started.SelectedLenses {
		findings := []facadeFinding{}
		if order == 0 {
			findings = []facadeFinding{{
				Location: "candidate.go:1", Severity: "CRITICAL", Claim: "candidate exposes the wrong behavior",
				ProofRefs:     []string{"exact changed hunk", "reproduced candidate failure"},
				EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
			}}
		}
		captureCLIReviewerResultWithFindings(t, repo, started, order, findings, &bytes.Buffer{})
	}
	corroborateRefuterClaimsForTest(t, repo, lineage)

	// Only the committed range moves the base branch afterwards; the frozen
	// tree, never the mutable ref, is what a selectorless re-entry resumes.
	runReviewCLIGit(t, repo, "branch", "-f", baseRef, "HEAD")
}

func loadCompactReviewRecordForTest(t *testing.T, repo, lineage string) reviewtransaction.CompactRecord {
	t.Helper()
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return record
}

// TestSelectorlessStatusResumesCommittedRangeCorrectionRequired pins
// gentle-shell#2379/#1596: a committed-range lineage (base-diff,
// committed-only, empty frozen intended-untracked selection) that reaches
// correction_required must keep offering its own correction continuation on a
// bound STATUS that names the lineage but no selector -- the shape a host sends
// after a restart. Before the fix the selectorless re-entry fell back to the
// current-changes target and stranded the lineage as unrelated.
func TestSelectorlessStatusResumesCommittedRangeCorrectionRequired(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)

	t.Run("clean committed range resumes its frozen correction target", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		const lineage = "committed-range-correction-selectorless"
		closeCommittedBaseDiffLineageIntoCorrectionRequired(t, repo, lineage)
		// An eligible untracked file must not pull the committed range into
		// a fresh intended-untracked declaration either.
		writeUndeclaredWorkspaceFile(t, repo, "scratch.txt", "local-only notes\n", 0o644)

		record := loadCompactReviewRecordForTest(t, repo, lineage)
		if record.State.State != reviewtransaction.StateCorrectionRequired ||
			record.State.CurrentSnapshot.Kind != reviewtransaction.TargetBaseDiff ||
			len(record.State.InitialSnapshot.IntendedUntracked) != 0 {
			t.Fatalf("fixture authority = state %q kind %q untracked %v, want correction_required base-diff with no untracked",
				record.State.State, record.State.CurrentSnapshot.Kind, record.State.InitialSnapshot.IntendedUntracked)
		}

		status := negotiatedReviewStatusForLineage(t, repo, lineage)
		if status.Applicability != reviewtransaction.TargetApplicabilityCurrent || status.Authority == nil ||
			status.Authority.LineageID != lineage || status.Authority.State != reviewtransaction.StateCorrectionRequired {
			t.Fatalf("selectorless committed-range correction status = applicability %q authority %#v, want current_target correction_required %q",
				status.Applicability, status.Authority, lineage)
		}
		if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
			status.NextTransition.ReasonCode != "correction_plan_required" || status.NextTransition.CorrectionRequest == nil {
			t.Fatalf("selectorless committed-range correction transition = %#v, want collect/correction_plan_required", status.NextTransition)
		}
		if status.TargetIdentity != record.State.CurrentSnapshot.Identity ||
			status.NextTransition.CorrectionRequest.TargetIdentity != record.State.CurrentSnapshot.Identity {
			t.Fatalf("selectorless committed-range bindings = status %q, request %q, want authority target %q",
				status.TargetIdentity, status.NextTransition.CorrectionRequest.TargetIdentity, record.State.CurrentSnapshot.Identity)
		}

		// The offered correction plan must actually capture against the
		// authority, through the same selectorless re-entry.
		captureArgs := reviewTransitionInputTokens(t, repo, status.NextTransition.Collect.Inputs[0])
		captureArgs = append(captureArgs, "--correction-lines", "1")
		if err := RunReviewCaptureCorrectionPlan(captureArgs, &bytes.Buffer{}); err != nil {
			t.Fatalf("capture status-offered correction plan: %v", err)
		}
	})

	t.Run("committed drift outside the frozen manifest routes to recovery", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		const lineage = "committed-range-correction-drift"
		closeCommittedBaseDiffLineageIntoCorrectionRequired(t, repo, lineage)
		record := loadCompactReviewRecordForTest(t, repo, lineage)

		if err := os.WriteFile(filepath.Join(repo, "unrelated.go"), []byte("package unrelated\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runReviewCLIGit(t, repo, "add", "unrelated.go")
		runReviewCLIGit(t, repo, "commit", "-qm", "drift outside the reviewed range")

		status := negotiatedReviewStatusForLineage(t, repo, lineage)
		if status.TargetIdentity == record.State.CurrentSnapshot.Identity {
			t.Fatalf("drifted committed range kept the frozen identity %q", status.TargetIdentity)
		}
		if status.NextTransition != nil && status.NextTransition.ReasonCode == "correction_plan_required" {
			t.Fatalf("drifted committed range was offered the frozen correction plan: %#v", status.NextTransition)
		}
		// Same fail-closed routing as current-changes drift outside the frozen
		// manifest (#4094): recovery, never a forced-current correction.
		if status.Action != reviewtransaction.TargetStatusActionRecover || status.ActionDisposition != reviewtransaction.RecoveryScopeChanged ||
			status.Authority == nil || status.Authority.LineageID != lineage || status.NextTransition == nil ||
			status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.recover" {
			t.Fatalf("drifted committed range status = %#v, want review.recover for scope_changed", status)
		}
	})

	t.Run("committed drift inside the frozen manifest matches the explicit selector", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		const lineage = "committed-range-correction-in-manifest"
		closeCommittedBaseDiffLineageIntoCorrectionRequired(t, repo, lineage)
		record := loadCompactReviewRecordForTest(t, repo, lineage)
		writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 2 }\n", 0o644)
		runReviewCLIGit(t, repo, "commit", "-qam", "edit the reviewed path")

		// The resumed target is re-derived from the live worktree, never
		// forced to the frozen identity: the selectorless re-entry must route
		// exactly like the explicit committed-range selector does.
		status := negotiatedReviewStatusForLineage(t, repo, lineage)
		var explicitOutput bytes.Buffer
		if err := RunReview([]string{
			"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition", "--lineage", lineage,
			"--base-ref", record.State.CurrentSnapshot.BaseTree, "--committed-only",
		}, &explicitOutput); err != nil {
			t.Fatalf("explicit committed-range status: %v\n%s", err, explicitOutput.String())
		}
		var explicit ReviewTargetStatusResult
		decodeStrictReviewJSON(t, explicitOutput.Bytes(), &explicit)
		if status.TargetIdentity == record.State.CurrentSnapshot.Identity {
			t.Fatalf("drifted committed range kept the frozen identity %q", status.TargetIdentity)
		}
		if status.TargetIdentity != explicit.TargetIdentity || status.Applicability != explicit.Applicability ||
			status.Action != explicit.Action || status.NextTransition == nil || explicit.NextTransition == nil ||
			status.NextTransition.ReasonCode != explicit.NextTransition.ReasonCode {
			t.Fatalf("selectorless status = %q/%q/%q, explicit = %q/%q/%q, want identical routing",
				status.TargetIdentity, status.Applicability, status.Action, explicit.TargetIdentity, explicit.Applicability, explicit.Action)
		}
	})

	t.Run("current-changes correction lineage keeps requiring a live untracked declaration", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		lineage := closeReviewLineageIntoCorrectionRequired(t, repo, "current-changes-correction-selectorless")
		writeUndeclaredWorkspaceFile(t, repo, "scratch.txt", "local-only notes\n", 0o644)

		status := negotiatedReviewStatusForLineage(t, repo, lineage)
		if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
			status.NextTransition.ReasonCode != "intended_untracked_selection_required" {
			t.Fatalf("current-changes correction transition = %#v, want collect/intended_untracked_selection_required", status.NextTransition)
		}
	})
}
