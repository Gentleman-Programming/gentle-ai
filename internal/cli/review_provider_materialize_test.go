package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// piHostRelayCaptureBinding renders the exact binding argument vector the
// negotiated collect transition hands the Pi host: the opaque repository
// context plus the frozen lineage, target, revision, lens, order, and
// provider-owned subject hash.
func piHostRelayCaptureBinding(t *testing.T, repo string, args []string, record reviewtransaction.CompactRecord) []string {
	t.Helper()
	handle := args[slices.Index(args, "--repository-context")+1]
	lens := record.State.SelectedLenses[0]
	subjectHash := admittedReviewerResultForTest(t, repo, record, lens, 0).SubjectHash
	return []string{
		"--repository-context", handle,
		"--lineage", record.State.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--expected-revision", record.Revision, "--lens", lens, "--order", "0",
		"--subject-hash", subjectHash,
	}
}

func TestReviewCaptureResultMaterializePrintsPiProviderTaskWithoutCapturing(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, args, record, _ := newCandidateInspectionReview(t, "candidate\n", true)
	binding := piHostRelayCaptureBinding(t, repo, args, record)
	handle := args[slices.Index(args, "--repository-context")+1]
	lens := record.State.SelectedLenses[0]

	var first bytes.Buffer
	if err := RunReviewCaptureResult(append(slices.Clone(binding), "--agent", string(model.AgentPi), "--materialize=true"), &first); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(first.Bytes(), []byte(record.State.InitialSnapshot.Identity)) {
		t.Fatal("materialized provider task omits the frozen target identity")
	}
	var native bytes.Buffer
	if err := RunReview([]string{"lens-context", "--repository-context", handle, "--lens", lens}, &native); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), native.Bytes()) {
		t.Fatalf("materialized bytes diverged from the Go-materialized lens context\nmaterialize:\n%s\nnative:\n%s", first.Bytes(), native.Bytes())
	}
	var second bytes.Buffer
	if err := RunReviewCaptureResult(append(slices.Clone(binding), "--agent", string(model.AgentPi), "--materialize=true"), &second); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("repeated materialization changed the provider task bytes")
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, record.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	slot, err := reviewtransaction.ReadCompactReviewerResultSlot(store.Dir, 0, lens)
	if err != nil || slot.Occupied {
		t.Fatalf("materialize occupied the reviewer result slot: %#v, %v", slot, err)
	}

	stale := replaceInspectionArg(t, slices.Clone(binding), "--subject-hash", "sha256:"+strings.Repeat("0", 64))
	err = RunReviewCaptureResult(append(stale, "--agent", string(model.AgentPi), "--materialize=true"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "materialize subject hash does not match") {
		t.Fatalf("stale materialize subject hash refusal = %v", err)
	}
}

func TestReviewCaptureResultMaterializedPiTaskSubmitsThroughExistingInputPath(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, args, record, _ := newCandidateInspectionReview(t, "candidate\n", true)
	binding := piHostRelayCaptureBinding(t, repo, args, record)
	lens := record.State.SelectedLenses[0]
	if err := RunReviewCaptureResult(append(slices.Clone(binding), "--agent", string(model.AgentPi), "--materialize=true"), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "pi-result.json")
	if err := os.WriteFile(input, admittedReviewerPayloadForTest(t, repo, record, lens, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReviewCaptureResult(append(slices.Clone(binding), "--input", input), &output); err != nil {
		t.Fatal(err)
	}
	var terminal reviewLastEventClosureResult
	decodeStrictReviewJSON(t, output.Bytes(), &terminal)
	if terminal.Operation != "review/capture-result" || terminal.State != reviewtransaction.StateApproved {
		t.Fatalf("submitted reviewer terminal result = %#v", terminal)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, record.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	assertApprovedCompactAuthorityBurned(t, store, record.State.LineageID)
}

func TestReviewCaptureResultMaterializeRefusals(t *testing.T) {
	fakeBinding := []string{
		"--repository-context", "rctx1_" + strings.Repeat("0", 64),
		"--lineage", "materialize-refusals", "--target", "sha256:" + strings.Repeat("0", 64),
		"--expected-revision", "sha256:" + strings.Repeat("1", 64), "--lens", "review-reliability", "--order", "0",
	}
	tests := []struct {
		name string
		env  string
		argv []string
		want string
	}{
		{
			name: "compiled claude-code runtime", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentClaudeCode), "--materialize=true"),
			want: "materializes internally",
		},
		{
			name: "compiled codex runtime", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentCodex), "--materialize=true"),
			want: "materializes internally",
		},
		{
			// #3441: this refusal used to be byte-identical to the opposite
			// one (--materialize omitted for a non-capture runtime), so the
			// assertion could not tell them apart either. The exact texts of
			// both live in
			// TestCaptureResultHostMediatedRefusalsAreDistinguishable.
			name: "opencode keeps its host-mediated refusal", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentOpenCode), "--materialize=true"),
			want: "--materialize is unavailable for \"opencode\": printing the Go-materialized provider task is the host-relay form",
		},
		{
			name: "combined with input", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentPi), "--materialize=true", "--input", "-"),
			want: "cannot be combined with --input or --preflight",
		},
		{
			name: "combined with preflight", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentPi), "--materialize=true", "--preflight=true"),
			want: "cannot be combined with --input or --preflight",
		},
		{
			name: "without agent", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--materialize=true"),
			want: "requires --agent",
		},
		{
			name: "without relay handshake", env: "",
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentPi), "--materialize=true"),
			want: "not eligible for immutable receipt review",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(reviewPiHostRelayContractEnvironment, test.env)
			err := RunReviewCaptureResult(test.argv, io.Discard)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("materialize refusal = %v, want %q", err, test.want)
			}
		})
	}
}

