package cli

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// emptyWorkspaceCandidateStatus reproduces the status a clean workspace
// produces once the reviewed work is already committed: a fresh target whose
// workspace projection froze zero paths, so base and candidate trees are the
// same tree. Executing the START this classification used to return fails
// deterministically in preflight with empty_candidate_scope, which is what
// issue #2584 observed looping.
func emptyWorkspaceCandidateStatus() ReviewTargetStatusResult {
	return ReviewTargetStatusResult{
		Contract:       ReviewIntegrationContractV2,
		Applicability:  reviewtransaction.TargetApplicabilityUnrelated,
		Action:         reviewtransaction.TargetStatusActionStart,
		Replayability:  reviewtransaction.ReplayabilityNotReplayable,
		TargetIdentity: "sha256:" + strings.Repeat("b", 64),
		Projection: ReviewTargetStatusProjection{
			Kind:                 reviewtransaction.TargetCurrentChanges,
			Projection:           reviewtransaction.ProjectionWorkspace,
			BaseTree:             strings.Repeat("c", 40),
			CurrentCandidateTree: strings.Repeat("c", 40),
			Paths:                nil,
		},
	}
}

func TestNegotiatedStatusCollectsBaseRefForEmptyWorkspaceCandidate(t *testing.T) {
	got := newReviewNextTransition(emptyWorkspaceCandidateStatus(), nil, nil, nil, nil, reviewNextTransitionInput{StartLineage: "review-empty-candidate"})

	if got.Kind != reviewNextTransitionCollect {
		t.Fatalf("empty workspace candidate transition kind = %q, want %q; re-offering an executable START loops on a preflight refusal", got.Kind, reviewNextTransitionCollect)
	}
	if got.ReasonCode != "empty_candidate_base_ref_required" {
		t.Fatalf("empty workspace candidate reason code = %q, want %q", got.ReasonCode, "empty_candidate_base_ref_required")
	}
	if got.Execute != nil {
		t.Fatalf("empty workspace candidate carries an executable transition %#v, want none", got.Execute)
	}
	if got.Collect == nil || len(got.Collect.Inputs) != 1 {
		t.Fatalf("empty workspace candidate collection = %#v, want exactly one input", got.Collect)
	}
	input := got.Collect.Inputs[0]
	if input.Name != "base_ref" {
		t.Fatalf("collected input name = %q, want %q", input.Name, "base_ref")
	}
	if input.CaptureOperation != "external.select_base_ref" {
		t.Fatalf("collected input capture operation = %q, want %q", input.CaptureOperation, "external.select_base_ref")
	}
}

// The refusal in PR #2505 deliberately names base_ref without deriving it, so
// the continuation that replaces it must not derive one either. A collection
// that shipped a base value would silently choose a review scope the caller
// never asked for.
func TestNegotiatedEmptyCandidateCollectionDoesNotDeriveBaseRef(t *testing.T) {
	got := newReviewNextTransition(emptyWorkspaceCandidateStatus(), nil, nil, nil, nil, reviewNextTransitionInput{StartLineage: "review-empty-candidate"})

	if got.Collect == nil || len(got.Collect.Inputs) != 1 {
		t.Fatalf("empty workspace candidate collection = %#v, want exactly one input", got.Collect)
	}
	input := got.Collect.Inputs[0]
	if input.Submission != nil {
		t.Fatalf("collected base_ref input carries submission descriptor %#v, want none: only plan_correction and run_targeted_validation may carry one", input.Submission)
	}
	for _, argument := range input.Arguments {
		if argument.Name == "base_ref" || argument.Name == "base-ref" {
			t.Fatalf("collected base_ref input pre-supplies %q=%q, want the caller to choose the base", argument.Name, argument.Value)
		}
	}
}

