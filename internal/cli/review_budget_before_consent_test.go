package cli

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// TestRelayedStartRefusesOverBudgetCandidateBeforeConsent pins issue #4790:
// a START that declares it can relay the consent question must not ask the
// human about a candidate whose reviewer evidence cannot fit the runtime
// budget. Granting would only reach the same typed refusal, so the relay
// START returns that refusal itself, with no consent envelope and no write.
func TestRelayedStartRefusesOverBudgetCandidateBeforeConsent(t *testing.T) {
	home := reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	stubReviewConsole(t, false, "")
	// The shell script makes the candidate high risk so consent is required;
	// the large file makes its reviewer evidence exceed the runtime budget.
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)
	writeRuntimeBudgetOverCandidate(t, repo)
	authorityRoot := reviewCLIAuthorityRoot(t, repo)
	authorityBefore := snapshotAuthorityTree(t, authorityRoot)
	homeBefore := readLegacyAuthorityTree(t, home)

	var output bytes.Buffer
	err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", "review-budget-before-consent", "--consent", "relay",
	}), &output)
	if err == nil || !strings.Contains(err.Error(), "lens_context_budget_exceeded") {
		t.Fatalf("over-budget relay START error = %v\n%s", err, output.String())
	}
	if strings.Contains(output.String(), "consent_required") || strings.Contains(output.String(), ReviewIntegrationConsentSchema) {
		t.Fatalf("over-budget relay START asked for consent before refusing:\n%s", output.String())
	}
	var failure *ReviewIntegrationFailureError
	if !errors.As(err, &failure) {
		t.Fatalf("over-budget relay START did not emit a typed negotiated failure: %T", err)
	}
	if failure.Failure.MutationOutcome != ReviewMutationNotStarted || failure.Failure.Phase != "preflight" ||
		failure.Failure.NextAction != "stop" || failure.Failure.Code != "lens_context_budget_exceeded" {
		t.Fatalf("over-budget relay START envelope does not report a refusal that wrote nothing: %#v", failure.Failure)
	}
	if after := snapshotAuthorityTree(t, authorityRoot); authorityBefore != after {
		t.Fatalf("over-budget relay START changed authority storage:\nbefore:\n%s\nafter:\n%s", authorityBefore, after)
	}
	if after := readLegacyAuthorityTree(t, home); !reflect.DeepEqual(homeBefore, after) {
		t.Fatalf("over-budget relay START persisted an artifact: before=%#v after=%#v", homeBefore, after)
	}
}

// TestRelayedStartStillAsksConsentForCandidateWithinBudget pins the unchanged
// path: a high-risk candidate that fits the budget still receives the
// blocking consent question from a relay START.
func TestRelayedStartStillAsksConsentForCandidateWithinBudget(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)
	writeRuntimeBudgetUnderCandidate(t, repo)

	output := runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", "review-budget-fits-consent", "--consent", "relay",
	}))
	question := decodeConsentQuestion(t, output.Bytes())
	if err := question.Validate(); err != nil {
		t.Fatalf("consent question does not validate: %v\n%s", err, output.String())
	}
	if question.Action != "consent_required" || !question.Blocking {
		t.Fatalf("within-budget relay START did not ask for consent: %#v", question)
	}
}
