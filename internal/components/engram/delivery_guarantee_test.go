package engram

import (
	"strings"
	"testing"
)

// normalizeForSemanticMatch collapses all whitespace runs (including line
// wraps) into single spaces and lowercases, so semantic invariants are
// asserted on meaning-bearing phrases rather than on exact formatting or
// byte layout (issue #1042 acceptance scope).
func normalizeForSemanticMatch(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// deliveryGuaranteeInvariants are the semantic rules every rendered protocol
// surface must state so that memory bookkeeping can never replace the final
// user-facing reply — including when a memory call fails or times out.
var deliveryGuaranteeInvariants = []struct {
	name   string
	phrase string
}{
	{"memory-is-bookkeeping", "bookkeeping"},
	{"saving-never-counts-as-answering", "never counts as answering"},
	{"turn-ends-with-answer-as-final-message", "final message"},
	{"save-before-composing-the-reply", "before composing"},
	{"covers-memory-call-failure-and-timeout", "fails or times out"},
	{"failure-still-delivers-the-answer", "deliver the"},
	{"failure-never-replaces-the-reply", "answer anyway"},
}

// TestDeliveryGuaranteeSemantics_RenderedSurfaces asserts the delivery
// guarantee on every rendered protocol variant, independent of golden
// byte-equality: full (non-slim adapters), slim (Claude Code), and the
// Codex model_instructions content.
func TestDeliveryGuaranteeSemantics_RenderedSurfaces(t *testing.T) {
	surfaces := map[string]string{
		"full":               protocolFull(),
		"slim":               protocolSlim(),
		"codex-instructions": codexInstructions(),
	}

	for surfaceName, content := range surfaces {
		normalized := normalizeForSemanticMatch(content)
		for _, inv := range deliveryGuaranteeInvariants {
			if !strings.Contains(normalized, inv.phrase) {
				t.Errorf("surface %q violates delivery-guarantee invariant %q: missing phrase %q", surfaceName, inv.name, inv.phrase)
			}
		}
	}
}
