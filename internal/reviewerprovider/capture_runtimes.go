package reviewerprovider

import "github.com/gentleman-programming/gentle-ai/v2/internal/model"

// ApprovedRuntimeContextBudget is the provider-owned cap on the complete
// reviewer context one runtime is handed for a single capture: the whole
// materialized lens block, wrappers and result schema included. It is a
// conservative policy cap agreed for every runtime the contract admits,
// independent of the unchanged native per-command Git diff ceiling, and it is
// policy rather than a measured model-capacity guarantee: no runtime here is
// being told what it can hold, only what this product will ask it to hold.
const ApprovedRuntimeContextBudget = 200 << 10

// RuntimeContextBudget reports the approved complete reviewer-context budget
// for one runtime identity. Every registered runtime currently shares the
// same approved cap; that equality is the decided policy, not a model
// guarantee, so a future per-runtime cap specializes the registered case
// below instead of widening it. An identity the contract does not register --
// including the empty identity a manual, non-agent review carries -- fails
// closed: it never receives a larger budget than a supported one, and whether
// an unknown identity may act at all stays with the existing runtime
// validation paths (negotiated START's immutable-transport admission, adapter
// resolution), which this function does not replace.
func RuntimeContextBudget(agent model.AgentID) int {
	if RegisteredRuntime(agent) {
		return ApprovedRuntimeContextBudget
	}
	// Fail closed: an unknown runtime identity earns the same conservative
	// cap, never a larger one.
	return ApprovedRuntimeContextBudget
}

// CapturesInProcess reports whether this runtime's compiled transport runs the
// reviewer itself, inside the capture command, rather than relying on a host or
// a caller to carry the reviewer's evidence to it.
//
// It is the single answer to a question two very different surfaces ask. The
// dispatcher asks it to decide whether `review capture-result --agent <runtime>`
// may execute an adapter at all; the orchestrator contract renderer asks it to
// decide whether the parent should be told to assemble and relay a reviewer
// prompt. Those two answers must be the same answer, because a parent told to
// relay for a runtime that captures in process reproduces the entire immutable
// candidate verbatim for every lens -- roughly 145 KB per lens on a 60-file
// candidate -- to reach a result one command already produces from nothing
// (issue #3825).
//
// It lives here, beside the adapters it describes, for the reason issue #2777
// established: a fact restated in two packages is a fact that eventually
// disagrees with itself.
func CapturesInProcess(agent model.AgentID) bool {
	switch agent {
	case model.AgentClaudeCode, model.AgentCodex:
		return true
	default:
		return false
	}
}