func TestNegotiatedStatusRendersPiHostRelayMaterializeCaptureInput(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, _, record, _ := newCandidateInspectionReview(t, "candidate\n", true)
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", record.State.LineageID, "--contract", ReviewIntegrationContractV2,
		"--agent", string(model.AgentPi), "--next-transition",
	}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if err := status.Validate(); err != nil {
		t.Fatalf("pi host relay STATUS is invalid: %v", err)
	}
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "reviewer_results_required" || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("pi host relay transition = %#v", status.NextTransition)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.CaptureOperation != "review.capture-result" {
		t.Fatalf("pi host relay capture operation = %q", input.CaptureOperation)
	}
	tokens := map[string]string{}
	for _, argument := range input.Arguments {
		tokens[argument.Name] = argument.Token
	}
	if tokens["agent"] != "--agent="+string(model.AgentPi) || tokens["materialize"] != "--materialize=true" {
		t.Fatalf("pi host relay capture arguments = %#v", input.Arguments)
	}
	wantTokens := make([]string, 0, len(input.Arguments)-1)
	for _, argument := range input.Arguments {
		if argument.Name != "agent" && argument.Name != "materialize" {
			wantTokens = append(wantTokens, argument.Token)
		}
	}
	wantTokens = append(wantTokens, "--input={{value}}")
	if input.Submission == nil || input.Submission.OperationToken != "capture-result" ||
		!slices.Equal(input.Submission.ArgumentTokens, wantTokens) || len(input.Submission.Values) != 0 ||
		input.Submission.Value == nil || input.Submission.Value.Slot != "reviewer_result" ||
		input.Submission.Value.Domain != "artifact_path_or_stdin" || input.Submission.Value.Schema != reviewReviewerSchemaID ||
		input.Submission.Value.SubstitutionLocation != len(wantTokens)-1 {
		t.Fatalf("pi host relay submission = %#v, want tokens %v", input.Submission, wantTokens)
	}

	var compiled bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", record.State.LineageID, "--contract", ReviewIntegrationContractV2,
		"--agent", string(model.AgentClaudeCode), "--next-transition",
	}, &compiled); err != nil {
		t.Fatal(err)
	}
	var compiledStatus ReviewTargetStatusResult
	decodeStrictReviewJSON(t, compiled.Bytes(), &compiledStatus)
	if compiledStatus.NextTransition == nil || compiledStatus.NextTransition.Collect == nil ||
		len(compiledStatus.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("compiled runtime transition = %#v", compiledStatus.NextTransition)
	}
	compiledArguments, err := reviewTransitionArgumentMap(compiledStatus.NextTransition.Collect.Inputs[0].Arguments)
	if err != nil {
		t.Fatal(err)
	}
	if _, leaked := compiledArguments["materialize"]; leaked || compiledArguments["agent"] != string(model.AgentClaudeCode) ||
		compiledStatus.NextTransition.Collect.Inputs[0].Submission != nil {
		t.Fatalf("compiled runtime capture rendering changed: %#v", compiledStatus.NextTransition.Collect.Inputs[0])
	}
}
