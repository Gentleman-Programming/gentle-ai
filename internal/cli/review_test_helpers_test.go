package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// writeReviewCLIRawInput writes raw provider role result bytes to a temp file
// and returns its path, for tests driving the `--input=<path>` submission
// form a host-relay runtime uses after it materializes and runs its own
// reviewer (#4611): Go never spawns anything for these tests either.
func writeReviewCLIRawInput(t *testing.T, raw []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "role-result.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// corroborateRefuterClaimsForTest submits the one refuter batch a reviewing
// lineage requires once every lens is captured with a severe candidate-caused
// finding (L20: deterministic findings reach the refuter too when the frozen
// runtime can run it; otherwise it returns a zero closure). It corroborates
// every issued claim through the public host-relay capture and returns the
// terminal closure, which leaves the lineage where the last lens capture used
// to leave it when deterministic findings were corroborated without a refuter.
func corroborateRefuterClaimsForTest(t *testing.T, repo, lineage string) reviewLastEventClosureResult {
	t.Helper()
	root, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(t.Context())
	if err != nil {
		t.Fatalf("resolve refuter repository root: %v", err)
	}
	store, record, err := discoverCompactFacadeReview(t.Context(), root, lineage, false)
	if err != nil {
		t.Fatalf("discover refuter authority: %v", err)
	}
	if record.State.State != reviewtransaction.StateReviewing && !reviewProviderRefutesDeterministic(record.State.RuntimeAgent) {
		// Without a refuter runtime the last lens capture already closed the
		// review: a deterministic finding blocks directly, so no batch exists.
		return reviewLastEventClosureResult{}
	}
	request, err := reviewProviderNewRefuterRequest(t.Context(), root, store.Dir, record.State, record.State.CapturePhaseRevision)
	if err != nil {
		t.Fatalf("build required refuter request: %v", err)
	}
	results := make([]facadeRefuterOutcome, 0, len(request.Claims))
	for _, claim := range request.Claims {
		results = append(results, facadeRefuterOutcome{
			FindingID: claim.FindingID, Outcome: reviewtransaction.OutcomeCorroborated, ProofRefs: []string{"independent reproduction of " + claim.FindingID},
		})
	}
	raw, err := json.Marshal(facadeRefuterResult{RequestHash: request.RequestHash, Results: results})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReview([]string{
		"capture-refuter", "--cwd", root, "--lineage", lineage, "--target", record.State.InitialSnapshot.Identity,
		"--expected-revision", record.State.CapturePhaseRevision, "--agent", "pi", "--input", writeReviewCLIRawInput(t, raw),
	}, &output); err != nil {
		t.Fatalf("capture corroborating refuter: %v\n%s", err, output.String())
	}
	var closure reviewLastEventClosureResult
	decodeStrictReviewJSON(t, output.Bytes(), &closure)
	return closure
}

func startFacadeReview(t *testing.T, repo string) ReviewFacadeStartResult {
	t.Helper()
	return startFacadeReviewForRuntime(t, repo, "")
}

// startFacadeReviewForRuntime freezes the lineage to runtime exactly as a
// negotiated START declaring --agent does; empty keeps the manual route.
func startFacadeReviewForRuntime(t *testing.T, repo string, runtime model.AgentID) ReviewFacadeStartResult {
	t.Helper()
	ctx := context.Background()
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	root, err := builder.ResolveRepositoryRoot(ctx)
	if err != nil {
		t.Fatalf("resolve facade review repository root: %v", err)
	}
	rootBuilder := reviewtransaction.SnapshotBuilder{Repo: root}
	snapshot, err := rootBuilder.Build(ctx, reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatalf("build facade review target: %v", err)
	}
	assessment, err := rootBuilder.AssessSnapshotRisk(ctx, snapshot)
	if err != nil {
		t.Fatalf("classify facade review target: %v", err)
	}
	lenses, err := facadeSelectedLenses(assessment, "reliability")
	if err != nil {
		t.Fatalf("select facade review lenses: %v", err)
	}
	request, err := prepareReviewFacadeCompactAtomicStart(ctx, root, "", "", reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	}, snapshot, assessment, assessment.ChangedLines, lenses, runtime)
	if err != nil {
		t.Fatalf("prepare facade review compact atomic fixture: %v", err)
	}
	compactStarted, err := runReviewFacadeCompactAtomicStart(ctx, root, request)
	if err != nil {
		t.Fatalf("start facade review compact atomic fixture: %v", err)
	}
	action := "created"
	if compactStarted.Replayed {
		action = "replayed"
	}
	return reviewFacadeStartResultFor(action, len(compactStarted.Record.State.SelectedLenses) > 0, compactStarted.Record.State)
}

func reviewProviderRequestHashForTest(t *testing.T, prompt []byte) string {
	t.Helper()
	input := bytes.SplitN(prompt, []byte("\n\nInput:\n"), 2)
	if len(input) != 2 {
		t.Fatalf("provider prompt does not contain input: %s", prompt)
	}
	payload := bytes.SplitN(input[1], []byte("\n\nOutput schema:\n"), 2)
	if len(payload) != 2 {
		t.Fatalf("provider prompt does not contain output schema: %s", prompt)
	}
	var request reviewProviderRefuterRequest
	if err := json.Unmarshal(payload[0], &request); err != nil {
		t.Fatal(err)
	}
	if request.RequestHash == "" {
		t.Fatal("provider refuter request hash is empty")
	}
	return request.RequestHash
}

type providerTestAdapter struct {
	raw []byte
	err error
}

func (adapter providerTestAdapter) Review(context.Context, reviewerprovider.Invocation) ([]byte, error) {
	return adapter.raw, adapter.err
}

type providerTestAdapterFunc func(context.Context, reviewerprovider.Invocation) ([]byte, error)

func (adapter providerTestAdapterFunc) Review(ctx context.Context, invocation reviewerprovider.Invocation) ([]byte, error) {
	return adapter(ctx, invocation)
}

func providerTargetedValidationPayload(t *testing.T, request reviewtransaction.TargetedValidationRequest) []byte {
	t.Helper()
	payload, err := json.Marshal(facadeValidationResult{
		TargetedValidationRequestHash: request.RequestHash,
		CorrectionTargetIdentity:      request.CorrectionTargetIdentity,
		OriginalCriteria:              facadeValidationCheck{Passed: true, Evidence: []string{"original criteria passed"}},
		CorrectionRegression:          facadeValidationCheck{Passed: true, Evidence: []string{"correction regression passed"}},
		FollowUps:                     []reviewtransaction.FollowUp{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

// prepareReviewFacadeCompactAtomicStart is the tier-default (no agent lens
// selection) form of prepareReviewFacadeCompactAtomicStartFor these tests use.
func prepareReviewFacadeCompactAtomicStart(
	ctx context.Context, root, explicitLineage, policySource string,
	target reviewtransaction.Target, snapshot reviewtransaction.Snapshot,
	assessment reviewtransaction.RiskAssessment, changedLines int, lenses []string, runtimeAgent model.AgentID,
) (reviewtransaction.CompactAtomicStartRequest, error) {
	return prepareReviewFacadeCompactAtomicStartFor(ctx, root, explicitLineage, policySource, target, snapshot, assessment, changedLines, lenses, "", runtimeAgent)
}
