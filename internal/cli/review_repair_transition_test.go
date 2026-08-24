package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// corruptedDispositionStatus builds a TargetApplicabilityCorrupted status
// whose classified repair assessment is a real "unsupported" verdict (not
// eligible — Status == AuthorityRepairUnsupported, Candidate == nil — the
// actual non-eligible shape AssessAuthorityRepairAtRepositoryRoot returns
// via UnsupportedAuthorityRepairAssessment(), not an empty zero value
// review_facade.go never actually produces), but whose Disposition provider
// inputs ARE populated, exactly as review_facade.go's status assembly would
// leave it for a closed content-mismatched-recovery-authorization anomaly
// Wave 2's classified vocabulary never covers (rdd-closure-disposition-
// execution's whole reason for existing). This fixture feeds
// newReviewNextTransition directly (this file's tests below never call
// ReviewTargetStatusResult.Validate() on the surrounding envelope — that
// full-envelope contract, including the execute transition's Binding.
// LineageID/Revision requirement discovered while building
// ds12-negotiated-transition-route, is exercised end-to-end by the real
// `review status` CLI path and its bench journey instead).
func corruptedDispositionStatus() ReviewTargetStatusResult {
	return ReviewTargetStatusResult{
		Applicability:  reviewtransaction.TargetApplicabilityCorrupted,
		Action:         reviewtransaction.TargetStatusActionRepairAuthority,
		Replayability:  reviewtransaction.ReplayabilityManualActionRequired,
		TargetIdentity: "sha256:" + strings.Repeat("b", 64),
		Repair:         reviewtransaction.UnsupportedAuthorityRepairAssessment(),
		Disposition: &ReviewRepairDispositionProviderInputs{
			PlanDigest: "sha256:" + strings.Repeat("d", 64), AuthorityInventoryRevision: "sha256:" + strings.Repeat("e", 64),
			SeedLineageID: "review-damaged-closure-seed", SeedExpectedRevision: "sha256:" + strings.Repeat("f", 64),
		},
	}
}

// TestReviewNextTransitionDispositionCollectThenExecute satisfies tasks.md
// 4.1: for a repairable-classified graph with a derivable, admitted closure
// disposition plan (Disposition provider inputs populated, classified repair
// NOT eligible), next_transition offers the disposition collect{} first
// (no --actor/--reason/--authorization supplied), then the execute{} form
// once all three are supplied — never a raw flag triad the caller had to
// already know.
func TestReviewNextTransitionDispositionCollectThenExecute(t *testing.T) {
	t.Parallel()

	status := corruptedDispositionStatus()

	t.Run("collect when authorization inputs are missing", func(t *testing.T) {
		got := newReviewNextTransition(status, nil, nil, nil, reviewNextTransitionInput{})
		if got.Kind != reviewNextTransitionCollect {
			t.Fatalf("next transition kind = %q, want %q: %#v", got.Kind, reviewNextTransitionCollect, got)
		}
		if got.Collect == nil || len(got.Collect.Inputs) != 1 || got.Collect.Inputs[0].Name != "disposition_authorization" {
			t.Fatalf("collect inputs = %#v, want a single disposition_authorization input", got.Collect)
		}
		var sawPlanDigest, sawInventoryRevision bool
		for _, argument := range got.Collect.Inputs[0].Arguments {
			switch argument.Name {
			case "plan-digest":
				sawPlanDigest = argument.Value == status.Disposition.PlanDigest
			case "inventory-revision":
				sawInventoryRevision = argument.Value == status.Disposition.AuthorityInventoryRevision
			}
		}
		if !sawPlanDigest || !sawInventoryRevision {
			t.Fatalf("collect arguments do not carry the plan_digest/inventory_revision preview: %#v", got.Collect.Inputs[0].Arguments)
		}
	})

	t.Run("execute once actor/reason/authorization are supplied", func(t *testing.T) {
		input := reviewNextTransitionInput{RepairActor: "maintainer@example.com", RepairReason: "quarantine forged recovery authorization", RepairAuthorization: "sha256:real-disposition-authorization-bytes-never-emitted"}
		got := newReviewNextTransition(status, nil, nil, nil, input)
		if got.Kind != reviewNextTransitionExecute {
			t.Fatalf("next transition kind = %q, want %q: %#v", got.Kind, reviewNextTransitionExecute, got)
		}
		if got.Execute == nil || got.Execute.Operation != "review.repair" {
			t.Fatalf("execute operation = %#v, want review.repair", got.Execute)
		}
		wantFlags := map[string]string{
			"plan-digest": status.Disposition.PlanDigest, "inventory-revision": status.Disposition.AuthorityInventoryRevision,
			"actor": input.RepairActor, "reason": input.RepairReason,
		}
		seen := map[string]bool{}
		for _, argument := range got.Execute.Arguments {
			if want, ok := wantFlags[argument.Name]; ok {
				if argument.Value != want {
					t.Fatalf("execute argument %q = %q, want %q", argument.Name, argument.Value, want)
				}
				seen[argument.Name] = true
			}
			if argument.Name == "authorization" && argument.Value != "provided" {
				t.Fatalf("execute authorization argument leaks bytes: %q, want the \"provided\" sentinel", argument.Value)
			}
		}
		for name := range wantFlags {
			if !seen[name] {
				t.Fatalf("execute arguments are missing %q: %#v", name, got.Execute.Arguments)
			}
		}
		if !strings.Contains(got.Execute.Command, "--plan-digest") || !strings.Contains(got.Execute.Command, "--inventory-revision") ||
			!strings.Contains(got.Execute.Command, "--actor") || !strings.Contains(got.Execute.Command, "--reason") || !strings.Contains(got.Execute.Command, "--authorization") {
			t.Fatalf("rendered command line is missing a required flag: %q", got.Execute.Command)
		}
	})
}

