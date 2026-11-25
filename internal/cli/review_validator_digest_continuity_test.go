package cli

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/reviewtransaction"
)

func TestTargetedValidatorDigestContinuity(t *testing.T) {
	request := reviewtransaction.TargetedValidationRequest{
		RequestHash:              "sha256:" + strings.Repeat("1", 64),
		CorrectionTargetIdentity: "sha256:" + strings.Repeat("2", 64),
	}
	digest := func(domain, wire string) string {
		return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("gentle-ai.facade-"+domain+"/v1\x00"+wire)))
	}
	wantOriginal := digest("original-criteria", `{"passed":true,"evidence":["marker observed"]}`)
	wantRegression := digest("correction-regression", `{"passed":false,"evidence":["consumer failed"],"regressions":[{"location":"tracked.txt:5","claim":"marker broke consumer","proof_refs":["frozen diff"]}]}`)
	for _, name := range []string{"legacy", "completed"} {
		t.Run(name, func(t *testing.T) {
			result := facadeValidationResult{
				TargetedValidationRequestHash: request.RequestHash,
				CorrectionTargetIdentity:      request.CorrectionTargetIdentity,
				OriginalCriteria:              facadeValidationCheck{Passed: true, Evidence: []string{"marker observed"}},
				CorrectionRegression: facadeValidationCheck{
					Evidence:    []string{"consumer failed"},
					Regressions: []reviewtransaction.Regression{{Location: "tracked.txt:5", Claim: "marker broke consumer", ProofRefs: []string{"frozen diff"}}},
				},
				FollowUps: []reviewtransaction.FollowUp{{Observation: "inspect other consumers", ProofRefs: []string{"frozen diff"}}},
			}
			if name == "completed" {
				result.OriginalCriteria.Inspection = &reviewtransaction.ValidationInspection{Status: reviewtransaction.ValidationInspectionCompleted}
				result.CorrectionRegression.Inspection = result.OriginalCriteria.Inspection
			}
			got, err := result.compact("sha256:"+strings.Repeat("3", 64), []string{"finding-1"}, request)
			if err != nil {
				t.Fatal(err)
			}
			if got.OriginalCriteria.EvidenceHash != wantOriginal || got.CorrectionRegression.EvidenceHash != wantRegression {
				t.Fatalf("original=%s (want %s), regression=%s (want %s)", got.OriginalCriteria.EvidenceHash, wantOriginal, got.CorrectionRegression.EvidenceHash, wantRegression)
			}
			if err := reviewProviderTargetedValidatorEvidence(result).Validate(request, got); err != nil {
				t.Fatal(err)
			}
			evidence := reviewProviderTargetedValidatorEvidence(result)
			evidence.CorrectionRegression.Regressions[0].Claim = "different regression"
			if err := evidence.Validate(request, got); err == nil || err.Error() != "admitted targeted-validator evidence does not match its request and digests" {
				t.Fatalf("tampered regression: %v", err)
			}
		})
	}
}
