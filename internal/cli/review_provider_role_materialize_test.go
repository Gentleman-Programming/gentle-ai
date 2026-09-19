package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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

// TestReviewCaptureRefuterExecuteIsHostMediatedForPi is #4611: Go no longer
// spawns a pi process for the refuter role. --execute is refused for the pi
// runtime exactly as --materialize is refused for a compiled runtime, naming
// the STATUS-issued --materialize operation and the --input submission it
// feeds.
func TestReviewCaptureRefuterExecuteIsHostMediatedForPi(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, _, record, handle := piRefuterReview(t)
	binding := piRefuterBinding(repo, record, handle)
	// An execution without the identified host-relay runtime is refused; the
	// gate is symmetric with the materialize form.
	if err := RunReview(append(append([]string{"capture-refuter"}, binding...), "--execute=true"), io.Discard); err == nil || !strings.Contains(err.Error(), "requires --agent") {
		t.Fatalf("agent-free refuter execution refusal = %v", err)
	}
	execute := append(append([]string{"capture-refuter"}, binding...), "--agent", string(model.AgentPi), "--execute=true")
	err := RunReview(execute, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "host-mediated") || !strings.Contains(err.Error(), "--materialize") || !strings.Contains(err.Error(), "--input") {
		t.Fatalf("pi refuter --execute refusal = %v", err)
	}
}

// TestReviewCaptureRefuterInputSubmitsHostRunResultAndClosesTheSlot is T1
// (issue #4611): the host materializes the refuter prompt itself, runs its own
// reviewer on it out of process, and submits only the raw bytes through
// --input. Go never spawns anything for this submission -- the Go-owned pi
// spawn seam no longer exists in this binary -- and admission and closure
// land exactly where --execute used to land them for a compiled runtime.
func TestReviewCaptureRefuterInputSubmitsHostRunResultAndClosesTheSlot(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, store, record, handle := piRefuterReview(t)
	binding := piRefuterBinding(repo, record, handle)
	input := filepath.Join(t.TempDir(), "refuter-result.json")
	if err := os.WriteFile(input, piRefuterRawResult(t, repo, store, record), 0o600); err != nil {
		t.Fatal(err)
	}
	submit := append(append([]string{"capture-refuter"}, binding...), "--agent", string(model.AgentPi), "--input", input)
	var output bytes.Buffer
	if err := RunReview(submit, &output); err != nil {
		t.Fatal(err)
	}
	var terminal reviewLastEventClosureResult
	decodeStrictReviewJSON(t, output.Bytes(), &terminal)
	if terminal.Schema != reviewLastEventClosureSchema || terminal.Operation != reviewCaptureRefuterCaptureOperation ||
		terminal.LineageID != record.State.LineageID || terminal.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("refuter --input closure = %#v", terminal)
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
		t.Fatal("refuter --input submission did not merge an in-record role value")
	}
}

// TestReviewCaptureRefuterInputDashReadsStdin is the stdin form of the same
// submission (#4611): a host that piped its reviewer's stdout straight through
// submits with `--input -` instead of a file path.
func TestReviewCaptureRefuterInputDashReadsStdin(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, store, record, handle := piRefuterReview(t)
	binding := piRefuterBinding(repo, record, handle)
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatal(pipeErr)
	}
	originalStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = originalStdin })
	raw := piRefuterRawResult(t, repo, store, record)
	go func() {
		_, _ = w.Write(raw)
		_ = w.Close()
	}()
	submit := append(append([]string{"capture-refuter"}, binding...), "--agent", string(model.AgentPi), "--input", "-")
	if err := RunReview(submit, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	final, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !recordHasAdmittedRole(final.State, reviewtransaction.CompactRoleRefuter) {
		t.Fatal("refuter --input - submission did not merge an in-record role value")
	}
}

