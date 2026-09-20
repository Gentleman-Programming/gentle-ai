package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
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
