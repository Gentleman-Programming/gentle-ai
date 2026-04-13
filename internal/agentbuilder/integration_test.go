package agentbuilder

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// cannedSKILL is a valid SKILL.md response that the MockEngine returns.
const cannedSKILL = `# CSS A11y Reviewer

## Description
Reviews CSS files for accessibility issues, focusing on color contrast, focus visibility, and proper ARIA usage.

## Trigger
When the user asks to "review CSS for a11y", "check accessibility in CSS", or "audit CSS accessibility".

## Instructions
1. Scan all CSS files in the project for potential accessibility issues.
2. Check color contrast ratios against WCAG 2.1 AA standards.
3. Verify focus indicators are visible (outline: none without alternative must be flagged).
4. Identify elements that may need ARIA attributes.
5. Generate a structured report with file, line, and issue description.

## Rules
- Always provide specific file and line references.
- Never mark issues as critical without clear WCAG citation.
- Suggest concrete fixes for each issue found.

## Examples
User: "Review CSS for a11y issues"
Agent: Scans and reports: "button.css:14 — focus outline removed without alternative (WCAG 2.4.7)"
`

func TestIntegration_FullAgentBuilderFlow(t *testing.T) {
	// Step 1: Compose prompt.
	sddConfig := (*SDDIntegration)(nil) // standalone
	installedAgents := []model.AgentID{model.AgentClaudeCode}
	prompt := ComposePrompt("build an a11y CSS reviewer", sddConfig, installedAgents)

	if !strings.Contains(prompt, "a11y CSS reviewer") {
		t.Fatalf("prompt missing user input;\ngot:\n%s", prompt)
	}

	// Step 2: Call MockEngine.Generate with the prompt.
	engine := &MockEngine{
		AgentIDVal:  model.AgentClaudeCode,
		Output:      cannedSKILL,
		IsAvailable: true,
	}

	raw, err := engine.Generate(context.Background(), prompt)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Step 3: Parse the result.
	agent, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Step 4: Assert GeneratedAgent has correct fields.
	if agent.Name != "css-a11y-reviewer" {
		t.Errorf("Name = %q, want %q", agent.Name, "css-a11y-reviewer")
	}
	if agent.Title != "CSS A11y Reviewer" {
		t.Errorf("Title = %q, want %q", agent.Title, "CSS A11y Reviewer")
	}
	if !strings.Contains(agent.Description, "accessibility") {
		t.Errorf("Description missing 'accessibility'; got: %q", agent.Description)
	}
	if !strings.Contains(agent.Trigger, "a11y") {
		t.Errorf("Trigger missing 'a11y'; got: %q", agent.Trigger)
	}

	// Step 5: Call Install with temp dirs.
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	adapters := []AdapterInfo{
		{AgentID: model.AgentClaudeCode, SkillsDir: dir1},
		{AgentID: model.AgentOpenCode, SkillsDir: dir2},
	}

	results, err := Install(agent, adapters, "")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Step 6: Assert files written.
	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("result for %s: Success=false, err=%v", r.AgentID, r.Err)
		}
	}

	// Verify file content matches.
	skillFile1 := filepath.Join(dir1, "css-a11y-reviewer", "SKILL.md")
	data, err := os.ReadFile(skillFile1)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", skillFile1, err)
	}
	if !strings.Contains(string(data), "CSS A11y Reviewer") {
		t.Errorf("SKILL.md missing expected content; got: %s", string(data))
	}

	// Step 7: Call LoadRegistry, add entry, verify entry added.
	regPath := filepath.Join(t.TempDir(), "custom-agents.json")
	reg, err := LoadRegistry(regPath)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}

	reg.Add(RegistryEntry{
		Name:             agent.Name,
		Title:            agent.Title,
		Description:      agent.Description,
		CreatedAt:        time.Now().UTC(),
		GenerationEngine: model.AgentClaudeCode,
		InstalledAgents:  []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode},
	})

	if err := SaveRegistry(regPath, reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	// Reload and verify entry is present.
	loaded, err := LoadRegistry(regPath)
	if err != nil {
		t.Fatalf("LoadRegistry after save: %v", err)
	}

	found := loaded.FindByName("css-a11y-reviewer")
	if found == nil {
		t.Fatal("registry entry not found after add+save+load")
	}
	if found.Title != "CSS A11y Reviewer" {
		t.Errorf("registry entry Title = %q, want %q", found.Title, "CSS A11y Reviewer")
	}
	if found.GenerationEngine != model.AgentClaudeCode {
		t.Errorf("registry entry GenerationEngine = %q, want %q", found.GenerationEngine, model.AgentClaudeCode)
	}
}
