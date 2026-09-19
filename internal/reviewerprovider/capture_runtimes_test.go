package reviewerprovider

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// TestRuntimeBudgetCoversEveryRegisteredRuntime pins the approved per-runtime
// context policy to the closed runtime registry: every identity the provider
// contract admits receives exactly the approved cap. The table exists so a
// future per-runtime cap must specialize RuntimeContextBudget deliberately
// instead of quietly leaving a runtime on a value nobody decided.
func TestRuntimeBudgetCoversEveryRegisteredRuntime(t *testing.T) {
	for _, identity := range RegisteredRuntimeIdentities() {
		t.Run(identity, func(t *testing.T) {
			if got := RuntimeContextBudget(model.AgentID(identity)); got != ApprovedRuntimeContextBudget {
				t.Fatalf("RuntimeContextBudget(%q) = %d, want the approved %d byte runtime cap", identity, got, ApprovedRuntimeContextBudget)
			}
		})
	}
}

// TestRuntimeBudgetUnknownIdentityFailsClosed pins the fail-closed answer for
// an identity the contract does not register, including the empty identity a
// manual, non-agent review carries: it never receives a larger budget than a
// supported one. Whether an unknown identity may act at all stays with the
// existing runtime validation paths; the budget only makes sure it can never
// buy its way to a wider context than an admitted runtime.
func TestRuntimeBudgetUnknownIdentityFailsClosed(t *testing.T) {
	for _, identity := range []model.AgentID{"", "unknown-runtime", "claude-code-impersonator"} {
		if got := RuntimeContextBudget(identity); got != ApprovedRuntimeContextBudget {
			t.Fatalf("RuntimeContextBudget(%q) = %d, want the fail-closed %d byte cap; an unknown runtime never earns a larger budget", identity, got, ApprovedRuntimeContextBudget)
		}
	}
}
