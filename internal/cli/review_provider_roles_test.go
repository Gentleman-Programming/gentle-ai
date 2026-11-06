package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// runtimeBudgetRolePromptRequest builds the smallest refuter request whose
// complete serialized prompt size is controlled by one evidence content
// string. Plain ASCII grows the JSON payload byte for byte, so the boundary
// tests below can place the finished prompt exactly at the approved runtime
// cap and one byte over it.
func runtimeBudgetRolePromptRequest(content string) reviewProviderRefuterRequest {
	return reviewProviderRefuterRequest{
		Schema: "gentle-ai.review-provider-refuter-request/v1", LineageID: "lineage",
		AuthorityVersion: "revision", TargetIdentity: "target", SnapshotIdentity: "target",
		Claims:   []reviewtransaction.RefuterClaim{},
		Evidence: []reviewProviderEvidence{{Path: "path.txt", Content: content}},
	}
}

// TestReviewProviderRolePromptBoundsTheCompletePromptAtTheRuntimeCap pins the
// complete-prompt input ceiling to the approved runtime budget: the value the
// cap bounds is the finished serialized prompt -- escaped content, policy
// appendix, instruction, and result schema included -- not the raw evidence
// the request was built from. Boundaries are exact: a prompt of exactly the
// cap is handed over, one byte over refuses with the typed budget refusal,
// and content whose raw bytes fit while its JSON-escaped bytes do not refuses
// because the escaped form is what the runtime is actually handed.
func TestReviewProviderRolePromptBoundsTheCompletePromptAtTheRuntimeCap(t *testing.T) {
	contract, err := reviewProviderRoleContractFor(reviewProviderRoleRefuter)
	if err != nil {
		t.Fatal(err)
	}
	runtime := string(model.AgentClaudeCode)
	probe, err := reviewProviderRolePrompt(contract, runtimeBudgetRolePromptRequest(""), runtime)
	if err != nil {
		t.Fatal(err)
	}
	// Plain ASCII content grows the serialized payload byte for byte: the
	// prompt for content of n bytes is exactly len(probe)+n.
	base := len(probe)

	atCap, err := reviewProviderRolePrompt(contract, runtimeBudgetRolePromptRequest(strings.Repeat("a", reviewRuntimeBudgetTestCapBytes-base)), runtime)
	if err != nil {
		t.Fatalf("complete prompt of exactly the runtime cap was refused: %v", err)
	}
	if len(atCap) != reviewRuntimeBudgetTestCapBytes {
		t.Fatalf("boundary probe = %d bytes, want exactly the %d byte runtime cap", len(atCap), reviewRuntimeBudgetTestCapBytes)
	}

	_, err = reviewProviderRolePrompt(contract, runtimeBudgetRolePromptRequest(strings.Repeat("a", reviewRuntimeBudgetTestCapBytes-base+1)), runtime)
	var refusal *reviewLensContextError
	if !errors.As(err, &refusal) || refusal.Code != "lens_context_budget_exceeded" {
		t.Fatalf("complete prompt one byte over the runtime cap = %v, want the typed budget refusal", err)
	}

	// Escaped-content proof: every raw quote character doubles under JSON
	// escaping. Sized so the raw bytes fit the cap but the escaped prompt
	// cannot, this only refuses when the bound is measured on the serialized
	// form -- measuring raw evidence bytes would materialize the prompt.
	escaped := strings.Repeat(`"`, 150_000)
	if raw, err := reviewProviderRolePrompt(contract, runtimeBudgetRolePromptRequest(escaped), ""); err == nil && len(raw) <= reviewRuntimeBudgetTestCapBytes {
		t.Fatalf("escaped content of %d raw bytes produced a %d byte prompt with no refusal", len(escaped), len(raw))
	} else if !errors.As(err, &refusal) || refusal.Code != "lens_context_budget_exceeded" {
		t.Fatalf("escaped-content prompt over the runtime cap = %v, want the typed budget refusal", err)
	}
}

