package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// piRefuterReview builds a reviewing authority whose one captured lens carries
// a severe inferential finding, so the transaction-wide refuter batch is
// required before terminal closure.
func piRefuterReview(t *testing.T) (string, reviewtransaction.CompactStore, reviewtransaction.CompactRecord, string) {
	t.Helper()
	repo, started, store, record := newArtifactReview(t, false)
	result := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[0], 0)
	result.Findings = []facadeFinding{{
		ID: "R3-001", Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "candidate failure",
		ProofRefs: []string{"tracked.txt:1 candidate-specific proof"}, EvidenceClass: reviewtransaction.EvidenceInferential,
		CausalDisposition: reviewtransaction.CausalBehaviorActivated,
	}}
	input := filepath.Join(t.TempDir(), "result.json")
	writeReviewCLIJSON(t, input, result)
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
	}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	updated, loadErr := store.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	record = updated
	handle, err := reviewtransaction.DeriveReviewRepositoryContextHandle(t.Context(), repo, reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.State.CapturePhaseRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	return repo, store, record, handle
}

func piRefuterBinding(repo string, record reviewtransaction.CompactRecord, handle string) []string {
	return []string{
		"--cwd", repo,
		"--repository-context", handle, "--lineage", record.State.LineageID,
		"--target", record.State.InitialSnapshot.Identity, "--expected-revision", record.State.CapturePhaseRevision,
	}
}