// TestReviewNextTransitionClassifiedRepairTakesPriorityOverDisposition
// satisfies tasks.md 4.1's implicit ordering: when BOTH a classified repair
// candidate is eligible AND a disposition plan derives (an unlikely but
// possible overlap), the pre-existing classified route wins — Wave 6 adds a
// new route only for the case the classified vocabulary has nothing to say.
func TestReviewNextTransitionClassifiedRepairTakesPriorityOverDisposition(t *testing.T) {
	t.Parallel()

	status := corruptedDispositionStatus()
	status.Repair = reviewtransaction.AuthorityRepairAssessment{
		Schema: reviewtransaction.AuthorityRepairAssessmentSchema, Status: reviewtransaction.AuthorityRepairEligible,
		Class: "unchanged_target", RepositoryBinding: "binding", AuthorizationSchema: reviewtransaction.AuthorityRepairAuthorizationSchema,
		Candidate: &reviewtransaction.AuthorityRepairCandidate{LineageID: "classified-candidate", Revision: "sha256:" + strings.Repeat("f", 64)},
	}

	got := newReviewNextTransition(status, nil, nil, nil, reviewNextTransitionInput{})
	if got.Kind != reviewNextTransitionCollect || got.Collect == nil || len(got.Collect.Inputs) != 1 {
		t.Fatalf("next transition = %#v, want a single classified collect", got)
	}
	if got.Collect.Inputs[0].Name != "repair_authorization" {
		t.Fatalf("collect input name = %q, want the classified repair_authorization input (classified must win over disposition)", got.Collect.Inputs[0].Name)
	}
}

// TestReviewNextTransitionCorruptedWithNeitherRepairNorDispositionStops is
// the pre-existing (now regression-protected) behavior: a corrupted graph
// with no classified candidate and no derivable disposition plan still
// stops, naming the same reason code as before Wave 6.
func TestReviewNextTransitionCorruptedWithNeitherRepairNorDispositionStops(t *testing.T) {
	t.Parallel()

	status := ReviewTargetStatusResult{
		Applicability: reviewtransaction.TargetApplicabilityCorrupted, Action: reviewtransaction.TargetStatusActionRepairAuthority,
		Replayability: reviewtransaction.ReplayabilityManualActionRequired, TargetIdentity: "sha256:" + strings.Repeat("b", 64),
	}
	got := newReviewNextTransition(status, nil, nil, nil, reviewNextTransitionInput{})
	if got.Kind != reviewNextTransitionStop || got.ReasonCode != "corrupted_or_unverifiable_authority" {
		t.Fatalf("next transition = %#v, want stop/corrupted_or_unverifiable_authority (unchanged pre-Wave-6 behavior)", got)
	}
}

