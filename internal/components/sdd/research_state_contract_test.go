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

func TestResearchCollectorAuthorityIsOutputOnly(t *testing.T) {
	t.Parallel()

	claude, err := assets.Read("claude/agents/sdd-research.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"tools: WebFetch, WebSearch",
		"Do not read or mutate repository or Engram state.",
		"The orchestrator validates and persists this envelope through the selected store route.",
	} {
		if !strings.Contains(claude, required) {
			t.Errorf("Claude research collector missing %q", required)
		}
	}
	for _, forbidden := range []string{"Write", "Edit", "mem_save"} {
		if strings.Contains(claude, forbidden) {
			t.Errorf("Claude research collector grants or requires %q", forbidden)
		}
	}
}

func TestResearchSkillLeavesPersistenceToTheOrchestrator(t *testing.T) {
	t.Parallel()

	skill, err := assets.Read("skills/sdd-research/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"output-only evidence collector",
		"Do not read local artifacts or call persistence tools.",
		"The orchestrator validates and persists the returned envelope through the selected store route.",
	} {
		if !strings.Contains(skill, required) {
			t.Errorf("research skill missing collector boundary %q", required)
		}
	}
	for _, forbidden := range []string{
		"sdd-phase-common.md", "Persist `gentle-ai.sdd-research/v1`", "In hybrid mode, write identical bytes",
	} {
		if strings.Contains(skill, forbidden) {
			t.Errorf("research skill retains child persistence duty %q", forbidden)
		}
	}
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
	for _, required := range []string{
		"The orchestrator validates the returned collector envelope and persists it through the selected store route.",
		"both writes MUST succeed for the operation to be complete",
		"failed, missing, unequal, or divergent store",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("persistence-contract.md missing parent-side readiness clause %q", required)
		}
	}
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

// These assert shipped instructions only, not executed recovery, interviews,
// once-only prompting, or persistence. Host runtime acceptance belongs to Pi.
func TestConfirmedProposalHandoffDoesNotInterview(t *testing.T) {
	t.Parallel()

	lifecycle := sharedResearchContract(t, "research-lifecycle.md")
	proposer, err := assets.Read("skills/sdd-propose/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ content, clause string }{
		{lifecycle, "selected research is `done` or research is unselected"},
		{lifecycle, "product decisions are `confirmed`, evidence references are valid, and the selected artifact-store state is ready"},
		{lifecycle, "The proposal handoff carries the state revision, confirmed decisions, and optional evidence references."},
		{proposer, "Confirmed pre-proposal handoff with state revision, confirmed decisions, and optional exploration/research references"},
		{proposer, "The proposer MUST NOT interview, infer consent, or repair pending decisions; return `blocked` instead."},
	} {
		if !strings.Contains(test.content, test.clause) {
			t.Errorf("shipped handoff contract missing %q", test.clause)
		}
	}
}

func TestAutomaticUnresolvedChoicesEmitOneGroupedPrompt(t *testing.T) {
	t.Parallel()

	content := sharedResearchContract(t, "research-lifecycle.md")
	for _, clause := range []string{
		"The orchestrator owns product discovery.",
		"Automatic unresolved choices require one lossless grouped prompt with all context, options, consequences, allowed answers, and exact tokens",
		"it MUST persist the pending state before prompting, then STOP without invoking `sdd-propose`.",
		"The proposer receives a confirmed pre-proposal handoff and MUST NOT interview or infer consent.",
	} {
		if !strings.Contains(content, clause) {
			t.Errorf("shipped automatic-choice contract missing %q", clause)
		}
	}
	if got := strings.Count(content, "<!-- research-lifecycle-gate:start -->"); got != 1 {
		t.Errorf("managed gate count = %d, want exactly one", got)
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