func piRefuterRawResult(t *testing.T, repo string, store reviewtransaction.CompactStore, record reviewtransaction.CompactRecord) []byte {
	t.Helper()
	request, err := reviewProviderNewRefuterRequest(t.Context(), repo, store.Dir, record.State, record.State.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(facadeRefuterResult{RequestHash: request.RequestHash, Results: []facadeRefuterOutcome{{
		FindingID: "R3-001", Outcome: reviewtransaction.OutcomeCorroborated, ProofRefs: []string{"independent reproduction"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestReviewCaptureRefuterMaterializePrintsPiProviderTaskWithoutCapturing(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, store, record, handle := piRefuterReview(t)
	handle = rctx2ReviewRepositoryContextForTest(t, repo, reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.State.CapturePhaseRevision,
	})
	binding := piRefuterBinding(repo, record, handle)

	var first bytes.Buffer
	if err := RunReview(append(append([]string{"capture-refuter"}, binding...), "--agent", string(model.AgentPi), "--materialize=true"), &first); err != nil {
		t.Fatal(err)
	}
	request, err := reviewProviderNewRefuterRequest(t.Context(), repo, store.Dir, record.State, record.State.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), request.Invocation.Prompt()) {
		t.Fatalf("materialized bytes diverged from the Go-materialized refuter request\nmaterialize:\n%s\nnative:\n%s", first.Bytes(), request.Invocation.Prompt())
	}
	var second bytes.Buffer
	if err := RunReview(append(append([]string{"capture-refuter"}, binding...), "--agent", string(model.AgentPi), "--materialize=true"), &second); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("repeated refuter materialization changed the provider task bytes")
	}
	current, err := store.Load()
	if err != nil || recordHasAdmittedRole(current.State, reviewtransaction.CompactRoleRefuter) {
		t.Fatalf("refuter materialize mutated compact authority: %#v, %v", current, err)
	}
}

func TestLastCleanReviewerCaptureNeverStrandsARefuterSlot(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, store, record := newArtifactReview(t, false)
	input := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(input, admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
	}, &output); err != nil {
		t.Fatal(err)
	}
	var terminal reviewLastEventClosureResult
	decodeStrictReviewJSON(t, output.Bytes(), &terminal)
	if terminal.Operation != "review/capture-result" || terminal.State != reviewtransaction.StateApproved {
		t.Fatalf("clean terminal capture = %#v", terminal)
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
}

// overrideProviderRoleHostAdapter substitutes the Go-owned pi spawn seam with
// a fake transport for one test.
func overrideProviderRoleHostAdapter(t *testing.T, adapter reviewerprovider.Adapter) {
	t.Helper()
	previous := reviewProviderRoleHostAdapter
	t.Cleanup(func() { reviewProviderRoleHostAdapter = previous })
	reviewProviderRoleHostAdapter = func() reviewerprovider.Adapter { return adapter }
}

func TestReviewCaptureRefuterExecutesGoOwnedPiAndClosesOnTheRefuterEvent(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, store, record, handle := piRefuterReview(t)
	binding := piRefuterBinding(repo, record, handle)
	// An execution without the identified host-relay runtime is refused; the
	// gate is symmetric with the materialize form.
	if err := RunReview(append(append([]string{"capture-refuter"}, binding...), "--execute=true"), io.Discard); err == nil || !strings.Contains(err.Error(), "requires --agent") {
		t.Fatalf("agent-free refuter execution refusal = %v", err)
	}
	overrideProviderRoleHostAdapter(t, providerTestAdapter{raw: piRefuterRawResult(t, repo, store, record)})
	var output bytes.Buffer
	if err := RunReview(append(append([]string{"capture-refuter"}, binding...), "--agent", string(model.AgentPi), "--execute=true"), &output); err != nil {
		t.Fatal(err)
	}
	var terminal reviewLastEventClosureResult
	decodeStrictReviewJSON(t, output.Bytes(), &terminal)
	if terminal.Schema != reviewLastEventClosureSchema || terminal.Operation != reviewCaptureRefuterCaptureOperation ||
		terminal.LineageID != record.State.LineageID || terminal.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("refuter capture closure = %#v", terminal)
	}
	final, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	request, err := reviewProviderNewRefuterRequest(t.Context(), repo, store.Dir, record.State, record.State.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}
	if _, captured := final.State.AdmittedRoleResult(reviewtransaction.CompactRoleRefuter, record.State.CapturePhaseRevision, record.State.InitialSnapshot.Identity, request.RequestHash); !captured {
		t.Fatal("refuter submission did not merge an in-record role value")
	}
	if final.State.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("terminal state = %q, want corroborated blocking finding to require correction", final.State.State)
	}
}

// TestReviewCaptureRefuterExecuteDeadlineFailsClosedWithoutCapture stalls a
// real spawned fake pi (its grandchild holds the inherited stdout pipe) past
// the shrunken deadline: typed refusal, untouched slot, no hang.
func TestReviewCaptureRefuterExecuteDeadlineFailsClosedWithoutCapture(t *testing.T) {
	reviewEnabledHome(t)
	if goruntime.GOOS == "windows" {
		t.Skip("the stalled fake pi is a POSIX shell script")
	}
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, store, record, handle := piRefuterReview(t)
	previous := reviewProviderRoleCaptureTimeout
	t.Cleanup(func() { reviewProviderRoleCaptureTimeout = previous })
	reviewProviderRoleCaptureTimeout = 100 * time.Millisecond
	stalled := filepath.Join(t.TempDir(), "stalled-pi")
	if err := os.WriteFile(stalled, []byte("#!/bin/sh\nsleep 10\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	overrideProviderRoleHostAdapter(t, &reviewerprovider.PiAdapter{LookPath: func(string) (string, error) { return stalled, nil }})
	err := RunReview(append(append([]string{"capture-refuter"}, piRefuterBinding(repo, record, handle)...), "--agent", string(model.AgentPi), "--execute=true"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "pi reviewer transport failed") || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("stalled pi deadline refusal = %v", err)
	}
	current, loadErr := store.Load()
	if loadErr != nil || recordHasAdmittedRole(current.State, reviewtransaction.CompactRoleRefuter) {
		t.Fatalf("stalled pi execution mutated compact authority: %#v, %v", current, loadErr)
	}
}

func TestReviewCaptureRefuterRefusals(t *testing.T) {
	fakeBinding := []string{
		"--repository-context", "rctx1_" + strings.Repeat("0", 64),
		"--lineage", "role-materialize-refusals", "--target", "sha256:" + strings.Repeat("0", 64),
		"--expected-revision", "sha256:" + strings.Repeat("1", 64),
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
			want: "invalid_request",
		},
		{
			name: "compiled codex runtime", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentCodex), "--materialize=true"),
			want: "invalid_request",
		},
		{
			name: "opencode keeps its host-mediated refusal", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentOpenCode), "--materialize=true"),
			want: "invalid_request",
		},
		{
			name: "materialize combined with execute", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentPi), "--materialize=true", "--execute=true"),
			want: "cannot be combined with --execute",
		},
		{
			name: "retired caller-authored input flag", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentPi), "--input", "-"),
			want: "not defined: -input",
		},
		{
			name: "without agent", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--materialize=true"),
			want: "requires --agent",
		},
		{
			name: "agent without materialize or input", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentPi)),
			want: "either --materialize",
		},
		{
			name: "without relay handshake", env: "",
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentPi), "--materialize=true"),
			want: "not eligible for immutable receipt review",
		},
		{
			name: "without materialize or input", env: reviewPiHostRelayContract,
			argv: slices.Clone(fakeBinding),
			want: "either --materialize",
		},
		// The execute form is gated exactly like the materialize form: the
		// Go-owned adversarial spawn runs only for the identified host-relay
		// runtime, never for an unidentified or compiled caller.
		{
			name: "execution without agent", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--execute=true"),
			want: "requires --agent",
		},
		{
			name: "execution from compiled claude-code runtime", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentClaudeCode), "--execute=true"),
			want: "invalid_request",
		},
		{
			name: "execution from compiled codex runtime", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentCodex), "--execute=true"),
			want: "invalid_request",
		},
		{
			name: "execution from opencode", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentOpenCode), "--execute=true"),
			want: "invalid_request",
		},
		{
			name: "execution without relay handshake", env: "",
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentPi), "--execute=true"),
			want: "not eligible for immutable receipt review",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(reviewPiHostRelayContractEnvironment, test.env)
			err := RunReview(append([]string{"capture-refuter"}, test.argv...), io.Discard)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("capture-refuter refusal = %v, want %q", err, test.want)
			}
			validationErr := RunReview(append([]string{"capture-validation"}, append(slices.Clone(test.argv), "--request-hash", "sha256:"+strings.Repeat("2", 64))...), io.Discard)
			if validationErr == nil || !strings.Contains(validationErr.Error(), test.want) {
				t.Fatalf("capture-validation refusal = %v, want %q", validationErr, test.want)
			}
		})
	}
}

