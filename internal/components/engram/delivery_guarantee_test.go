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

type deliveryGuaranteeInvariant struct {
	name   string
	phrase string
}

// deliveryGuaranteeInvariants are the semantic rules every rendered protocol
// surface must state so that memory bookkeeping can never replace the final
// user-facing reply.
var deliveryGuaranteeInvariants = []deliveryGuaranteeInvariant{
	{"memory-is-bookkeeping", "bookkeeping"},
	{"saving-never-counts-as-answering", "never counts as answering"},
	{"turn-ends-with-answer-as-final-message", "final message"},
	{"save-before-composing-the-reply", "before composing"},
}

// The failure contracts deliberately include both the positive delivery
// obligation and the negative blocking/replacement prohibition. Independent
// fragments such as "answer anyway" cannot prove the direction of the rule.
var fullFailureContract = []deliveryGuaranteeInvariant{
	{"failure-delivers-complete-answer", "fails or times out, deliver the complete answer anyway and note the failure briefly"},
	{"failure-never-blocks-truncates-or-replaces", "failed or slow memory operation never blocks, truncates, or replaces the reply"},
}

var slimFailureContract = []deliveryGuaranteeInvariant{
	{"failure-delivers-answer", "fails or times out, deliver the answer anyway"},
	{"failure-never-blocks-or-replaces", "memory failures never block or replace the reply"},
}

func missingDeliveryGuaranteeInvariants(content string, failureContract []deliveryGuaranteeInvariant) []string {
	normalized := normalizeForSemanticMatch(content)
	invariants := append(append([]deliveryGuaranteeInvariant{}, deliveryGuaranteeInvariants...), failureContract...)
	missing := make([]string, 0)
	for _, inv := range invariants {
		if !strings.Contains(normalized, inv.phrase) {
			missing = append(missing, inv.name)
		}
	}
	return missing
}

// TestDeliveryGuaranteeSemantics_RenderedSurfaces asserts the delivery
// guarantee on every rendered protocol variant, independent of golden
// byte-equality: full (non-slim adapters), slim (Claude Code), and the
// Codex model_instructions content.
func TestDeliveryGuaranteeSemantics_RenderedSurfaces(t *testing.T) {
	surfaces := map[string]struct {
		content         string
		failureContract []deliveryGuaranteeInvariant
	}{
		"full":               {content: protocolFull(), failureContract: fullFailureContract},
		"slim":               {content: protocolSlim(), failureContract: slimFailureContract},
		"codex-instructions": {content: codexInstructions(), failureContract: fullFailureContract},
	}

	for surfaceName, surface := range surfaces {
		for _, missing := range missingDeliveryGuaranteeInvariants(surface.content, surface.failureContract) {
			t.Errorf("surface %q violates delivery-guarantee invariant %q", surfaceName, missing)
		}
	}
}

func TestDeliveryGuaranteeSemantics_RejectsOppositeFailureContract(t *testing.T) {
	const fullPositive = "If a memory call (`mem_save`, `mem_judge`, `mem_session_summary`) fails or times out, deliver the complete answer anyway and note the failure briefly — a failed or slow memory operation never blocks, truncates, or replaces the reply."
	const fullOpposite = "If a memory call (`mem_save`, `mem_judge`, `mem_session_summary`) fails or times out, do not deliver the complete answer anyway; a failed or slow memory operation may block, truncate, or replace the reply."
	const slimPositive = "If a memory call fails or times out, deliver the answer anyway — memory failures never block or replace the reply."
	const slimOpposite = "If a memory call fails or times out, do not deliver the answer anyway — memory failures may block or replace the reply."

	tests := []struct {
		name            string
		content         string
		positive        string
		opposite        string
		failureContract []deliveryGuaranteeInvariant
	}{
		{name: "full", content: protocolFull(), positive: fullPositive, opposite: fullOpposite, failureContract: fullFailureContract},
		{name: "slim", content: protocolSlim(), positive: slimPositive, opposite: slimOpposite, failureContract: slimFailureContract},
		{name: "codex-instructions", content: codexInstructions(), positive: fullPositive, opposite: fullOpposite, failureContract: fullFailureContract},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalizedContent := normalizeForSemanticMatch(tt.content)
			mutated := strings.Replace(
				normalizedContent,
				normalizeForSemanticMatch(tt.positive),
				normalizeForSemanticMatch(tt.opposite),
				1,
			)
			if mutated == normalizedContent {
				t.Fatalf("positive failure contract was not found in %s surface", tt.name)
			}
			if missing := missingDeliveryGuaranteeInvariants(mutated, tt.failureContract); len(missing) == 0 {
				t.Fatalf("opposite failure contract retained weak fragments and was incorrectly accepted")
			}
		})
	}
}
