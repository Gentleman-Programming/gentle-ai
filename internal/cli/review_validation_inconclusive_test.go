package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// A scoped-fix validator that could not inspect the immutable corrected
// candidate produced no verdict: recording its check as failed would consume
// the single correction attempt on a non-observation, and recording it as
// passed would approve without inspection (issue #1309 follow-up).
func TestFacadeValidationRejectsInconclusiveEvidence(t *testing.T) {
	request := reviewtransaction.TargetedValidationRequest{
		RequestHash:              "sha256:" + strings.Repeat("1", 64),
		CorrectionTargetIdentity: "sha256:" + strings.Repeat("2", 64),
	}
	conclusive := facadeValidationCheck{Passed: true, Evidence: []string{"go test ./internal/... passed for the corrected candidate"}}

	tests := []struct {
		name  string
		check facadeValidationCheck
	}{
		{name: "failed without access", check: facadeValidationCheck{Passed: false, Evidence: []string{"original criteria could not be verified: permission denied reading the corrected diff"}}},
		{name: "failed candidate unavailable", check: facadeValidationCheck{Passed: false, Evidence: []string{"immutable candidate unavailable to the validator process"}}},
		{name: "passed without inspection", check: facadeValidationCheck{Passed: true, Evidence: []string{"assumed clean because the candidate was not inspected"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, result := range []facadeValidationResult{
				{OriginalCriteria: tt.check, CorrectionRegression: conclusive,
					TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: request.CorrectionTargetIdentity},
				{OriginalCriteria: conclusive, CorrectionRegression: tt.check,
					TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: request.CorrectionTargetIdentity},
			} {
				if _, err := result.compact("sha256:"+strings.Repeat("3", 64), []string{"finding-1"}, request); err == nil || !strings.Contains(err.Error(), "inconclusive") {
					t.Fatalf("compact conversion admitted an inconclusive validation check: %v", err)
				}
				if _, err := result.native(reviewtransaction.Transaction{}); err == nil || !strings.Contains(err.Error(), "inconclusive") {
					t.Fatalf("native conversion admitted an inconclusive validation check: %v", err)
				}
			}
		})
	}
}

// A genuinely failed check with observed evidence is a real verdict and must
// keep flowing into escalation unchanged.
func TestFacadeValidationKeepsGenuineFailedVerdicts(t *testing.T) {
	request := reviewtransaction.TargetedValidationRequest{
		RequestHash:              "sha256:" + strings.Repeat("1", 64),
		CorrectionTargetIdentity: "sha256:" + strings.Repeat("2", 64),
	}
	result := facadeValidationResult{
		OriginalCriteria:              facadeValidationCheck{Passed: true, Evidence: []string{"original acceptance criteria re-ran and passed"}},
		CorrectionRegression:          facadeValidationCheck{Passed: false, Evidence: []string{"regression TestReviewStatus failed: exit status 1 on the corrected candidate"}},
		TargetedValidationRequestHash: request.RequestHash,
		CorrectionTargetIdentity:      request.CorrectionTargetIdentity,
	}
	converted, err := result.compact("sha256:"+strings.Repeat("3", 64), []string{"finding-1"}, request)
	if err != nil {
		t.Fatalf("compact conversion rejected a genuine failed verdict: %v", err)
	}
	if converted.CorrectionRegression.Passed {
		t.Fatal("genuine failed verdict was not preserved")
	}
}

// A validator that rephrases "could not inspect" to the passive "could not
// be inspected" must still be recognized as inconclusive (issue #3378).
func TestFacadeValidationRejectsPassiveInconclusivePhrasing(t *testing.T) {
	request := reviewtransaction.TargetedValidationRequest{
		RequestHash:              "sha256:" + strings.Repeat("1", 64),
		CorrectionTargetIdentity: "sha256:" + strings.Repeat("2", 64),
	}
	conclusive := facadeValidationCheck{Passed: true, Evidence: []string{"go test ./internal/... passed for the corrected candidate"}}

	passivePhrases := []string{
		"the immutable candidate could not be inspected",
		"unable to be inspected by the validator",
		"the corrected candidate was not able to inspect",
	}
	for _, phrase := range passivePhrases {
		t.Run(phrase, func(t *testing.T) {
			inconclusive := facadeValidationCheck{Passed: false, Evidence: []string{phrase}}
			result := facadeValidationResult{
				OriginalCriteria:              inconclusive,
				CorrectionRegression:          conclusive,
				TargetedValidationRequestHash: request.RequestHash,
				CorrectionTargetIdentity:      request.CorrectionTargetIdentity,
			}
			if _, err := result.compact("sha256:"+strings.Repeat("3", 64), []string{"finding-1"}, request); err == nil || !strings.Contains(err.Error(), "inconclusive") {
				t.Fatalf("compact admitted passive inconclusive phrasing %q: %v", phrase, err)
			}
		})
	}
}

// The STATUS path must not treat an inconclusive validation result as
// terminally corrupt (issue #3378). The routing branch must classify the
// error and emit a non-terminal stop so the validator can retry.
func TestReviewNextTransitionClassifiesInconclusiveArtifactError(t *testing.T) {
	inconclusiveErr := errors.New("targeted validation is inconclusive: original_criteria evidence reports the immutable candidate could not be inspected, so no verdict was produced and the correction attempt was not consumed")
	transition := newReviewNextTransition(
		ReviewTargetStatusResult{
			Applicability: reviewtransaction.TargetApplicabilityCurrent,
			Authority:     &ReviewTargetStatusAuthority{State: reviewtransaction.StateReviewing},
		},
		nil, nil, nil, inconclusiveErr, reviewNextTransitionInput{},
	)
	if transition.Kind != reviewNextTransitionStop {
		t.Fatalf("transition kind = %v, want stop", transition.Kind)
	}
	if transition.ReasonCode != "targeted_validation_inconclusive" {
		t.Fatalf("reason code = %q, want targeted_validation_inconclusive", transition.ReasonCode)
	}

	corruptErr := errors.New("captured provider targeted validator result is no longer admitted: decode provider targeted validator result: invalid character")
	transition = newReviewNextTransition(
		ReviewTargetStatusResult{
			Applicability: reviewtransaction.TargetApplicabilityCurrent,
			Authority:     &ReviewTargetStatusAuthority{State: reviewtransaction.StateReviewing},
		},
		nil, nil, nil, corruptErr, reviewNextTransitionInput{},
	)
	if transition.Kind != reviewNextTransitionStop {
		t.Fatalf("transition kind = %v, want stop", transition.Kind)
	}
	if transition.ReasonCode != "captured_artifacts_unverifiable" {
		t.Fatalf("reason code = %q, want captured_artifacts_unverifiable", transition.ReasonCode)
	}
}