// TestHostileProviderRoleCaptureTransitionsFailClosedWithoutPanic decodes
// hostile role-capture transitions -- zero arguments, and a truncated
// argument vector -- and requires a typed refusal from Validate(), never a
// panic from unguarded slice arithmetic over the argument count.
func TestHostileProviderRoleCaptureTransitionsFailClosedWithoutPanic(t *testing.T) {
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	for _, test := range []struct {
		name      string
		operation string
		arguments string
	}{
		{name: "refuter with zero arguments", operation: "review.capture-refuter", arguments: `[]`},
		{name: "validation with zero arguments", operation: "review.capture-validation", arguments: `[]`},
		{
			name: "refuter with a truncated argument vector", operation: "review.capture-refuter",
			arguments: `[{"name":"lineage","value":"hostile"}]`,
		},
		{
			name: "validation with a truncated argument vector", operation: "review.capture-validation",
			arguments: `[{"name":"lineage","value":"hostile"}]`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := `{"kind":"collect","reason_code":"provider_refuter_required","collect":{"inputs":[{` +
				`"name":"provider_refuter","schema":"` + reviewRefuterSchemaID + `",` +
				`"capture_operation":"` + test.operation + `","arguments":` + test.arguments + `}]}}`
			var transition ReviewNextTransition
			if err := json.Unmarshal([]byte(payload), &transition); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("hostile role capture transition panicked: %v", recovered)
				}
			}()
			if err := transition.Validate(); err == nil {
				t.Fatal("hostile role capture transition validated")
			}
		})
	}
}

