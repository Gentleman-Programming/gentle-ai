package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// approveTrackedGoChangeWithAdvisoryFindings persists an approved compact
// authority whose completed review froze one WARNING and one SUGGESTION.
// Neither severity is candidate-causal-severe, so both route to the
// informational outcome, follow_ups stays empty, and the approval stands --
// exactly the shape of the field incident this file covers.
func approveTrackedGoChangeWithAdvisoryFindings(t *testing.T, repo, lineage string) reviewtransaction.CompactState {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "feature.go"), []byte("package feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "feature.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "add feature base")
	if err := os.WriteFile(filepath.Join(repo, "feature.go"), []byte("package feature\n\nfunc Feature() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(context.Background(), reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := builder.ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1, Snapshot: snapshot,
		PolicyHash: "sha256:" + hexDigest("advisory-policy"), RiskLevel: risk,
		SelectedLenses: []string{reviewtransaction.LensReliability}, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	findings := []reviewtransaction.Finding{
		{
			ID: "R3-001", Lens: "reliability", Location: "feature.go:3", Severity: "WARNING",
			Claim: "Feature() has no covering assertion", ProofRefs: []string{"feature.go:3 has no differential test"},
		},
		{
			ID: "R3-002", Lens: "reliability", Location: "feature.go:3", Severity: "SUGGESTION",
			Claim: "Feature() could return a named constant", ProofRefs: []string{"feature.go:3 returns a bare literal"},
		},
	}
	results := []reviewtransaction.LensResult{{
		Lens: reviewtransaction.LensReliability, Findings: findings, Evidence: []string{"review completed"},
	}}
	if err := state.CompleteReview(reviewtransaction.CompactReviewInput{
		LensResults: results, Classifications: []reviewtransaction.FindingEvidence{}, RefuterOutcomes: []reviewtransaction.EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Replace(revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("independent verification passed\n"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-verification", state); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	if state.State != reviewtransaction.StateApproved || len(state.FollowUps) != 0 || len(state.FixFindingIDs) != 0 {
		t.Fatalf("advisory fixture must approve with no follow-up and no correction: %#v", state)
	}
	return state
}

// TestApprovedFinalizeDeclaresAdvisoryFindings is the regression for the field
// incident: an approved receipt carried a WARNING, the consumer had only the
// severity string to reason from, inferred it had to act, and re-ran a full
// review on an already-approved candidate. The approved payload must say the
// disposition out loud instead.
func TestApprovedFinalizeDeclaresAdvisoryFindings(t *testing.T) {
	repo := initReviewCLIRepo(t)
	state := approveTrackedGoChangeWithAdvisoryFindings(t, repo, "advisory-legacy")

	var stdout bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", state.LineageID}, &stdout); err != nil {
		t.Fatalf("finalize approved lineage: %v", err)
	}
	var result ReviewFacadeFinalizeResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode finalize result %q: %v", stdout.String(), err)
	}
	if result.State != reviewtransaction.StateApproved {
		t.Fatalf("finalize state = %q, want approved", result.State)
	}
	if result.AdvisoryFindings == nil {
		t.Fatalf("approved finalize payload carries no advisory_findings block: %s", stdout.String())
	}
	statement := result.AdvisoryFindings.Statement
	for _, want := range []string{"approved", "non-blocking", "no correction"} {
		if !strings.Contains(strings.ToLower(statement), want) {
			t.Fatalf("advisory statement %q must state %q", statement, want)
		}
	}
	got := map[string]string{}
	for _, finding := range result.AdvisoryFindings.Findings {
		got[finding.ID] = string(finding.Disposition)
		if finding.Severity == "" {
			t.Fatalf("advisory finding %q must keep its severity: %#v", finding.ID, finding)
		}
	}
	want := map[string]string{
		"R3-001": string(reviewtransaction.AdvisoryInformational),
		"R3-002": string(reviewtransaction.AdvisoryInformational),
	}
	if len(got) != len(want) {
		t.Fatalf("advisory findings = %#v, want %#v", got, want)
	}
	for id, disposition := range want {
		if got[id] != disposition {
			t.Fatalf("advisory disposition for %q = %q, want %q", id, got[id], disposition)
		}
	}
}

// TestApprovedFinalizeAdvisoryFindingsSurfaceOnNegotiatedContract proves the
// same disposition reaches the negotiated v2 consumers (gentle-pi and every
// other host reading the published operation envelope), not only the legacy
// bare result.
func TestApprovedFinalizeAdvisoryFindingsSurfaceOnNegotiatedContract(t *testing.T) {
	repo := initReviewCLIRepo(t)
	state := approveTrackedGoChangeWithAdvisoryFindings(t, repo, "advisory-negotiated")

	var stdout bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--lineage", state.LineageID, "--contract", ReviewIntegrationContractV2,
	}, &stdout); err != nil {
		t.Fatalf("negotiated finalize approved lineage: %v", err)
	}
	// gentle-pi and every other host vendor these schemas byte-pinned, so the
	// live envelope carrying the new block must validate against the published
	// contract, not only against the Go struct.
	validatePublishedReviewSchema(t, compileWholePublishedReviewSchema(t, "v2", "operation.schema.json"), stdout.Bytes())

	var envelope ReviewIntegrationOperationResult
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode negotiated envelope %q: %v", stdout.String(), err)
	}
	var result ReviewIntegrationFinalizeResult
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		t.Fatalf("decode negotiated finalize result %q: %v", string(envelope.Result), err)
	}
	if result.State != reviewtransaction.StateApproved || result.AdvisoryFindings == nil {
		t.Fatalf("negotiated approved payload carries no advisory_findings block: %s", stdout.String())
	}
	if len(result.AdvisoryFindings.Findings) != 2 {
		t.Fatalf("negotiated advisory findings = %#v, want both frozen non-blocking findings", result.AdvisoryFindings.Findings)
	}
	for _, finding := range result.AdvisoryFindings.Findings {
		if finding.Disposition != reviewtransaction.AdvisoryInformational {
			t.Fatalf("negotiated advisory disposition for %q = %q, want informational", finding.ID, finding.Disposition)
		}
	}

	// The negative half of the contract: nothing about an advisory finding may
	// offer a way back into review. No correction request, no validation
	// request, and the routed next transition is the delivery gate.
	if result.ValidationRequest != nil {
		t.Fatalf("approved advisory payload offers a validation request: %#v", result.ValidationRequest)
	}
	if result.NextTransition != nil && result.NextTransition.CorrectionRequest != nil {
		t.Fatalf("approved advisory payload offers a correction request: %#v", result.NextTransition.CorrectionRequest)
	}

	runReviewCLIGit(t, repo, "add", "-A")
	var statusOut bytes.Buffer
	if err := RunReviewStatus([]string{
		"--cwd", repo, "--contract", ReviewIntegrationContractV2, "--agent", "claude-code", "--next-transition",
		"--lineage", state.LineageID, "--projection", "staged", "--gate", "pre-commit",
	}, &statusOut); err != nil {
		t.Fatalf("negotiated status after advisory approval: %v", err)
	}
	var status struct {
		NextTransition *ReviewNextTransition `json:"next_transition"`
	}
	if err := json.Unmarshal(statusOut.Bytes(), &status); err != nil {
		t.Fatalf("decode status %q: %v", statusOut.String(), err)
	}
	if status.NextTransition == nil {
		t.Fatalf("status after advisory approval returned no next transition: %s", statusOut.String())
	}
	if status.NextTransition.CorrectionRequest != nil || strings.Contains(status.NextTransition.ReasonCode, "correction") {
		t.Fatalf("advisory approval routed back into correction: %#v", status.NextTransition)
	}
	if status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.validate" {
		t.Fatalf("advisory approval next transition = %#v, want the review.validate delivery gate", status.NextTransition)
	}
}

