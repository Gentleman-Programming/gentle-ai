package cli

import (
	"context"
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
func targetedValidatorAdmissionRequest(t *testing.T) reviewProviderTargetedValidatorRequest {
	t.Helper()
	reviewEnabledHome(t)
	repo, lineage, _ := providerCorrectionReadyWithoutVerificationEvidence(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	correction, err := reviewProviderTargetedValidatorCorrection(context.Background(), repo, record.State)
	if err != nil {
		t.Fatal(err)
	}
	request, err := reviewProviderNewTargetedValidatorRequest(context.Background(), repo, record.State, record.State.CapturePhaseRevision, correction)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestTargetedValidatorRawAdmissionAllowsAdditiveTopLevelFields(t *testing.T) {
	request := targetedValidatorAdmissionRequest(t)
	valid := providerTargetedValidationPayload(t, request.ValidationRequest)
	payload := append(append([]byte(nil), valid[:len(valid)-1]...), []byte(`,"passed_note_unused":"provider diagnostic"}`)...)

	result, native, err := reviewProviderAdmitTargetedValidatorRaw(request, payload)
	if err != nil {
		t.Fatalf("admit targeted validator result with an additive top-level field: %v", err)
	}
	if !result.OriginalCriteria.Passed || !result.CorrectionRegression.Passed ||
		!native.OriginalCriteria.Passed || !native.CorrectionRegression.Passed {
		t.Fatalf("admitted targeted validator verdict = result=%#v native=%#v, want both checks passed", result, native)
	}
}

func TestTargetedValidatorRawAdmissionRejectsOmittedPassed(t *testing.T) {
	request := targetedValidatorAdmissionRequest(t)
	payload := []byte(strings.Replace(string(providerTargetedValidationPayload(t, request.ValidationRequest)), `"passed":true,`, "", 1))

	if _, _, err := reviewProviderAdmitTargetedValidatorRaw(request, payload); err == nil || !strings.Contains(err.Error(), "requires passed checks") {
		t.Fatalf("admit targeted validator result without required passed = %v, want rejection", err)
	}
}

func TestTargetedValidatorRawAdmissionRejectsUnknownNestedFields(t *testing.T) {
	request := targetedValidatorAdmissionRequest(t)
	valid := string(providerTargetedValidationPayload(t, request.ValidationRequest))
	payload := strings.Replace(valid, `"original_criteria":{"passed":true,`, `"original_criteria":{"passed":true,"passed_note_unused":"nested",`, 1)
	if payload == valid {
		t.Fatal("could not add an unknown nested field to the targeted validator result")
	}

	if _, _, err := reviewProviderAdmitTargetedValidatorRaw(request, []byte(payload)); err == nil || !strings.Contains(err.Error(), `unknown field "passed_note_unused"`) {
		t.Fatalf("admit targeted validator result with an unknown nested field = %v, want rejection", err)
	}
}

// TestTargetedValidatorRawAdmissionRejectsRegressionFalseWithoutRegressions is
// the RED-first proof for issue #4214: a targeted validator reported
// correction_regression.passed=false while every evidence string said there
// was no regression, and admission escalated the lineage to terminal
// native_stop_required on the false alarm. Raw admission must instead refuse
// with a plain (non-inconclusive) error so the compiled-runtime capture
// spends its existing one-shot corrective retry on it.
func TestTargetedValidatorRawAdmissionRejectsRegressionFalseWithoutRegressions(t *testing.T) {
	request := targetedValidatorAdmissionRequest(t)
	valid := string(providerTargetedValidationPayload(t, request.ValidationRequest))
	original := `"correction_regression":{"passed":true,"evidence":["correction regression passed"]}`
	// An empty {} or blank-claim entry must also be refused (issue #4214).
	refused := func(replacement string) {
		payload := strings.Replace(valid, original, replacement, 1)
		if payload == valid {
			t.Fatal("could not flip correction_regression to a failed verdict")
		}
		if _, _, err := reviewProviderAdmitTargetedValidatorRaw(request, []byte(payload)); err == nil {
			t.Fatal("admitted an inconsistent regression verdict")
		} else if errors.Is(err, errReviewTargetedValidationInconclusive) {
			t.Fatalf("regression-verdict inconsistency was misclassified as inconclusive: %v", err)
		}
	}
	refused(`"correction_regression":{"passed":false,"evidence":["there was no regression in the corrected candidate"]}`)
	refused(`"correction_regression":{"passed":false,"evidence":["saw a regression"],"regressions":[{}]}`)
	refused(`"correction_regression":{"passed":false,"evidence":["saw a regression"],"regressions":[{"location":"internal/example.go:1","claim":"   ","proof_refs":["ref"]}]}`)
}

func TestTargetedValidatorEvidenceRejectsDigestAndBindingMismatch(t *testing.T) {
	request := reviewtransaction.TargetedValidationRequest{
		RequestHash:              "sha256:" + strings.Repeat("1", 64),
		CorrectionTargetIdentity: "sha256:" + strings.Repeat("2", 64),
	}
	result := facadeValidationResult{
		TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: request.CorrectionTargetIdentity,
		OriginalCriteria:     facadeValidationCheck{Passed: false, Evidence: []string{"the corrected candidate still fails the original criterion"}},
		CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"the correction introduced no unrelated regression"}},
		FollowUps:            []reviewtransaction.FollowUp{},
	}
	native := reviewtransaction.ScopedValidationResult{
		TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: request.CorrectionTargetIdentity,
		OriginalCriteria: reviewtransaction.ValidationCheck{
			Passed: result.OriginalCriteria.Passed, EvidenceHash: facadeValueHash("original-criteria", result.OriginalCriteria),
		},
		CorrectionRegression: reviewtransaction.ValidationCheck{
			Passed: result.CorrectionRegression.Passed, EvidenceHash: facadeValueHash("correction-regression", result.CorrectionRegression),
		},
		FollowUps: result.FollowUps,
	}
	evidence := reviewProviderTargetedValidatorEvidence(result)
	if err := evidence.Validate(request, native); err != nil {
		t.Fatalf("valid targeted-validator evidence rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*reviewtransaction.CompactTargetedValidatorEvidence)
	}{
		{name: "digest", mutate: func(value *reviewtransaction.CompactTargetedValidatorEvidence) {
			value.OriginalCriteria.Evidence[0] = "different observation"
		}},
		{name: "binding", mutate: func(value *reviewtransaction.CompactTargetedValidatorEvidence) {
			value.CorrectionTargetIdentity = "sha256:" + strings.Repeat("4", 64)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := evidence
			candidate.OriginalCriteria.Evidence = append([]string(nil), evidence.OriginalCriteria.Evidence...)
			tt.mutate(&candidate)
			if err := candidate.Validate(request, native); err == nil {
				t.Fatal("mismatched targeted-validator evidence was admitted")
			}
		})
	}
}

func TestTargetedValidatorCaptureRejectsOutcomeOnlyTerminalFailureBeforeAuthorityMutation(t *testing.T) {
	reviewEnabledHome(t)
	repo, lineage, request := providerCorrectionReadyWithoutVerificationEvidence(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CaptureAdmittedTargetedValidatorResult(t.Context(), reviewtransaction.CompactAdmittedTargetedValidatorResultRequest{
		ExpectedRequest: request, Payload: []byte(`{"outcome":"failed"}`),
		Complete: func(*reviewtransaction.CompactState) error { return nil },
	}); err == nil {
		t.Fatal("outcome-only terminal failed capture was admitted")
	}
	after, err := store.Load()
	if err != nil || after.Revision != before.Revision || len(after.State.AdmittedRoleResults) != len(before.State.AdmittedRoleResults) {
		t.Fatalf("outcome-only terminal capture mutated authority: before=%#v after=%#v err=%v", before, after, err)
	}
}

func TestFacadeValidationKeepsGenuineFailedVerdicts(t *testing.T) {
	request := reviewtransaction.TargetedValidationRequest{
		RequestHash:              "sha256:" + strings.Repeat("1", 64),
		CorrectionTargetIdentity: "sha256:" + strings.Repeat("2", 64),
	}
	result := facadeValidationResult{
		OriginalCriteria: facadeValidationCheck{Passed: true, Evidence: []string{"original acceptance criteria re-ran and passed"}},
		CorrectionRegression: facadeValidationCheck{Passed: false, Evidence: []string{"regression TestReviewStatus failed: exit status 1 on the corrected candidate"},
			Regressions: []reviewtransaction.Regression{{
				Location: "internal/review_status_test.go:42", Claim: "the correction reintroduced the flaky race in TestReviewStatus",
				ProofRefs: []string{"internal/review_status_test.go:42 exit status 1 on the corrected candidate"},
			}},
		},
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

// TestFacadeValidationKeepsGenuineFailedVerdictsWithRegressions extends the
// fixture above (issue #4214): a failed correction_regression verdict that
// names its regressions must still compact successfully, while the same
// verdict without any named regression -- the exact defect #4214 reported,
// where every evidence string claimed no regression yet passed was false --
// must be refused instead of silently admitted and escalated.
func TestFacadeValidationKeepsGenuineFailedVerdictsWithRegressions(t *testing.T) {
	request := reviewtransaction.TargetedValidationRequest{
		RequestHash:              "sha256:" + strings.Repeat("1", 64),
		CorrectionTargetIdentity: "sha256:" + strings.Repeat("2", 64),
	}
	base := facadeValidationResult{
		OriginalCriteria:              facadeValidationCheck{Passed: true, Evidence: []string{"original acceptance criteria re-ran and passed"}},
		TargetedValidationRequestHash: request.RequestHash,
		CorrectionTargetIdentity:      request.CorrectionTargetIdentity,
	}

	named := base
	named.CorrectionRegression = facadeValidationCheck{
		Passed: false, Evidence: []string{"there was no regression in the corrected candidate; the following was nonetheless observed"},
		Regressions: []reviewtransaction.Regression{{
			ID: "R4-4214", Location: "internal/example.go:12", Claim: "the fix reintroduced the TOCTOU window",
			ProofRefs: []string{"internal/example.go:12 lock released before the guarded read"},
		}},
	}
	if _, err := named.compact("sha256:"+strings.Repeat("3", 64), []string{"finding-1"}, request); err != nil {
		t.Fatalf("compact conversion rejected a genuine failed verdict with a named regression: %v", err)
	}

	unnamed := base
	unnamed.CorrectionRegression = facadeValidationCheck{Passed: false, Evidence: []string{"there was no regression"}}
	if _, err := unnamed.compact("sha256:"+strings.Repeat("3", 64), []string{"finding-1"}, request); err == nil || !strings.Contains(err.Error(), "without naming any regression") {
		t.Fatalf("compact conversion admitted a regression-failed verdict without any named regression: %v", err)
	}

	// compact() must also refuse an empty {} or blank-claim entry.
	for _, blank := range []reviewtransaction.Regression{
		{}, {Location: "internal/example.go:12", Claim: "  ", ProofRefs: []string{"ref"}},
	} {
		incomplete := base
		incomplete.CorrectionRegression = facadeValidationCheck{Passed: false, Evidence: []string{"saw a regression"}, Regressions: []reviewtransaction.Regression{blank}}
		if _, err := incomplete.compact("sha256:"+strings.Repeat("3", 64), []string{"finding-1"}, request); err == nil {
			t.Fatal("compact conversion admitted an incompletely named regression entry")
		}
	}
}
