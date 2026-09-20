package sdd

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
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

	// The Claude research COMMAND (claude/commands/sdd-research.md, V3: shipped
	// unprefixed, never as upstream's own gentle-sdd-research.md) is a distinct
	// asset from the Claude research AGENT checked above: it identifies the
	// command actor as the orchestrator and hands off persistence exactly once,
	// never duplicated.
	claudeCommand, err := assets.Read("claude/commands/sdd-research.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(claudeCommand, "The command actor is the orchestrator.") {
		t.Error("Claude research command does not identify the command actor as the orchestrator")
	}
	if got := strings.Count(claudeCommand, "The orchestrator handles any authorized persistence after the collector returns."); got != 1 {
		t.Errorf("Claude research command has %d persistence handoffs, want 1", got)
	}
	for _, forbidden := range []string{"SDD Session Preflight and `sdd-init` must already be complete", "blocks proposal readiness"} {
		if strings.Contains(claudeCommand, forbidden) {
			t.Errorf("Claude research command retains closed-admission wording %q", forbidden)
		}
	}
}