// TestApprovedFinalizeWithoutFindingsOmitsAdvisoryBlock keeps the addition
// additive: a clean approval's bytes are unchanged, so byte-pinned consumers
// see nothing new until a review actually produces a non-blocking finding.
func TestApprovedFinalizeWithoutFindingsOmitsAdvisoryBlock(t *testing.T) {
	repo := initReviewCLIRepo(t)
	state := approveTrackedGoChangeWithNoFindings(t, repo, "advisory-clean")

	var stdout bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", state.LineageID}, &stdout); err != nil {
		t.Fatalf("finalize clean approved lineage: %v", err)
	}
	if bytes.Contains(stdout.Bytes(), []byte("advisory_findings")) {
		t.Fatalf("clean approval must not carry an advisory block: %s", stdout.String())
	}
}

// approveTrackedGoChangeWithNoFindings is the clean-path twin of the advisory
// fixture: identical lifecycle, zero frozen findings.
func approveTrackedGoChangeWithNoFindings(t *testing.T, repo, lineage string) reviewtransaction.CompactState {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "feature.go"), []byte("package feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "feature.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "add feature base")
	if err := os.WriteFile(filepath.Join(repo, "feature.go"), []byte("package feature\n\nfunc Feature() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(context.Background(), reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := builder.ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1, Snapshot: snapshot,
		PolicyHash: "sha256:" + hexDigest("advisory-policy"), RiskLevel: risk,
		SelectedLenses: []string{reviewtransaction.LensReliability}, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	results := []reviewtransaction.LensResult{{
		Lens: reviewtransaction.LensReliability, Findings: []reviewtransaction.Finding{}, Evidence: []string{"review completed"},
	}}
	if err := state.CompleteReview(reviewtransaction.CompactReviewInput{
		LensResults: results, Classifications: []reviewtransaction.FindingEvidence{}, RefuterOutcomes: []reviewtransaction.EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Replace(revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("independent verification passed\n"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-verification", state); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	return state
}
