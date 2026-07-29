package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// writeReviewStartSizedCandidate stages one deterministic text candidate whose
// rendered diff lands on the requested side of MaxFrozenCandidateDiffBytes. It
// returns the exact byte size of the diff START must render, measured with the
// same bounded capture the product uses, so these tests assert against the real
// boundary instead of an assumed one.
func writeReviewStartSizedCandidate(t *testing.T, repo string, lines int) int {
	t.Helper()
	var builder strings.Builder
	for index := 0; index < lines; index++ {
		fmt.Fprintf(&builder, "line %08d payload aaaaaaaaaaaaaaaaaaaa\n", index)
	}
	if err := os.WriteFile(filepath.Join(repo, "big.txt"), []byte(builder.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "big.txt")
	measured := runReviewCLIGitOutput(t, repo, "diff", "--cached", "--binary", "--full-index",
		"--no-color", "--no-renames", "--no-ext-diff", "--no-textconv")
	return len(measured)
}

// TestReviewStartRefusesOversizedCandidateBeforeCommittingAuthority pins the
// release criterion this defect broke: START may not admit a candidate whose
// frozen reviewer context cannot be rendered. The unnegotiated form is the one
// that regressed, because it skipped the pre-mutation render entirely and left
// an active authority that STATUS could only answer with a stop.
func TestReviewStartRefusesOversizedCandidateBeforeCommittingAuthority(t *testing.T) {
	repo := initReviewCLIRepo(t)
	rendered := writeReviewStartSizedCandidate(t, repo, 110000)
	if rendered <= reviewtransaction.MaxFrozenCandidateDiffBytes {
		t.Fatalf("fixture diff = %d bytes, want more than the %d-byte bound", rendered, reviewtransaction.MaxFrozenCandidateDiffBytes)
	}

	var output bytes.Buffer
	err := RunReview([]string{"start", "--cwd", repo, "--lineage", "review-start-oversized"}, &output)
	if err == nil {
		t.Fatalf("unnegotiated START admitted an unrenderable candidate:\n%s", output.String())
	}

	stores, storeErr := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	if len(stores) != 0 {
		t.Fatalf("refused START persisted authority stores: %#v", stores)
	}

	message := err.Error()
	for _, required := range []string{
		strconv.Itoa(reviewtransaction.MaxFrozenCandidateDiffBytes),
		"smaller candidates",
	} {
		if !strings.Contains(message, required) {
			t.Fatalf("refusal does not name %q:\n%s", required, message)
		}
	}
	if !strings.Contains(message, strconv.Itoa(rendered)) {
		t.Fatalf("refusal does not name the rendered size %d:\n%s", rendered, message)
	}
}

// TestNegotiatedReviewStartOversizedRefusalNamesBoundAndWayOut asserts the
// typed negotiated envelope carries the same numbers and the same remedy, so a
// machine consumer is not handed a bare code it cannot act on.
func TestNegotiatedReviewStartOversizedRefusalNamesBoundAndWayOut(t *testing.T) {
	repo := initReviewCLIRepo(t)
	rendered := writeReviewStartSizedCandidate(t, repo, 110000)

	var output bytes.Buffer
	err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", "review-start-oversized-negotiated",
	}), &output)
	if err == nil {
		t.Fatalf("negotiated START admitted an unrenderable candidate:\n%s", output.String())
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Phase != "pre_native" || failure.MutationOutcome != ReviewMutationNotStarted || failure.NextAction != "stop" {
		t.Fatalf("oversized negotiated failure = %#v", failure)
	}
	// The remedy is only useful if the envelope carrying it stays valid; the
	// contract caps a failure message at 240 bytes.
	if err := failure.Validate(); err != nil {
		t.Fatalf("oversized negotiated failure envelope is invalid: %v\nmessage (%d bytes) = %s", err, len(failure.Message), failure.Message)
	}
	for _, required := range []string{
		strconv.Itoa(reviewtransaction.MaxFrozenCandidateDiffBytes),
		strconv.Itoa(rendered),
		"smaller candidates",
	} {
		if !strings.Contains(failure.Message, required) {
			t.Fatalf("negotiated refusal message does not name %q:\n%s", required, failure.Message)
		}
	}
	stores, storeErr := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	if len(stores) != 0 {
		t.Fatalf("refused negotiated START persisted authority stores: %#v", stores)
	}
}