// TestReviewDispositionTransitionTokensCarryNoAuthorizationBytes is tasks.md
// 4.5's threat matrix (PR commands, applicable): the emitted transition's
// tokens never carry the real authorization bytes — only the "provided"
// sentinel — and every argument is tokenized via the shared
// reviewTokenizedTransitionArguments seam (the same one every other
// execute-form transition uses), so a caller running the printed command
// verbatim runs exactly what was named, nothing hidden.
func TestReviewDispositionTransitionTokensCarryNoAuthorizationBytes(t *testing.T) {
	t.Parallel()

	status := corruptedDispositionStatus()
	secretAuthorization := "sha256:top-secret-disposition-authorization-must-never-appear-in-output"
	input := reviewNextTransitionInput{RepairActor: "maintainer@example.com", RepairReason: "quarantine forged recovery authorization", RepairAuthorization: secretAuthorization}
	got := newReviewNextTransition(status, nil, nil, nil, input)
	if got.Execute == nil {
		t.Fatalf("expected an execute transition, got %#v", got)
	}
	for _, argument := range got.Execute.Arguments {
		if strings.Contains(argument.Token, secretAuthorization) || strings.Contains(argument.Value, secretAuthorization) {
			t.Fatalf("emitted argument leaks the real authorization bytes: %#v", argument)
		}
	}
	if strings.Contains(got.Execute.Command, secretAuthorization) {
		t.Fatalf("rendered command line leaks the real authorization bytes: %q", got.Execute.Command)
	}
	// Every argument must carry a non-empty token — reviewTokenizedTransitionArguments
	// is the one seam every execute-form transition (this one included) runs
	// through, so a caller never sees two different renderings of the same flag.
	for _, argument := range got.Execute.Arguments {
		if argument.Token == "" {
			t.Fatalf("argument %q has no token — did not run through reviewTokenizedTransitionArguments: %#v", argument.Name, argument)
		}
	}
}

// writeNonPristineDispositionRepairFixture builds a forged content-
// mismatched-recovery-authorization pair (mirroring writeInspectCLIRecoveryPair)
// whose successor has ALREADY completed a review — non-pristine — so
// SanctionedCompactRecoveryExits' abandon-eligibility check refuses first and
// falls through to the disposition-plan repair exit (mirrors
// compact_forged_authorization_test.go's forgedRecoveryPair captured-review
// mutation in package reviewtransaction, replicated here with exported
// constructs since this is package cli, which cannot import that unexported
// helper).
func writeNonPristineDispositionRepairFixture(t *testing.T, repo, prefix string) {
	t.Helper()
	path := "docs/" + prefix + ".md"
	absolute := filepath.Join(repo, path)
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte("predecessor\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	predecessorSnapshot, err := builder.Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{path},
	})
	if err != nil {
		t.Fatal(err)
	}
	predecessor := newInspectCLIState(t, builder, prefix+"-predecessor", 1, predecessorSnapshot)
	predecessor.State = reviewtransaction.StateEscalated
	predecessorRevision := writeReconcileCLIRecord(t, repo, predecessor)

	if err := os.WriteFile(absolute, []byte("successor\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	successorSnapshot, err := builder.Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{path},
	})
	if err != nil {
		t.Fatal(err)
	}
	successor := newInspectCLIState(t, builder, prefix+"-successor", 2, successorSnapshot)
	successor.Recovery = &reviewtransaction.CompactRecoveryProvenance{
		PredecessorLineageID: predecessor.LineageID, PredecessorRevision: predecessorRevision,
		Disposition: reviewtransaction.RecoveryEscalated, Reason: "retry", Actor: "maintainer@example.com",
		RecoveredAt: time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC), MaintainerAuthorization: dispositionForgedAuthorization,
	}
	results := make([]reviewtransaction.LensResult, 0, len(successor.SelectedLenses))
	for _, lens := range successor.SelectedLenses {
		results = append(results, reviewtransaction.LensResult{Lens: lens, Findings: []reviewtransaction.Finding{}, Evidence: []string{"reviewed once"}})
	}
	if err := successor.CompleteReview(reviewtransaction.CompactReviewInput{
		LensResults: results, Classifications: []reviewtransaction.FindingEvidence{}, RefuterOutcomes: []reviewtransaction.EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
	writeReconcileCLIRecord(t, repo, successor)
	if err := os.Remove(absolute); err != nil {
		t.Fatal(err)
	}
}