// TestReviewProviderRolePromptKeepsTheOutputLimitDistinct proves the native
// per-invocation output limit survives beside the runtime input ceiling
// without being conflated with it: a contract whose output limit sits under
// the runtime cap still refuses on its own native limit, while a prompt over
// both bounds refuses on the tighter input ceiling first.
func TestReviewProviderRolePromptKeepsTheOutputLimitDistinct(t *testing.T) {
	runtime := string(model.AgentClaudeCode)
	tiny := reviewerprovider.Contract{
		Role: reviewerprovider.RoleRefuter, PromptInstruction: "instruction",
		ResultSchema: []byte("{}"), ResultLimit: 10,
	}
	_, err := reviewProviderRolePrompt(tiny, runtimeBudgetRolePromptRequest(""), runtime)
	if err == nil || !strings.Contains(err.Error(), "native 10 byte limit") {
		t.Fatalf("prompt over the contract's own output limit = %v, want the native limit refusal", err)
	}
	var refusal *reviewLensContextError
	if errors.As(err, &refusal) {
		t.Fatalf("the native output limit refusal must not be the runtime budget refusal: %v", err)
	}

	contract, err := reviewProviderRoleContractFor(reviewProviderRoleRefuter)
	if err != nil {
		t.Fatal(err)
	}
	_, err = reviewProviderRolePrompt(contract, runtimeBudgetRolePromptRequest(strings.Repeat("a", contract.ResultLimit)), runtime)
	if !errors.As(err, &refusal) || refusal.Code != "lens_context_budget_exceeded" {
		t.Fatalf("prompt over both bounds = %v, want the tighter runtime input ceiling", err)
	}
}

// TestReviewProviderRefuterPromptIsRuntimeConditional pins the S11 refuter
// instruction: the Codex prompt offers one isolated reproducing probe, every
// other runtime is told by name that the probe is unavailable, and the
// targeted-validator prompt gains neither paragraph.
func TestReviewProviderRefuterPromptIsRuntimeConditional(t *testing.T) {
	refuter, err := reviewProviderRoleContractFor(reviewProviderRoleRefuter)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := reviewProviderRoleContractFor(reviewProviderRoleTargetedValidator)
	if err != nil {
		t.Fatal(err)
	}
	for _, runtime := range []model.AgentID{model.AgentCodex, model.AgentClaudeCode, model.AgentPi, model.AgentOpenCode} {
		prompt, err := reviewProviderRolePrompt(refuter, runtimeBudgetRolePromptRequest(""), string(runtime))
		if err != nil {
			t.Fatal(err)
		}
		probeOffered := strings.Contains(string(prompt), "one reproducing command")
		if probeOffered != (runtime == model.AgentCodex) {
			t.Fatalf("%s refuter prompt offers a probe = %t:\n%s", runtime, probeOffered, prompt)
		}
		if note := reviewerprovider.RefuterProbeUnavailableNote(runtime); runtime != model.AgentCodex && !strings.Contains(string(prompt), note) {
			t.Fatalf("%s refuter prompt omits %q:\n%s", runtime, note, prompt)
		}
		validatorPrompt, err := reviewProviderRolePrompt(validator, reviewProviderTargetedValidatorRequest{}, string(runtime))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(validatorPrompt), "one reproducing command") || strings.Contains(string(validatorPrompt), "probe unavailable on") {
			t.Fatalf("%s targeted-validator prompt gained the refuter probe paragraph:\n%s", runtime, validatorPrompt)
		}
	}
}

