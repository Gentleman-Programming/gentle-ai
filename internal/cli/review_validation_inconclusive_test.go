package cli

import (
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
