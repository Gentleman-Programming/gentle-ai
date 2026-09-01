package reviewerprovider

import "github.com/gentleman-programming/gentle-ai/v2/internal/model"

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