func TestNegotiatedStatusRendersPiHostRelayRefuterCollectInput(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, store, record, _ := piRefuterReview(t)
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
		t.Fatalf("pi host relay refuter STATUS is invalid: %v", err)
	}
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "provider_refuter_required" || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("pi host relay refuter transition = %#v", status.NextTransition)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.CaptureOperation != "review.capture-refuter" || input.Name != "provider_refuter" || input.Schema != reviewRefuterSchemaID || input.ProviderTask != nil {
		t.Fatalf("pi host relay refuter input = %#v", input)
	}
	// The rendered vector is self-contained and name-based: every token is
	// re-derived from the input's own arguments, and no submission descriptor
	// exists for a caller to author a verdict through.
	tokens := map[string]string{}
	for _, argument := range input.Arguments {
		if argument.Token != reviewTransitionArgumentToken(argument) {
			t.Fatalf("pi host relay refuter token %q diverged from its own argument", argument.Token)
		}
		tokens[argument.Name] = argument.Token
	}
	if tokens["agent"] != "--agent="+string(model.AgentPi) || tokens["execute"] != "--execute=true" || input.Submission != nil {
		t.Fatalf("pi host relay refuter arguments = %#v, submission = %#v", input.Arguments, input.Submission)
	}

	// The OpenCode rendering stays byte-identical: a Go-issued provider task,
	// no submission descriptor. Checked before the submission below occupies
	// the refuter slot and retires this collection state.
	var opencode bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", record.State.LineageID, "--contract", ReviewIntegrationContractV2,
		"--agent", string(model.AgentOpenCode), "--next-transition",
	}, &opencode); err != nil {
		t.Fatal(err)
	}
	var opencodeStatus ReviewTargetStatusResult
	decodeStrictReviewJSON(t, opencode.Bytes(), &opencodeStatus)
	if opencodeStatus.NextTransition == nil || opencodeStatus.NextTransition.ReasonCode != "provider_refuter_required" ||
		opencodeStatus.NextTransition.Collect == nil || len(opencodeStatus.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("OpenCode refuter transition changed: %#v", opencodeStatus.NextTransition)
	}
	opencodeInput := opencodeStatus.NextTransition.Collect.Inputs[0]
	task := opencodeInput.ProviderTask
	if opencodeInput.CaptureOperation != "external.run_provider_role" || opencodeInput.Submission != nil ||
		task == nil || task.Agent != "review-refuter" || task.Role != string(reviewerprovider.RoleRefuter) ||
		!strings.HasPrefix(task.Prompt, reviewProviderTaskBindingHeader+" ") {
		t.Fatalf("OpenCode refuter rendering changed: %#v", opencodeInput)
	}

	// The rendered transition is executable exactly as issued: the vector
	// itself materializes, spawns the Go-owned pi process, and admits.
	overrideProviderRoleHostAdapter(t, providerTestAdapter{raw: piRefuterRawResult(t, repo, store, record)})
	execute := []string{"capture-refuter", "--cwd=" + repo}
	for _, argument := range input.Arguments {
		execute = append(execute, argument.Token)
	}
	if err := RunReview(execute, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	final, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range final.State.AdmittedRoleResults {
		found = found || entry.Role == reviewtransaction.CompactRoleRefuter
	}
	if !found {
		t.Fatal("rendered execution did not merge an in-record refuter value")
	}
}

