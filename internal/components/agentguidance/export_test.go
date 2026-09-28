package agentguidance

import "github.com/gentleman-programming/gentle-ai/v4/internal/model"

// Test-only seams. Production callers always pass the review contract source
// explicitly through RoutingOptions.ReviewContract / RenderOrchestratorWithSource;
// these helpers let tests render with the package-level fallback.

// SetReviewContractSource sets the package-level fallback used only when a
// caller supplies no source of its own. Production callers pass the source
// explicitly (RoutingOptions.ReviewContract); the fallback exists so tests can
// render with RoutingOptions{} after wiring it once in TestMain.
func SetReviewContractSource(source ReviewContractSource) {
	reviewContractMu.Lock()
	defer reviewContractMu.Unlock()
	reviewContractSource = source
}

// RenderOrchestrator composes the orchestrator instructions one runtime
// installs: its asset with the shared ODD sections expanded, the native review
// execution contract bound to the runtime identity where the runtime receives
// receipt-driven development, and no template tokens.
//
// A runtime outside model.SupportsReceiptDrivenDevelopment receives ODD only:
// the provider defect handoff (which exists to settle the review consent
// envelope) and the review lifecycle are removed, and the verification and
// checking contracts take their ODD-only form. A render that would still carry
// review wording fails closed.
//
// Pi is rejected: its prompt is owned by the Gentle Shell package.
//
// It renders with the package-level review contract source; production callers
// use RenderOrchestratorWithSource or RoutingOptions.ReviewContract instead.
func RenderOrchestrator(agent model.AgentID) (string, error) {
	return RenderOrchestratorWithSource(agent, nil)
}
