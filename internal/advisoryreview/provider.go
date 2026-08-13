package advisoryreview

import (
	"context"
)

// Provider is the typed provider contract (REQ-RPC-1): prompt rendering
// (PromptFor) and admission validation (Validate) under the pinned budgets
// (MaxEvidenceEntries=32, maxResultBytes=64KiB, maxEvidenceBytes==
// MaxFrozenCandidateDiffBytes), OutputSchema==ReviewerResultSchema.
//
// PromptFor/Validate render byte-identical output to the pre-extraction path
// (the package-level PromptFor/Prompt/Validate) for equal input (REQ-RPC-2,
// SEN-RPC-3). Validate carries the Request because admission is binding-bound:
// the frozen free function Validate(raw, request) is the sole admission
// authority, and the typed method mirrors it exactly rather than inventing a
// weaker result shape.
type Provider interface {
	// PromptFor renders the canonical prompt bytes for a bounded review
	// request: the same bytes every supported runtime already receives.
	PromptFor(ctx context.Context, r Request) ([]byte, error)
	// Validate admits the raw final output against the request binding,
	// budget-first and never truncating (SEN-RPC-1/2).
	Validate(ctx context.Context, raw []byte, r Request) (ValidatedResult, error)
}

// Invoker is the adapter seam (REQ-RPC-4/5): adapters invoke the reviewer for
// a request and return untouched raw bytes plus a transport error, never
// fabricated, parsed, or validated bytes (SEN-RPC-5).
type Invoker interface {
	Invoke(ctx context.Context, r Request) ([]byte, error)
}

// provider is the default Provider. It delegates PromptFor/Validate to the
// canonical package renderers, which is what keeps the extraction
// byte-identical by construction.
type provider struct{}

// NewProvider returns a Provider backed by the native rendering path.
func NewProvider() Provider { return provider{} }

// PromptFor delegates to Prompt, the one canonical rendering for equal input.
func (provider) PromptFor(_ context.Context, r Request) ([]byte, error) {
	prompt, err := Prompt(r)
	if err != nil {
		return nil, err
	}
	return []byte(prompt), nil
}

// Validate delegates to the freeze-pinned Validate, preserving exact admission
// semantics including refusal before invocation and no truncation.
func (provider) Validate(_ context.Context, raw []byte, r Request) (ValidatedResult, error) {
	return Validate(raw, r)
}

var _ Provider = provider{}