func TestReviewCaptureValidationMaterializesExecutesAndCloses(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, lineage, request := providerCorrectionReadyWithoutVerificationEvidence(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2,
		"--agent", string(model.AgentPi), "--next-transition",
	}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if err := status.Validate(); err != nil {
		t.Fatalf("pi host relay validation STATUS is invalid: %v", err)
	}
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "targeted_validation_required" || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("pi host relay validation transition = %#v", status.NextTransition)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.CaptureOperation != "review.capture-validation" || input.Schema != reviewValidatorSchemaID ||
		input.ProviderTask != nil || input.ValidationRequest == nil || input.ValidationRequest.RequestHash != request.RequestHash {
		t.Fatalf("pi host relay validation input = %#v", input)
	}
	arguments := map[string]string{}
	tokens := map[string]string{}
	for _, argument := range input.Arguments {
		arguments[argument.Name] = argument.Value
		tokens[argument.Name] = argument.Token
	}
	if arguments["request-hash"] != request.RequestHash || arguments["target"] != request.CorrectionTargetIdentity ||
		tokens["agent"] != "--agent="+string(model.AgentPi) || tokens["execute"] != "--execute=true" || input.Submission != nil {
		t.Fatalf("pi host relay validation arguments = %#v, submission = %#v", input.Arguments, input.Submission)
	}

	// Materialize form of the same rendered binding: idempotent,
	// byte-identical to the Go-materialized validator request, slot-free.
	prelude := []string{"capture-validation", "--cwd=" + repo}
	for _, argument := range input.Arguments {
		if argument.Name == "execute" {
			continue
		}
		prelude = append(prelude, argument.Token)
	}
	prelude = append(prelude, "--materialize=true")
	var first bytes.Buffer
	if err := RunReview(slices.Clone(prelude), &first); err != nil {
		t.Fatal(err)
	}
	correction, err := reviewProviderTargetedValidatorCorrection(t.Context(), repo, record.State)
	if err != nil {
		t.Fatal(err)
	}
	native, err := reviewProviderNewTargetedValidatorRequest(t.Context(), repo, record.State, record.State.CapturePhaseRevision, correction)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), native.Invocation.Prompt()) {
		t.Fatal("materialized validator bytes diverged from the Go-materialized request")
	}
	var second bytes.Buffer
	if err := RunReview(slices.Clone(prelude), &second); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("repeated validator materialization changed the provider task bytes")
	}
	current, err := store.Load()
	if err != nil || recordHasAdmittedRole(current.State, reviewtransaction.CompactRoleTargetedValidator) {
		t.Fatalf("validator materialize mutated compact authority: %#v, %v", current, err)
	}

	// Execution: the rendered vector spawns the Go-owned pi transport and the
	// raw bytes close the bounded correction on their terminal capture.
	overrideProviderRoleHostAdapter(t, providerTestAdapter{raw: providerTargetedValidationPayload(t, request)})
	execute := []string{"capture-validation", "--cwd=" + repo}
	for _, argument := range input.Arguments {
		execute = append(execute, argument.Token)
	}
	var captured bytes.Buffer
	if err := RunReview(execute, &captured); err != nil {
		t.Fatal(err)
	}
	var closure reviewLastEventClosureResult
	decodeStrictReviewJSON(t, captured.Bytes(), &closure)
	if closure.Schema != reviewLastEventClosureSchema || closure.Operation != "review/capture-validation" ||
		closure.LineageID != lineage || closure.State != reviewtransaction.StateApproved {
		t.Fatalf("validator terminal capture = %#v", closure)
	}
	assertApprovedCompactAuthorityBurned(t, store, lineage)
}