// TestReviewProviderRefuterAdmissionNotesProbeUnavailable pins the explicit
// no-probe note: on every runtime whose adapter cannot isolate a probe, each
// admitted refuter result carries "probe unavailable on <runtime>" in its
// existing proof_refs, written by Go so it never depends on the model. A Codex
// result, whose refuter could probe, is admitted untouched.
func TestReviewProviderRefuterAdmissionNotesProbeUnavailable(t *testing.T) {
	request := runtimeBudgetRolePromptRequest("")
	request.RequestHash = "sha256:" + strings.Repeat("a", 64)
	request.Claims = []reviewtransaction.RefuterClaim{{FindingID: "R3-001", SnapshotIdentity: request.SnapshotIdentity, Proof: "tracked.txt:1 lens proof", Claim: "candidate failure"}}
	raw := []byte(`{"refuter_request_hash":"` + request.RequestHash + `","results":[{"finding_id":"R3-001","outcome":"refuted","proof_refs":["tracked.txt:1 baseline already fails"]}]}`)
	for _, runtime := range []model.AgentID{model.AgentClaudeCode, model.AgentPi, model.AgentOpenCode, model.AgentCodex} {
		request.Runtime = string(runtime)
		result, err := reviewProviderAdmitRefuterRaw(request, raw)
		if err != nil {
			t.Fatalf("%s admission: %v", runtime, err)
		}
		want := []string{"tracked.txt:1 baseline already fails"}
		if runtime != model.AgentCodex {
			want = append(want, "probe unavailable on "+string(runtime))
		}
		if got := result.Results[0].ProofRefs; strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("%s admitted proof_refs = %q, want %q", runtime, got, want)
		}
		if outcome := result.Results[0].Outcome; outcome != reviewtransaction.OutcomeRefuted {
			t.Fatalf("%s outcome = %q, want the provider's refuted outcome unchanged", runtime, outcome)
		}
	}
	// A refuter that already wrote the note does not get it twice.
	request.Runtime = string(model.AgentPi)
	noted := []byte(`{"refuter_request_hash":"` + request.RequestHash + `","results":[{"finding_id":"R3-001","outcome":"corroborated","proof_refs":["tracked.txt:1 read","probe unavailable on pi"]}]}`)
	result, err := reviewProviderAdmitRefuterRaw(request, noted)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Results[0].ProofRefs; len(got) != 2 {
		t.Fatalf("noted proof_refs = %q, want the note once", got)
	}
}

// TestReviewProviderCodexRefuterInvocationMaterializesTheCandidateTree pins
// the Go half of the S11 probe: a Codex refuter invocation carries a probe
// workspace that writes the frozen candidate tree, not the live workspace, and
// no other runtime's invocation carries one.
func TestReviewProviderCodexRefuterInvocationMaterializesTheCandidateTree(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, store, record, _ := piRefuterReview(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("drifted after start\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := record.State
	state.RuntimeAgent = string(model.AgentCodex)
	request, err := reviewProviderNewRefuterRequest(t.Context(), repo, store.Dir, state, state.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}
	workspace := request.Invocation.ProbeWorkspace()
	if workspace == nil {
		t.Fatal("Codex refuter invocation carries no probe workspace")
	}
	if got := request.Invocation.ProbeSourceRoot(); got != repo {
		t.Fatalf("Codex refuter probe source root = %q, want the reviewed repository %q", got, repo)
	}
	dir := t.TempDir()
	if err := workspace(t.Context(), dir); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "tracked.txt")); err != nil || string(got) != "candidate\n" {
		t.Fatalf("probe copy tracked.txt = %q, %v; want the frozen candidate", got, err)
	}
	if !strings.Contains(string(request.Invocation.Prompt()), "one reproducing command") {
		t.Fatal("Codex refuter prompt does not offer the probe its invocation carries")
	}

	for _, runtime := range []model.AgentID{model.AgentClaudeCode, model.AgentPi, model.AgentOpenCode} {
		state.RuntimeAgent = string(runtime)
		other, err := reviewProviderNewRefuterRequest(t.Context(), repo, store.Dir, state, state.CapturePhaseRevision)
		if err != nil {
			t.Fatal(err)
		}
		if other.Invocation.ProbeWorkspace() != nil || other.Invocation.ProbeSourceRoot() != "" {
			t.Fatalf("%s refuter invocation carries a probe workspace", runtime)
		}
		if other.RequestHash != request.RequestHash {
			t.Fatalf("%s request hash %q differs from Codex %q: the runtime paragraph must not rebind the batch", runtime, other.RequestHash, request.RequestHash)
		}
	}
}