// TestReviewCaptureRefuterInputRefusesMissingEmptyOrUnadmittableBytesWithoutStrandingTheSlot
// covers three distinct submission failures (#4611): a missing file, an empty
// file, and syntactically invalid bytes. Every one leaves the compact
// authority untouched, so a following STATUS reoffers the exact same refuter
// collection input -- no verdict is ever synthesized from a failed submission.
func TestReviewCaptureRefuterInputRefusesMissingEmptyOrUnadmittableBytesWithoutStrandingTheSlot(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, store, record, handle := piRefuterReview(t)
	binding := piRefuterBinding(repo, record, handle)

	missing := filepath.Join(t.TempDir(), "missing.json")
	if err := RunReview(append(append([]string{"capture-refuter"}, binding...), "--agent", string(model.AgentPi), "--input", missing), io.Discard); err == nil || !strings.Contains(err.Error(), "read provider refuter result") {
		t.Fatalf("missing --input file refusal = %v", err)
	}

	empty := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReview(append(append([]string{"capture-refuter"}, binding...), "--agent", string(model.AgentPi), "--input", empty), io.Discard); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty --input file refusal = %v", err)
	}

	garbage := filepath.Join(t.TempDir(), "garbage.json")
	if err := os.WriteFile(garbage, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReview(append(append([]string{"capture-refuter"}, binding...), "--agent", string(model.AgentPi), "--input", garbage), io.Discard); err == nil {
		t.Fatal("unadmittable --input bytes were accepted")
	}

	current, err := store.Load()
	if err != nil || recordHasAdmittedRole(current.State, reviewtransaction.CompactRoleRefuter) {
		t.Fatalf("rejected --input bytes mutated compact authority: %#v, %v", current, err)
	}
	var status bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", record.State.LineageID, "--contract", ReviewIntegrationContractV2,
		"--agent", string(model.AgentPi), "--next-transition",
	}, &status); err != nil {
		t.Fatal(err)
	}
	var reoffered ReviewTargetStatusResult
	decodeStrictReviewJSON(t, status.Bytes(), &reoffered)
	if reoffered.NextTransition == nil || reoffered.NextTransition.ReasonCode != "provider_refuter_required" {
		t.Fatalf("STATUS did not reoffer the refuter slot after a rejected --input submission: %#v", reoffered.NextTransition)
	}
}

// TestReviewCaptureValidationInputSubmitsHostRunResultAndCloses is
// capture-validation's counterpart to
// TestReviewCaptureRefuterInputSubmitsHostRunResultAndClosesTheSlot (#4611):
// --input carries the frozen --request-hash binding exactly like --execute
// does, submits without invoking any adapter, and closes the bounded
// correction on its terminal capture.
func TestReviewCaptureValidationInputSubmitsHostRunResultAndCloses(t *testing.T) {
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
	binding := []string{
		"--cwd", repo, "--lineage", lineage,
		"--target", request.CorrectionTargetIdentity, "--expected-revision", record.State.CapturePhaseRevision,
		"--request-hash", request.RequestHash,
	}
	input := filepath.Join(t.TempDir(), "validation-result.json")
	if err := os.WriteFile(input, providerTargetedValidationPayload(t, request), 0o600); err != nil {
		t.Fatal(err)
	}
	submit := append(append([]string{"capture-validation"}, binding...), "--agent", string(model.AgentPi), "--input", input)
	var output bytes.Buffer
	if err := RunReview(submit, &output); err != nil {
		t.Fatal(err)
	}
	var closure reviewLastEventClosureResult
	decodeStrictReviewJSON(t, output.Bytes(), &closure)
	if closure.Schema != reviewLastEventClosureSchema || closure.Operation != "review/capture-validation" ||
		closure.LineageID != lineage || closure.State != reviewtransaction.StateApproved {
		t.Fatalf("validation --input closure = %#v", closure)
	}
	assertApprovedCompactAuthorityBurned(t, store, lineage)
}

