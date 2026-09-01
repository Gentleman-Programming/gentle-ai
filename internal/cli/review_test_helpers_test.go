package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func startFacadeReview(t *testing.T, repo string) ReviewFacadeStartResult {
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
	}, snapshot, assessment, assessment.ChangedLines, lenses, "")
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