func TestReviewCaptureValidationBindsFrozenRequestHash(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, lineage, request := providerCorrectionReadyWithoutVerificationEvidence(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	handle, err := reviewtransaction.DeriveReviewRepositoryContextHandle(t.Context(), repo, reviewtransaction.ReviewRepositoryContextBinding{LineageID: request.LineageID, TargetIdentity: request.CorrectionTargetIdentity, Revision: request.ExpectedRevision})
	if err != nil {
		t.Fatal(err)
	}
	binding := []string{
		"--cwd", repo,
		"--repository-context", handle, "--lineage", lineage,
		"--target", request.CorrectionTargetIdentity, "--expected-revision", record.State.CapturePhaseRevision,
	}
	stale := append(slices.Clone(binding), "--request-hash", "sha256:"+strings.Repeat("0", 64),
		"--agent", string(model.AgentPi), "--materialize=true")
	err = RunReview(append([]string{"capture-validation"}, stale...), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "request hash does not match") {
		t.Fatalf("stale validation request hash refusal = %v", err)
	}
	missing := append(slices.Clone(binding), "--agent", string(model.AgentPi), "--materialize=true")
	err = RunReview(append([]string{"capture-validation"}, missing...), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "--request-hash") {
		t.Fatalf("missing validation request hash refusal = %v", err)
	}
}

func TestNegotiatedStatusOffersCurrentValidationCaptureForOtherRuntimes(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, lineage, _ := providerCorrectionReadyWithoutVerificationEvidence(t)

	// claude-code keeps the current targeted-validation collection.
	var compiled bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2,
		"--agent", string(model.AgentClaudeCode), "--next-transition",
	}, &compiled); err != nil {
		t.Fatal(err)
	}
	var compiledStatus ReviewTargetStatusResult
	decodeStrictReviewJSON(t, compiled.Bytes(), &compiledStatus)
	if compiledStatus.NextTransition == nil || compiledStatus.NextTransition.ReasonCode != "targeted_validation_required" ||
		compiledStatus.NextTransition.Collect == nil || len(compiledStatus.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("compiled validation transition changed: %#v", compiledStatus.NextTransition)
	}
	compiledInput := compiledStatus.NextTransition.Collect.Inputs[0]
	if compiledInput.CaptureOperation != reviewCaptureValidationCaptureOperation || compiledInput.ValidationRequest == nil {
		t.Fatalf("compiled validation rendering changed: %#v", compiledInput)
	}

	// OpenCode keeps its Go-issued provider validator task.
	var opencode bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2,
		"--agent", string(model.AgentOpenCode), "--next-transition",
	}, &opencode); err != nil {
		t.Fatal(err)
	}
	var opencodeStatus ReviewTargetStatusResult
	decodeStrictReviewJSON(t, opencode.Bytes(), &opencodeStatus)
	if opencodeStatus.NextTransition == nil || opencodeStatus.NextTransition.ReasonCode != "targeted_validation_required" ||
		opencodeStatus.NextTransition.Collect == nil || len(opencodeStatus.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("OpenCode validation transition changed: %#v", opencodeStatus.NextTransition)
	}
	opencodeInput := opencodeStatus.NextTransition.Collect.Inputs[0]
	task := opencodeInput.ProviderTask
	if opencodeInput.CaptureOperation != "external.run_provider_role" || opencodeInput.Submission != nil ||
		task == nil || task.Agent != "review-validator" || task.Role != string(reviewerprovider.RoleTargetedValidator) ||
		!strings.HasPrefix(task.Prompt, reviewProviderTaskBindingHeader+" ") {
		t.Fatalf("OpenCode validation rendering changed: %#v", opencodeInput)
	}
}

func recordHasAdmittedRole(state reviewtransaction.CompactState, role reviewtransaction.CompactRole) bool {
	for _, entry := range state.AdmittedRoleResults {
		if entry.Role == role {
			return true
		}
	}
	return false
}

// TestRefuterRequestCarriesTheFindingClaimText is #3482: the compiled refuter
// batch listed each inferential finding as finding_id plus proof locations
// only, so the refuter had no proposition to corroborate or refute and could
// honestly return nothing but inconclusive, which escalates every lineage.
func TestRefuterRequestCarriesTheFindingClaimText(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, _, record, handle := piRefuterReview(t)
	handle = rctx2ReviewRepositoryContextForTest(t, repo, reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.State.CapturePhaseRevision,
	})
	var prompt bytes.Buffer
	if err := RunReview(append(append([]string{"capture-refuter"}, piRefuterBinding(repo, record, handle)...), "--agent", string(model.AgentPi), "--materialize=true"), &prompt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt.String(), `"finding_id":"R3-001"`) || !strings.Contains(prompt.String(), `"claim":"candidate failure"`) {
		t.Fatalf("refuter request omits the finding's claim text; the refuter can only return inconclusive:\n%s", prompt.String())
	}
}
