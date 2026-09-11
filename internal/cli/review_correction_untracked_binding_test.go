package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestNegotiatedStatusPreservesUntrackedBindingThroughCorrectionLineage pins
// issue #3849: a correction_required lineage whose frozen candidate carries
// an explicit intended-untracked selection must keep offering its own
// correction transition, bound to the authority's own target identity, on a
// plain STATUS re-entry that names no untracked-scope flags -- exactly the
// shape of the exact bound continuation `review.capture-correction-plan`
// itself returns. It must not re-demand a fresh intended-untracked
// declaration and bind the resulting collect input to a different (live,
// declaration-less) target identity than the one the authority is bound to.
func TestNegotiatedStatusPreservesUntrackedBindingThroughCorrectionLineage(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "correction-untracked-binding-3849"
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, "notes.txt", "untracked but explicitly selected\n", 0o644)

	_, digest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{
		"--cwd", repo, "--lineage", lineage,
		"--untracked-scope=select", "--expected-untracked-inventory=" + digest, "--intended-untracked", "notes.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &started)
	if len(started.SelectedLenses) != 1 {
		t.Fatalf("started lenses = %v, want exactly one selected lens", started.SelectedLenses)
	}

	captureCLIReviewerResultWithFindings(t, repo, started, 0, []facadeFinding{{
		Location: "candidate.go:1", Severity: "CRITICAL", Claim: "candidate exposes the wrong behavior",
		ProofRefs:     []string{"exact changed hunk", "reproduced candidate failure"},
		EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, &bytes.Buffer{})

	captureCorrectionPlanFromCurrentStatus(t, repo, lineage, 1)

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(record.State.InitialSnapshot.IntendedUntracked) != 1 || record.State.InitialSnapshot.IntendedUntracked[0] != "notes.txt" {
		t.Fatalf("fixture did not freeze the declared untracked selection: %#v", record.State.InitialSnapshot)
	}

	// The exact bound STATUS continuation `review.capture-correction-plan`
	// returns carries no untracked-scope flags: it is a plain
	// `--next-transition --lineage` re-entry, exactly like the operation
	// shape issue #3849 reports.
	status := negotiatedReviewStatusForLineage(t, repo, lineage)
	if status.NextTransition == nil {
		t.Fatal("correction lineage STATUS produced no next transition")
	}
	if status.NextTransition.Kind == reviewNextTransitionCollect && status.NextTransition.ReasonCode == "intended_untracked_selection_required" {
		t.Fatalf("correction lineage dead-ended into a fresh intended-untracked declaration: %#v", status.NextTransition)
	}
	if status.NextTransition.ReasonCode != "corrected_candidate_unavailable" {
		t.Fatalf("next transition reason = %q, want corrected_candidate_unavailable", status.NextTransition.ReasonCode)
	}
	if status.TargetIdentity != record.State.CurrentSnapshot.Identity {
		t.Fatalf("status target identity = %q, want the authority's own bound target identity %q", status.TargetIdentity, record.State.CurrentSnapshot.Identity)
	}
	if status.NextTransition.Collect != nil {
		for _, input := range status.NextTransition.Collect.Inputs {
			for _, argument := range input.Arguments {
				if argument.Name == "target_identity" && argument.Value != record.State.CurrentSnapshot.Identity {
					t.Fatalf("collect input target_identity = %q, want authority target identity %q", argument.Value, record.State.CurrentSnapshot.Identity)
				}
			}
		}
	}
}

// TestSelectorlessStatusPreservesIntendedUntrackedBindingBeforeCorrectionPlan
// reproduces #4435's first dead end: after a selected-untracked review enters
// correction_required, the provider-published selectorless STATUS continuation
// must retain the frozen selection rather than demanding a new declaration.
func TestSelectorlessStatusPreservesIntendedUntrackedBindingBeforeCorrectionPlan(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "correction-untracked-selectorless-status-4435"
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, "notes.txt", "untracked but explicitly selected\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, "local-only.txt", "eligible but deliberately unselected\n", 0o644)

	_, digest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{
		"--cwd", repo, "--lineage", lineage,
		"--untracked-scope=select", "--expected-untracked-inventory=" + digest, "--intended-untracked", "notes.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &started)
	if len(started.SelectedLenses) != 1 {
		t.Fatalf("started lenses = %v, want exactly one selected lens", started.SelectedLenses)
	}

	var closureOutput bytes.Buffer
	captureCLIReviewerResultWithFindings(t, repo, started, 0, []facadeFinding{{
		Location: "candidate.go:1", Severity: "CRITICAL", Claim: "candidate exposes the wrong behavior",
		ProofRefs:     []string{"exact changed hunk", "reproduced candidate failure"},
		EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, &closureOutput)

	var closure reviewLastEventClosureResult
	decodeStrictReviewJSON(t, closureOutput.Bytes(), &closure)
	if closure.Schema != reviewLastEventClosureSchema || closure.Operation != "review/capture-result" ||
		closure.LineageID != lineage || closure.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("selectorless final capture closure = %#v, want correction_required closure", closure)
	}
	if closure.StatusContinuation == nil {
		t.Fatalf("selectorless final capture closure lacks status_continuation: %s", closureOutput.String())
	}
	continuation := closure.StatusContinuation
	if continuation.Operation != "review.status" {
		t.Fatalf("selectorless status continuation operation = %q, want review.status", continuation.Operation)
	}
	for _, argument := range continuation.Arguments {
		for _, forbidden := range []string{"--untracked-scope", "--expected-untracked-inventory", "--intended-untracked"} {
			if argument.Token == forbidden || strings.HasPrefix(argument.Token, forbidden+"=") {
				t.Fatalf("selectorless status continuation leaks %q: %#v", forbidden, continuation.Arguments)
			}
		}
	}

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	// Replay only the operation and opaque tokens the final reviewer capture
	// published; reconstructing STATUS would hide a malformed continuation.
	statusArgs := []string{strings.TrimPrefix(continuation.Operation, "review.")}
	for _, argument := range continuation.Arguments {
		statusArgs = append(statusArgs, argument.Token)
	}
	var statusOutput bytes.Buffer
	if err := RunReview(statusArgs, &statusOutput); err != nil {
		t.Fatalf("run selectorless status continuation unchanged: %v\n%s", err, statusOutput.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	if status.Authority == nil || status.Authority.LineageID != lineage || status.Authority.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("selectorless STATUS authority = %#v, want correction_required authority for lineage %q", status.Authority, lineage)
	}
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "correction_plan_required" || status.NextTransition.CorrectionRequest == nil {
		t.Fatalf("selectorless STATUS transition = %#v, want collect/correction_plan_required", status.NextTransition)
	}

	request := status.NextTransition.CorrectionRequest
	if status.TargetIdentity != record.State.CurrentSnapshot.Identity || request.TargetIdentity != record.State.CurrentSnapshot.Identity {
		t.Fatalf("selectorless STATUS target bindings = status %q, correction request %q, want %q", status.TargetIdentity, request.TargetIdentity, record.State.CurrentSnapshot.Identity)
	}
	if status.Authority.Revision != record.Revision {
		t.Fatalf("selectorless STATUS authority revision = %q, want %q", status.Authority.Revision, record.Revision)
	}
	if request.ExpectedRevision != record.State.CapturePhaseRevision {
		t.Fatalf("correction request capture-phase revision = %q, want %q", request.ExpectedRevision, record.State.CapturePhaseRevision)
	}
	if request.LineageID != lineage {
		t.Fatalf("correction request lineage = %q, want %q", request.LineageID, lineage)
	}

	// Losing one frozen selected path is real scope drift. Extra unselected paths
	// may remain outside the candidate, but they must not stand in for the missing
	// selected path when the same continuation is re-entered.
	if err := os.Remove(filepath.Join(repo, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	statusOutput.Reset()
	if err := RunReview(statusArgs, &statusOutput); err != nil {
		t.Fatalf("run continuation after selected path removal: %v\n%s", err, statusOutput.String())
	}
	var missingSelected ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &missingSelected)
	if missingSelected.NextTransition == nil || missingSelected.NextTransition.Kind != reviewNextTransitionCollect ||
		missingSelected.NextTransition.ReasonCode != "intended_untracked_selection_required" {
		t.Fatalf("missing selected path transition = %#v, want intended_untracked_selection_required", missingSelected.NextTransition)
	}
}
