package cli

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// runNonBlockingStatus runs the same contracted STATUS with and without the
// --non-blocking-findings opt-in and returns both outputs.
func runNonBlockingStatus(t *testing.T, repo, lineage string) (plain, opted []byte) {
	t.Helper()
	args := []string{"--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2, "--next-transition"}
	var plainOutput, optedOutput bytes.Buffer
	if err := RunReviewStatus(args, &plainOutput); err != nil {
		t.Fatalf("STATUS without opt-in: %v\n%s", err, plainOutput.String())
	}
	if err := RunReviewStatus(append(append([]string{}, args...), "--non-blocking-findings"), &optedOutput); err != nil {
		t.Fatalf("STATUS with opt-in: %v\n%s", err, optedOutput.String())
	}
	return plainOutput.Bytes(), optedOutput.Bytes()
}

// assertOptInOnlyAddsNonBlockingFindings proves the client that did not opt
// in receives exactly what it receives without this change (no key at all),
// and that the opt-in adds that one key and changes nothing else.
func assertOptInOnlyAddsNonBlockingFindings(t *testing.T, plain, opted []byte) *reviewtransaction.NonBlockingFindingSet {
	t.Helper()
	if bytes.Contains(plain, []byte("non_blocking_findings")) {
		t.Fatalf("STATUS without opt-in carries non_blocking_findings:\n%s", plain)
	}
	optedDocument := decodeJSONObjectCopy(t, opted)
	raw, present := optedDocument["non_blocking_findings"]
	delete(optedDocument, "non_blocking_findings")
	if !reflect.DeepEqual(optedDocument, decodeJSONObjectCopy(t, plain)) {
		t.Fatalf("opt-in changed more than non_blocking_findings:\nplain %s\nopted %s", plain, opted)
	}
	if !present {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var set reviewtransaction.NonBlockingFindingSet
	if err := json.Unmarshal(encoded, &set); err != nil {
		t.Fatal(err)
	}
	return &set
}

func assertStatusSchemaKeepsNonBlockingFindingsSeparate(t *testing.T, opted []byte) {
	t.Helper()
	schema := compileWholeNativeStatusSchema(t, "status-v9.schema.json")
	validatePublishedReviewSchema(t, schema, opted)
	for _, key := range []string{"advisory_findings", "reviewer_results"} {
		document := decodeJSONObjectCopy(t, opted)
		document[key] = document["non_blocking_findings"]
		if err := schema.Validate(document); err == nil {
			t.Fatalf("status-v9 schema accepted %s outside approved", key)
		}
	}
	document := decodeJSONObjectCopy(t, opted)
	findings := document["non_blocking_findings"].(map[string]any)["findings"].([]any)
	if len(findings) > 0 {
		delete(findings[0].(map[string]any), "claim")
		if err := schema.Validate(document); err == nil {
			t.Fatal("status-v9 schema accepted a non-blocking finding without its claim")
		}
	}
}

func TestCorrectionRequiredStatusReturnsNonBlockingFindingsOnlyToOptedInClient(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started := startHighRiskCLIReview(t, repo)
	for order := 0; order < len(started.SelectedLenses)-1; order++ {
		captureCleanCLIReviewerResult(t, repo, started, order, &bytes.Buffer{})
	}
	var closure bytes.Buffer
	captureCLIReviewerResultWithFindings(t, repo, started, len(started.SelectedLenses)-1, []facadeFinding{
		{
			ID: "R3-001", Location: "internal/auth/session.go:4", Severity: "CRITICAL",
			Claim: "the candidate introduces an observable authentication failure", ProofRefs: []string{"the changed line deterministically causes the reproduced failure"},
			EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
		},
		{ID: "R3-W01", Location: "internal/auth/session.go:4", Severity: "WARNING", Claim: "the token check could be easier to read", ProofRefs: []string{"the exact changed line was inspected"}},
		{
			ID: "R3-002", Location: "internal/auth/session.go:5", Severity: "CRITICAL",
			Claim: "empty tokens were already accepted before this change", ProofRefs: []string{"the base revision has the same comparison"},
			EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalPreExisting,
		},
	}, &closure)

	// The closure is untouched: no new key, and its schema still refuses
	// reviewer_results outside approved.
	closureDocument := decodeJSONObjectCopy(t, closure.Bytes())
	if closureDocument["state"] != string(reviewtransaction.StateCorrectionRequired) {
		t.Fatalf("closure = %s, want correction_required", closure.Bytes())
	}
	for _, key := range []string{"non_blocking_findings", "advisory_findings", "reviewer_results"} {
		if _, present := closureDocument[key]; present {
			t.Fatalf("correction_required closure carries %s:\n%s", key, closure.Bytes())
		}
	}
	closureDocument["reviewer_results"] = []any{}
	if err := compileWholePublishedReviewSchema(t, "v2", "last-event-closure.schema.json").Validate(closureDocument); err == nil {
		t.Fatal("last-event closure schema accepted reviewer_results outside approved")
	}

	plain, opted := runNonBlockingStatus(t, repo, started.LineageID)
	got := assertOptInOnlyAddsNonBlockingFindings(t, plain, opted)
	want := &reviewtransaction.NonBlockingFindingSet{Statement: reviewtransaction.NonBlockingStatement, Findings: []reviewtransaction.NonBlockingFinding{
		{ID: "R3-002", Lens: "reliability", Location: "internal/auth/session.go:5", Severity: "CRITICAL", Disposition: reviewtransaction.AdvisoryFollowUp, Claim: "empty tokens were already accepted before this change"},
		{ID: "R3-W01", Lens: "reliability", Location: "internal/auth/session.go:4", Severity: "WARNING", Disposition: reviewtransaction.AdvisoryInformational, Claim: "the token check could be easier to read"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("non_blocking_findings = %#v, want %#v", got, want)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, opted, &status)
	if status.NextTransition == nil || status.NextTransition.CorrectionRequest == nil ||
		!reflect.DeepEqual(status.NextTransition.CorrectionRequest.FixFindingIDs, []string{"R3-001"}) {
		t.Fatalf("correction request = %#v, want only R3-001 to correct", status.NextTransition)
	}
	assertStatusSchemaKeepsNonBlockingFindingsSeparate(t, opted)
}