// TestReviewStartAdmitsCandidateUnderBoundAndStatusYieldsCollect is the guard
// against a guard that passes by refusing everything: a candidate just under
// the bound must still start, and its STATUS must produce real reviewer work.
func TestReviewStartAdmitsCandidateUnderBoundAndStatusYieldsCollect(t *testing.T) {
	repo := initReviewCLIRepo(t)
	rendered := writeReviewStartSizedCandidate(t, repo, 95000)
	if rendered >= reviewtransaction.MaxFrozenCandidateDiffBytes {
		t.Fatalf("fixture diff = %d bytes, want less than the %d-byte bound", rendered, reviewtransaction.MaxFrozenCandidateDiffBytes)
	}

	var started bytes.Buffer
	if err := RunReview([]string{"start", "--cwd", repo, "--lineage", "review-start-under-bound"}, &started); err != nil {
		t.Fatalf("START refused a renderable candidate: %v\n%s", err, started.String())
	}

	var status bytes.Buffer
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--next-transition",
	}, &status); err != nil {
		t.Fatal(err)
	}
	var result ReviewTargetStatusResult
	if err := json.Unmarshal(status.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.NextTransition == nil || result.NextTransition.Kind != reviewNextTransitionCollect ||
		result.NextTransition.ReasonCode != "reviewer_results_required" {
		t.Fatalf("under-bound STATUS transition = %#v", result.NextTransition)
	}
	if result.NextTransition.Collect == nil || len(result.NextTransition.Collect.Inputs) == 0 {
		t.Fatalf("under-bound STATUS produced no reviewer collection context: %#v", result.NextTransition)
	}
	for _, input := range result.NextTransition.Collect.Inputs {
		if input.CandidateDiff == nil || input.ChangedPathManifest == nil || input.ArtifactSubject == nil {
			t.Fatalf("collect input is missing frozen reviewer context: %#v", input)
		}
	}
}

// TestStuckOversizedReviewingLineageExitsThroughAbandon covers the operators
// who already carry an authority created before this guard existed. The stuck
// authority is persisted the way the defective START persisted it -- with no
// pre-creation render -- so the exit is verified against the real dead end.
func TestStuckOversizedReviewingLineageExitsThroughAbandon(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartSizedCandidate(t, repo, 110000)
	ctx := context.Background()
	lineage := "review-stuck-oversized"

	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(ctx, reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionStaged, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := builder.AssessSnapshotRisk(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses, err := facadeSelectedLenses(assessment, "reliability")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := facadePolicyBytes("")
	if err != nil {
		t.Fatal(err)
	}
	changedLines := assessment.ChangedLines
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: facadePayloadHash(policy), RiskLevel: assessment.Level,
		SelectedLenses: lenses, OriginalChangedLines: &changedLines,
	})
	if err != nil {
		t.Fatal(err)
	}
	started, err := reviewtransaction.StartCompactAuthority(ctx, repo, reviewtransaction.CompactStartRequest{
		State: state, ExplicitLineage: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	var stuck bytes.Buffer
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--projection", "staged", "--next-transition",
	}, &stuck); err != nil {
		t.Fatal(err)
	}
	var stuckResult ReviewTargetStatusResult
	if err := json.Unmarshal(stuck.Bytes(), &stuckResult); err != nil {
		t.Fatal(err)
	}
	if stuckResult.NextTransition == nil || stuckResult.NextTransition.Kind != reviewNextTransitionStop ||
		stuckResult.NextTransition.ReasonCode != "captured_artifacts_unverifiable" {
		t.Fatalf("stuck lineage STATUS transition = %#v", stuckResult.NextTransition)
	}

	reason := "oversized candidate cannot render the frozen reviewer context"
	actor := "maintainer@example.com"
	authorization := strings.Join([]string{
		"gentle-ai.review-abandon-authorization/v1",
		"lineage=" + lineage,
		"revision=" + started.Record.Revision,
		"snapshot_identity=" + snapshot.Identity,
		"actor=" + actor,
		"reason=" + reason,
	}, "\n")
	var abandoned bytes.Buffer
	if err := RunReview([]string{
		"abandon", "--cwd", repo, "--lineage", lineage, "--expected-revision", started.Record.Revision,
		"--reason", reason, "--actor", actor, "--maintainer-authorization", authorization,
	}, &abandoned); err != nil {
		t.Fatalf("abandon did not free the stuck lineage: %v\n%s", err, abandoned.String())
	}

	var freed bytes.Buffer
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--projection", "staged", "--next-transition",
	}, &freed); err != nil {
		t.Fatal(err)
	}
	var freedResult ReviewTargetStatusResult
	if err := json.Unmarshal(freed.Bytes(), &freedResult); err != nil {
		t.Fatal(err)
	}
	if freedResult.NextTransition == nil || freedResult.NextTransition.Kind != reviewNextTransitionExecute ||
		freedResult.NextTransition.ReasonCode != "fresh_target_ready" {
		t.Fatalf("STATUS after abandon = %#v", freedResult.NextTransition)
	}
}
