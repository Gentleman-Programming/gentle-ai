package cli

import (
	"bytes"
	"context"
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