// The emitted continuation has to survive the same contract validation every
// negotiated transition crosses; an unrepresentable collection would strand
// the caller exactly like the looping START did.
func TestNegotiatedEmptyCandidateCollectionIsRepresentable(t *testing.T) {
	got := newReviewNextTransition(emptyWorkspaceCandidateStatus(), nil, nil, nil, nil, reviewNextTransitionInput{StartLineage: "review-empty-candidate"})

	if err := got.Validate(); err != nil {
		t.Fatalf("empty workspace candidate transition Validate() = %v, want nil", err)
	}
}

// The status contract validates a fresh target's transition separately from
// the transition's own shape, and it demanded an executable START for every
// fresh target that was not already stopping. Emitting the collection without
// teaching that validator about it would move the same disagreement from
// preflight into status validation.
func TestNegotiatedEmptyCandidateCollectionSatisfiesStatusTargetValidation(t *testing.T) {
	result := emptyWorkspaceCandidateStatus()
	result.NextTransition = &ReviewNextTransition{}
	*result.NextTransition = newReviewNextTransition(result, nil, nil, nil, nil, reviewNextTransitionInput{StartLineage: "review-empty-candidate"})

	if err := result.validateNextTransitionTargets(); err != nil {
		t.Fatalf("empty workspace candidate validateNextTransitionTargets() = %v, want nil", err)
	}
}

func TestNegotiatedEmptyCandidateCollectionRejectsMalformedStatusTarget(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ReviewNextTransition)
	}{
		{
			name: "missing collection",
			mutate: func(transition *ReviewNextTransition) {
				transition.Collect = nil
			},
		},
		{
			name: "wrong input name",
			mutate: func(transition *ReviewNextTransition) {
				transition.Collect.Inputs[0].Name = "lineage_selection"
			},
		},
		{
			name: "wrong target arguments",
			mutate: func(transition *ReviewNextTransition) {
				transition.Collect.Inputs[0].Arguments[0].Value = "sha256:" + strings.Repeat("a", 64)
			},
		},
		{
			name: "missing target arguments",
			mutate: func(transition *ReviewNextTransition) {
				transition.Collect.Inputs[0].Arguments = nil
			},
		},
		{
			name: "multiple inputs",
			mutate: func(transition *ReviewNextTransition) {
				transition.Collect.Inputs = append(transition.Collect.Inputs, transition.Collect.Inputs[0])
			},
		},
		{
			name: "unexpected submission descriptor",
			mutate: func(transition *ReviewNextTransition) {
				transition.Collect.Inputs[0].Submission = &ReviewTransitionSubmission{}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := emptyWorkspaceCandidateStatus()
			result.NextTransition = &ReviewNextTransition{}
			*result.NextTransition = newReviewNextTransition(result, nil, nil, nil, nil, reviewNextTransitionInput{StartLineage: "review-empty-candidate"})
			tt.mutate(result.NextTransition)

			if err := result.validateNextTransitionTargets(); err == nil {
				t.Fatal("malformed empty workspace candidate transition passed status-target validation")
			}
		})
	}
}

// Regression: a fresh candidate that really has changed paths keeps the
// executable START it always returned.
func TestNegotiatedStatusKeepsStartForNonEmptyWorkspaceCandidate(t *testing.T) {
	status := emptyWorkspaceCandidateStatus()
	status.Projection.Paths = []string{"internal/cli/review_next_transition.go"}
	status.Projection.CurrentCandidateTree = strings.Repeat("d", 40)

	got := newReviewNextTransition(status, nil, nil, nil, nil, reviewNextTransitionInput{StartLineage: "review-empty-candidate"})

	if got.Kind != reviewNextTransitionExecute || got.ReasonCode != "fresh_target_ready" {
		t.Fatalf("non-empty fresh candidate transition = %q/%q, want %q/%q", got.Kind, got.ReasonCode, reviewNextTransitionExecute, "fresh_target_ready")
	}
	if got.Execute == nil || got.Execute.Operation != "review.start" {
		t.Fatalf("non-empty fresh candidate execute = %#v, want review.start", got.Execute)
	}
}
