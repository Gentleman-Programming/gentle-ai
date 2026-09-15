package sdd

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
)

// Contract assertions describe shipped guidance, not live interviews or persistence.
func TestResearchCollectorsKeepAuthorityOutputOnly(t *testing.T) {
	for _, path := range []string{"claude/agents/sdd-research.md", "cursor/agents/sdd-research.md", "kimi/agents/sdd-research.md", "kiro/agents/sdd-research.md", "skills/sdd-research/SKILL.md"} {
		t.Run(path, func(t *testing.T) {
			content := assets.MustRead(path)
			for _, required := range []string{"output-only evidence collector", "Do not read local artifacts or call persistence tools.", "Do not read or mutate repository or Engram state.", "actually available and authorized external tools", "primary sources", "contradictions", "freshness", "do not interview", "infer consent"} {
				if !strings.Contains(content, required) {
					t.Errorf("%s missing %q", path, required)
				}
			}
			for _, forbidden := range []string{"gentle-ai.sdd-research-capability/v1", "supplies the immutable request", "Unsupported or undeclared classes deny admission", "persist blocked recovery state"} {
				if strings.Contains(content, forbidden) {
					t.Errorf("%s retains %q", path, forbidden)
				}
			}
		})
	}
	for _, path := range []string{"cursor/agents/sdd-research.md", "kimi/agents/sdd-research.md"} {
		if !strings.Contains(assets.MustRead(path), "readonly: true") {
			t.Errorf("%s lost read-only execution", path)
		}
	}
	kiro := assets.MustRead("kiro/agents/sdd-research.md")
	if !strings.Contains(kiro, `tools: ["@context7"]`) || strings.Contains(kiro, "@builtin") || strings.Contains(kiro, "@engram") {
		t.Fatal("Kiro research tools widened")
	}
	claude := assets.MustRead("claude/agents/sdd-research.md")
	if !strings.Contains(claude, "tools: WebFetch, WebSearch") {
		t.Fatal("Claude research tools changed")
	}
}

func TestResearchCommandsDoNotRequireAdministrativePreflight(t *testing.T) {
	for _, path := range []string{"claude/commands/gentle-sdd-research.md", "opencode/commands/sdd-research.md"} {
		content := assets.MustRead(path)
		for _, required := range []string{"Research remains optional, including after selection.", "Ask one focused question", "wait", "actually available and authorized external tools", "preserve", "claims"} {
			if !strings.Contains(strings.ToLower(content), strings.ToLower(required)) {
				t.Errorf("%s missing %q", path, required)
			}
		}
		for _, forbidden := range []string{"must already be complete", "selected research request must exist", "hybrid mismatch blocks", "Exact grants", "Persist intent before source access"} {
			if strings.Contains(content, forbidden) {
				t.Errorf("%s retains administrative admission %q", path, forbidden)
			}
		}
	}
}

func TestResearchNotesPreserveHistoryWithoutReadinessAuthority(t *testing.T) {
	for _, name := range []string{"research-lifecycle.md", "persistence-contract.md", "engram-convention.md", "openspec-convention.md", "sdd-status-contract.md"} {
		content := assets.MustRead("skills/_shared/" + name)
		for _, forbidden := range []string{"gentle-ai.sdd-research/v1", "gentle-ai.sdd-preproposal/v1", "proposal_ready", "selected research must be `done`", "matching hybrid state", "same revision and bytes on readback"} {
			if strings.Contains(content, forbidden) {
				t.Errorf("%s retains research readiness authority %q", name, forbidden)
			}
		}
	}
	for _, name := range []string{"engram-convention.md", "openspec-convention.md"} {
		content := assets.MustRead("skills/_shared/" + name)
		if !strings.Contains(content, "Preserve historical research") {
			t.Errorf("%s must preserve existing research", name)
		}
	}
}
