package cli

import (
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
		if other.Invocation.ProbeWorkspace() != nil {
			t.Fatalf("%s refuter invocation carries a probe workspace", runtime)
		}
		if other.RequestHash != request.RequestHash {
			t.Fatalf("%s request hash %q differs from Codex %q: the runtime paragraph must not rebind the batch", runtime, other.RequestHash, request.RequestHash)
		}
	}
}