// TestReviewProviderRefuterClaimsIncludeDeterministicSevereFindings pins L20:
// the refuter batch carries every severe candidate-caused finding, deterministic
// or inferential, so a deterministic false positive can be dropped. Non-severe
// findings never carry a classification and never reach the batch; severe
// findings that are pre-existing, base-only, unknown, or insufficient stay out
// too (PRESERVE).
func TestReviewProviderRefuterClaimsIncludeDeterministicSevereFindings(t *testing.T) {
	snapshot := "sha256:" + strings.Repeat("1", 64)
	finding := func(id string) reviewtransaction.Finding {
		return reviewtransaction.Finding{ID: id, Claim: "claim " + id}
	}
	classification := func(id string, class reviewtransaction.EvidenceClass, causality reviewtransaction.CausalDisposition) reviewtransaction.FindingEvidence {
		return reviewtransaction.FindingEvidence{FindingID: id, Severity: "CRITICAL", Class: class, Causality: causality, Proof: "proof " + id}
	}
	input := reviewtransaction.CompactReviewInput{
		LensResults: []reviewtransaction.LensResult{{Findings: []reviewtransaction.Finding{
			finding("D-introduced"), finding("I-worsened"), finding("D-pre-existing"), finding("D-unknown"), finding("X-insufficient"), finding("W-warning"),
		}}},
		Classifications: []reviewtransaction.FindingEvidence{
			classification("D-introduced", reviewtransaction.EvidenceDeterministic, reviewtransaction.CausalIntroduced),
			classification("I-worsened", reviewtransaction.EvidenceInferential, reviewtransaction.CausalWorsened),
			classification("D-pre-existing", reviewtransaction.EvidenceDeterministic, reviewtransaction.CausalPreExisting),
			classification("D-unknown", reviewtransaction.EvidenceDeterministic, reviewtransaction.CausalUnknown),
			classification("X-insufficient", reviewtransaction.EvidenceInsufficient, reviewtransaction.CausalIntroduced),
		},
	}
	claims, err := reviewProviderRefuterClaims(snapshot, input, string(model.AgentPi))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, claim := range claims {
		ids = append(ids, claim.FindingID)
		if claim.Claim != "claim "+claim.FindingID || claim.Proof != "proof "+claim.FindingID || claim.SnapshotIdentity != snapshot {
			t.Fatalf("claim %#v does not carry its finding's assertion and proof", claim)
		}
	}
	if got := strings.Join(ids, ","); got != "D-introduced,I-worsened" {
		t.Fatalf("refuter claims = %s, want the two severe candidate-caused findings", got)
	}

	input.Classifications = input.Classifications[2:]
	if _, err := reviewProviderRefuterClaims(snapshot, input, string(model.AgentPi)); !errors.Is(err, errReviewProviderRefuterNotRequired) {
		t.Fatalf("batch without severe candidate-caused findings = %v, want the not-required sentinel", err)
	}
}

// TestReviewProviderRefuterClaimsKeepDeterministicOnlyWhereARefuterRuns: a
// deterministic finding waits for the refuter only when the runtime frozen at
// START can run one. Elsewhere (manual lane, runtimes without a provider
// refuter transport) it blocks directly, as before L20, so the review closes
// instead of stopping at manual_intervention_required. Inferential findings
// always reach the refuter.
func TestReviewProviderRefuterClaimsKeepDeterministicOnlyWhereARefuterRuns(t *testing.T) {
	snapshot := "sha256:" + strings.Repeat("2", 64)
	input := reviewtransaction.CompactReviewInput{
		LensResults: []reviewtransaction.LensResult{{Findings: []reviewtransaction.Finding{{ID: "D-introduced", Claim: "d"}, {ID: "I-worsened", Claim: "i"}}}},
		Classifications: []reviewtransaction.FindingEvidence{
			{FindingID: "D-introduced", Severity: "CRITICAL", Class: reviewtransaction.EvidenceDeterministic, Causality: reviewtransaction.CausalIntroduced, Proof: "proof d"},
			{FindingID: "I-worsened", Severity: "CRITICAL", Class: reviewtransaction.EvidenceInferential, Causality: reviewtransaction.CausalWorsened, Proof: "proof i"},
		},
	}
	for _, tt := range []struct {
		runtime string
		want    string
	}{
		{runtime: string(model.AgentPi), want: "D-introduced,I-worsened"},
		{runtime: string(model.AgentOpenCode), want: "D-introduced,I-worsened"},
		{runtime: string(model.AgentClaudeCode), want: "D-introduced,I-worsened"},
		{runtime: string(model.AgentCodex), want: "D-introduced,I-worsened"},
		{runtime: "", want: "I-worsened"},
		{runtime: string(model.AgentGeminiCLI), want: "I-worsened"},
	} {
		claims, err := reviewProviderRefuterClaims(snapshot, input, tt.runtime)
		if err != nil {
			t.Fatalf("%q: %v", tt.runtime, err)
		}
		var ids []string
		for _, claim := range claims {
			ids = append(ids, claim.FindingID)
		}
		if got := strings.Join(ids, ","); got != tt.want {
			t.Fatalf("runtime %q refuter claims = %s, want %s", tt.runtime, got, tt.want)
		}
	}
	input.Classifications = input.Classifications[:1]
	if _, err := reviewProviderRefuterClaims(snapshot, input, ""); !errors.Is(err, errReviewProviderRefuterNotRequired) {
		t.Fatalf("deterministic-only batch without a refuter runtime = %v, want the not-required sentinel", err)
	}
}

