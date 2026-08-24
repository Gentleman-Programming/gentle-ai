package sdd

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
)

func sharedResearchContract(t *testing.T, name string) string {
	t.Helper()
	content, err := assets.Read("skills/_shared/" + name)
	if err != nil {
		t.Fatalf("read shared research contract %q: %v", name, err)
	}
	return content
}

func TestResearchEvidenceContractIsCompleteAndFailClosed(t *testing.T) {
	t.Parallel()

	content := sharedResearchContract(t, "research-lifecycle.md")
	for _, clause := range []string{
		"gentle-ai.sdd-research/v1",
		"questions",
		"admission and observed exact grants",
		"id, class, title, publisher, URL, accessed_at, excerpt",
		"each claim maps to source IDs",
		"contradictions",
		"uncertainty and freshness",
		"product choices are separate and non-authoritative",
		"partial or blocked outcomes MUST exclude unvalidated claims",
	} {
		if !strings.Contains(content, clause) {
			t.Errorf("research-lifecycle.md missing evidence clause %q", clause)
		}
	}
}

func TestSelectedResearchReadinessMatrixFailsClosed(t *testing.T) {
	t.Parallel()

	content := sharedResearchContract(t, "persistence-contract.md")
	tests := []struct {
		name  string
		state string
		ready string
	}{
		{name: "OpenSpec-only done", state: "openspec | done | valid | OpenSpec success and readback | confirmed", ready: "yes"},
		{name: "Engram-only done", state: "engram | done | valid | Engram success and readback | confirmed", ready: "yes"},
		{name: "complete matching hybrid done", state: "hybrid | done | valid | OpenSpec and Engram success; same revision and bytes on readback | confirmed", ready: "yes"},
		{name: "no-store selected research", state: "none | done | valid | no store | any", ready: "no"},
		{name: "partial", state: "any | partial | any | any | any", ready: "no"},
		{name: "blocked", state: "any | blocked | any | any | any", ready: "no"},
		{name: "missing evidence", state: "any | done | missing or invalid | any | any", ready: "no"},
		{name: "hybrid persistence divergence", state: "hybrid | done | valid | failed, missing, unequal, or divergent store | any", ready: "no"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			row := "| " + test.state + " | " + test.ready + " |"
			if !strings.Contains(content, row) {
				t.Errorf("persistence-contract.md missing readiness row %q", row)
			}
		})
	}
}

func TestHybridRestartRequiresByteEqualStateWithoutStorePreference(t *testing.T) {
	t.Parallel()

	contracts := map[string][]string{
		"research-lifecycle.md": {
			"gentle-ai.sdd-preproposal/v1", "revision", "research request and classes",
			"admission and outcome", "OpenSpec and Engram evidence references", "proposal_ready",
		},
		"persistence-contract.md": {
			"matching restart restores the request and evidence references",
			"retain pre-write intent and canonical desired content", "never derive content from either surviving store",
			"new positive revision to both stores", "read and compare both before readiness",
			"remain blocked and require explicit re-entry", "never invent state",
		},
		"engram-convention.md":   {"sdd/{change-name}/research", "gentle-ai.sdd-research/v1", "gentle-ai.sdd-preproposal/v1"},
		"openspec-convention.md": {"research.md", "gentle-ai.sdd-research/v1", "gentle-ai.sdd-preproposal/v1"},
		"sdd-status-contract.md": {"gentle-ai.sdd-status/v2", "pre-proposal gate", "matching hybrid state"},
	}

	for name, clauses := range contracts {
		content := sharedResearchContract(t, name)
		for _, clause := range clauses {
			if !strings.Contains(content, clause) {
				t.Errorf("%s missing hybrid state clause %q", name, clause)
			}
		}
	}
}