// TestReviewCaptureRefuterMaterializeRefusesCompletePromptOverRuntimeBudget
// pins the complete-prompt half of the approved runtime input policy: the
// evidence loop bounds raw patch bytes, but serializing the request into the
// role prompt JSON-escapes every content byte (one raw quote character
// doubles) and adds the instruction and result schema wrappers, so a request
// whose raw evidence fits the budget can still produce a complete prompt over
// the same approved cap. Materialization must refuse with the typed budget
// refusal before any adapter is invoked, and the refusal must name the
// split-candidate resolution.
func TestReviewCaptureRefuterMaterializeRefusesCompletePromptOverRuntimeBudget(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	// Raw patch bytes stay under the evidence budget; JSON escaping doubles
	// every quote character, so the serialized prompt cannot fit.
	writeReviewStartCandidate(t, repo, "runtime-budget-quotes.txt", strings.Repeat(`"`, 180_000)+"\n", 0o644)
	started := startFacadeReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	lens := record.State.SelectedLenses[0]
	result := admittedReviewerResultForTest(t, repo, record, lens, 0)
	result.Findings = []facadeFinding{{
		ID: "R3-001", Location: "runtime-budget-quotes.txt:1", Severity: "CRITICAL", Claim: "candidate failure",
		ProofRefs: []string{"runtime-budget-quotes.txt:1 candidate-specific proof"}, EvidenceClass: reviewtransaction.EvidenceInferential,
		CausalDisposition: reviewtransaction.CausalBehaviorActivated,
	}}
	input := filepath.Join(t.TempDir(), "result.json")
	writeReviewCLIJSON(t, input, result)
	captureErr := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", lens, "--order", "0", "--input", input,
	}, &bytes.Buffer{})
	var captureRefusal *reviewLensContextError
	if captureErr != nil && (!errors.As(captureErr, &captureRefusal) || captureRefusal.Code != "lens_context_budget_exceeded") {
		t.Fatal(captureErr)
	}
	updated, loadErr := store.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	record = updated
	// The closure that renders the refuter collect input materializes the
	// refuter request too, so the capture step may already carry the refusal;
	// the explicit materialization call must carry it regardless.
	_, err = reviewProviderNewRefuterRequest(t.Context(), repo, store.Dir, record.State, record.State.CapturePhaseRevision)
	var refusal *reviewLensContextError
	if !errors.As(err, &refusal) || refusal.Code != "lens_context_budget_exceeded" {
		t.Fatalf("complete refuter prompt over the approved runtime cap materialized without the typed budget refusal: %v", err)
	}
	if !strings.Contains(refusal.Error(), "split the candidate") {
		t.Fatalf("budget refusal does not name the split-candidate resolution: %v", refusal)
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
			name: "materialize combined with input", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentPi), "--materialize=true", "--input", "-"),
			want: "cannot be combined with --input",
		},
		{
			name: "execute combined with input", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentPi), "--execute=true", "--input", "-"),
			want: "cannot be combined with --input",
		},
		{
			name: "input from compiled claude-code runtime", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentClaudeCode), "--input", "-"),
			want: "invalid_request",
		},
		{
			name: "input from compiled codex runtime", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentCodex), "--input", "-"),
			want: "invalid_request",
		},
		{
			name: "input from opencode keeps its host-mediated refusal", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentOpenCode), "--input", "-"),
			want: "invalid_request",
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
			// Issue #4256: without a real bound transaction, an unrelayed Pi
			// materialize call is refused by the same repository-context
			// binding check every other runtime hits -- never by an
			// eligibility gate keyed on the relay handshake env var.
			name: "without relay handshake and without a bound transaction", env: "",
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentPi), "--materialize=true"),
			want: "repository_context_unavailable",
		},
		{
			name: "without materialize or input", env: reviewPiHostRelayContract,
			argv: slices.Clone(fakeBinding),
			want: "either --materialize",
		},
		// The execute form now runs only the compiled Go-owned adapter: it
		// requires an identified runtime, and it is refused for every runtime
		// that is not compiled -- including the pi host relay, which is
		// host-mediated instead (#4611).
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
			// Go never spawns a process for the pi host relay: --execute is
			// refused exactly as --materialize is refused for a compiled
			// runtime, naming the STATUS-issued --materialize operation and
			// the --input submission it feeds.
			name: "execution from pi runtime is host-mediated", env: reviewPiHostRelayContract,
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentPi), "--execute=true"),
			want: "host-mediated",
		},
		{
			// Same as the materialize case above: the fake binding is
			// refused on its own merits, never by the relay handshake env
			// var (issue #4256).
			name: "execution without relay handshake and without a bound transaction", env: "",
			argv: append(slices.Clone(fakeBinding), "--agent", string(model.AgentPi), "--execute=true"),
			want: "repository_context_unavailable",
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
	// The rendered vector is only the read-only materialize prelude: every
	// prelude token is re-derived from the input's own arguments, and the
	// submission descriptor -- the same binding tokens with the raw provider
	// result substituted into --input -- is what actually advances authority.
	// Go never spawns a process for this role (#4611).
	tokens := map[string]string{}
	for _, argument := range input.Arguments {
		if argument.Token != reviewTransitionArgumentToken(argument) {
			t.Fatalf("pi host relay refuter token %q diverged from its own argument", argument.Token)
		}
		tokens[argument.Name] = argument.Token
	}
	if tokens["agent"] != "--agent="+string(model.AgentPi) || tokens["materialize"] != "--materialize=true" || tokens["execute"] != "" {
		t.Fatalf("pi host relay refuter arguments = %#v", input.Arguments)
	}
	wantSubmissionTokens := make([]string, 0, len(input.Arguments))
	for _, argument := range input.Arguments {
		if argument.Name == "materialize" {
			continue
		}
		wantSubmissionTokens = append(wantSubmissionTokens, argument.Token)
	}
	wantSubmissionTokens = append(wantSubmissionTokens, "--input="+reviewSubmissionValuePlaceholder)
	if input.Submission == nil || input.Submission.OperationToken != "capture-refuter" ||
		!slices.Equal(input.Submission.ArgumentTokens, wantSubmissionTokens) || len(input.Submission.Values) != 0 ||
		input.Submission.Value == nil || input.Submission.Value.Slot != "provider_refuter" ||
		input.Submission.Value.Domain != "artifact_path_or_stdin" || input.Submission.Value.Schema != reviewRefuterSchemaID ||
		input.Submission.Value.SubstitutionLocation != len(wantSubmissionTokens)-1 {
		t.Fatalf("pi host relay refuter submission = %#v, want tokens %v", input.Submission, wantSubmissionTokens)
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

	// The rendered transition is executable exactly as issued: the
	// materialize prelude prints the exact Go-materialized prompt, and
	// substituting the submission descriptor's {{value}} slot with a raw
	// host-run result submits it through --input. Go never spawns anything.
	materialize := []string{"capture-refuter", "--cwd=" + repo}
	for _, argument := range input.Arguments {
		materialize = append(materialize, argument.Token)
	}
	var materialized bytes.Buffer
	if err := RunReview(materialize, &materialized); err != nil {
		t.Fatal(err)
	}
	request, err := reviewProviderNewRefuterRequest(t.Context(), repo, store.Dir, record.State, record.State.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(materialized.Bytes(), request.Invocation.Prompt()) {
		t.Fatal("STATUS-rendered materialize prelude diverged from the Go-materialized refuter request")
	}
	resultFile := writeReviewCLIRawInput(t, piRefuterRawResult(t, repo, store, record))
	submit := append([]string{input.Submission.OperationToken, "--cwd", repo}, input.Submission.ArgumentTokens...)
	for index := range submit {
		submit[index] = strings.ReplaceAll(submit[index], reviewSubmissionValuePlaceholder, resultFile)
	}
	if err := RunReview(submit, &bytes.Buffer{}); err != nil {
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

func TestReviewCaptureValidationMaterializesSubmitsAndCloses(t *testing.T) {
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
		tokens["agent"] != "--agent="+string(model.AgentPi) || tokens["materialize"] != "--materialize=true" || tokens["execute"] != "" {
		t.Fatalf("pi host relay validation arguments = %#v", input.Arguments)
	}
	wantSubmissionTokens := make([]string, 0, len(input.Arguments))
	for _, argument := range input.Arguments {
		if argument.Name == "materialize" {
			continue
		}
		wantSubmissionTokens = append(wantSubmissionTokens, argument.Token)
	}
	wantSubmissionTokens = append(wantSubmissionTokens, "--input="+reviewSubmissionValuePlaceholder)
	if input.Submission == nil || input.Submission.OperationToken != "capture-validation" ||
		!slices.Equal(input.Submission.ArgumentTokens, wantSubmissionTokens) || len(input.Submission.Values) != 0 ||
		input.Submission.Value == nil || input.Submission.Value.Slot != "provider_targeted_validator" ||
		input.Submission.Value.Domain != "artifact_path_or_stdin" || input.Submission.Value.Schema != reviewValidatorSchemaID ||
		input.Submission.Value.SubstitutionLocation != len(wantSubmissionTokens)-1 {
		t.Fatalf("pi host relay validation submission = %#v, want tokens %v", input.Submission, wantSubmissionTokens)
	}

	// Materialize form of the same rendered binding: idempotent,
	// byte-identical to the Go-materialized validator request, slot-free.
	prelude := []string{"capture-validation", "--cwd=" + repo}
	for _, argument := range input.Arguments {
		prelude = append(prelude, argument.Token)
	}
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

	// Submission: substituting the descriptor's {{value}} slot with a raw
	// host-run result submits it through --input and closes the bounded
	// correction on its terminal capture. Go never spawns anything.
	resultFile := writeReviewCLIRawInput(t, providerTargetedValidationPayload(t, request))
	submit := append([]string{input.Submission.OperationToken, "--cwd", repo}, input.Submission.ArgumentTokens...)
	for index := range submit {
		submit[index] = strings.ReplaceAll(submit[index], reviewSubmissionValuePlaceholder, resultFile)
	}
	var captured bytes.Buffer
	if err := RunReview(submit, &captured); err != nil {
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

	// pi is host-mediated: the collection input carries the materialize
	// prelude plus a submission descriptor, never --execute (#4611).
	var pi bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2,
		"--agent", string(model.AgentPi), "--next-transition",
	}, &pi); err != nil {
		t.Fatal(err)
	}
	var piStatus ReviewTargetStatusResult
	decodeStrictReviewJSON(t, pi.Bytes(), &piStatus)
	if piStatus.NextTransition == nil || piStatus.NextTransition.ReasonCode != "targeted_validation_required" ||
		piStatus.NextTransition.Collect == nil || len(piStatus.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("pi validation transition changed: %#v", piStatus.NextTransition)
	}
	piInput := piStatus.NextTransition.Collect.Inputs[0]
	piArguments := map[string]string{}
	for _, argument := range piInput.Arguments {
		piArguments[argument.Name] = argument.Token
	}
	if piInput.CaptureOperation != reviewCaptureValidationCaptureOperation || piInput.ValidationRequest == nil ||
		piArguments["materialize"] != "--materialize=true" || piArguments["execute"] != "" || piInput.Submission == nil ||
		piInput.Submission.OperationToken != "capture-validation" || piInput.Submission.Value == nil ||
		piInput.Submission.Value.Domain != "artifact_path_or_stdin" || piInput.Submission.Value.Schema != reviewValidatorSchemaID {
		t.Fatalf("pi validation rendering = %#v, submission = %#v", piInput, piInput.Submission)
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

// START admits a candidate by proving the lens block assembles, and the lens
// block summarizes generated paths. The refuter and the targeted validator
// answer only findings a lens issued, and a lens is told that anything it
// cannot see is not evidence, so no lens finding can ever cite generated
// content. Materializing that content for them would let START admit a
// candidate whose refuter evidence no runtime can hold: the unexecutable
// lineage #3367 closed and #4680 reopened by a different door.
func TestReviewProviderMaterializeEvidenceSummarizesGeneratedPaths(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeGeneratedSummaryCandidateFixture(t, repo)
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := reviewProviderMaterializeEvidence(context.Background(), repo, "claude-code", snapshot)
	if err != nil {
		t.Fatalf("materialize evidence: %v", err)
	}
	byPath := make(map[string]string, len(evidence))
	for _, item := range evidence {
		byPath[item.Path] = item.Content
	}
	for _, generated := range []string{"go.sum", "web/package-lock.json", "internal/render/testdata/golden/rendered.golden"} {
		content, ok := byPath[generated]
		if !ok {
			t.Fatalf("generated path %q is missing from refuter evidence", generated)
		}
		var summary map[string]any
		if err := json.Unmarshal([]byte(content), &summary); err != nil {
			t.Fatalf("generated path %q carries content hunks instead of a metadata summary: %v\n%s", generated, err, content)
		}
		if summary["generated"] != true || summary["content_omitted"] != true {
			t.Fatalf("generated path %q summary is not marked generated and omitted: %s", generated, content)
		}
	}
	authored, ok := byPath["internal/auth/token.go"]
	if !ok || !strings.Contains(authored, "+func Token() string") {
		t.Fatalf("authored path lost its complete patch: %q", authored)
	}
}