// TestReviewCaptureRefuterDropsADeterministicSevereFinding drives L20 end to
// end through the Pi host relay: a severe deterministic finding now requires
// the refuter batch, and a refuted verdict drops it to a refuted advisory on
// an approved review instead of opening a correction.
func TestReviewCaptureRefuterDropsADeterministicSevereFinding(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	// START freezes the Pi runtime: a deterministic finding waits for the
	// refuter only on a runtime that can run it.
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	started := startFacadeReviewForRuntime(t, repo, model.AgentPi)
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	result := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[0], 0)
	result.Findings = []facadeFinding{{
		ID: "R3-001", Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "candidate failure",
		ProofRefs: []string{"tracked.txt:1 candidate-specific proof"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
		CausalDisposition: reviewtransaction.CausalIntroduced,
	}}
	input := filepath.Join(t.TempDir(), "result.json")
	writeReviewCLIJSON(t, input, result)
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
	}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	record, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.State.State != reviewtransaction.StateReviewing {
		t.Fatalf("deterministic severe finding closed the review as %q before the refuter ran", record.State.State)
	}
	request, err := reviewProviderNewRefuterRequest(t.Context(), repo, store.Dir, record.State, record.State.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Claims) != 1 || request.Claims[0].FindingID != "R3-001" {
		t.Fatalf("refuter claims = %#v, want the deterministic finding", request.Claims)
	}
	raw, err := json.Marshal(facadeRefuterResult{RequestHash: request.RequestHash, Results: []facadeRefuterOutcome{{
		FindingID: "R3-001", Outcome: reviewtransaction.OutcomeRefuted, ProofRefs: []string{"tracked.txt:1 the baseline already prints candidate"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	handle := rctx2ReviewRepositoryContextForTest(t, repo, reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity, Revision: record.State.CapturePhaseRevision,
	})
	var output bytes.Buffer
	if err := RunReview(append(append([]string{"capture-refuter"}, piRefuterBinding(repo, record, handle)...), "--agent", "pi", "--input", writeReviewCLIRawInput(t, raw)), &output); err != nil {
		t.Fatalf("capture refuter: %v\n%s", err, output.String())
	}
	var terminal reviewLastEventClosureResult
	decodeStrictReviewJSON(t, output.Bytes(), &terminal)
	if terminal.State != reviewtransaction.StateApproved {
		t.Fatalf("refuted deterministic finding closed as %q, want approved", terminal.State)
	}
	if terminal.AdvisoryFindings == nil || len(terminal.AdvisoryFindings.Findings) != 1 ||
		terminal.AdvisoryFindings.Findings[0].Disposition != reviewtransaction.AdvisoryRefuted {
		t.Fatalf("advisory findings = %#v, want the one refuted finding", terminal.AdvisoryFindings)
	}
}
