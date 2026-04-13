package agentbuilder

import (
	"strings"
	"testing"
)

// validSKILL is a complete, parseable SKILL.md fixture.
const validSKILL = `# My Cool Agent

## Description
This agent helps you do cool things.

## Trigger
When the user says "do cool things" or asks for help.

## Instructions
1. Step one: listen carefully.
2. Step two: act precisely.

## Rules
- Never break things.
- Always ask before deleting.

## Examples
User: "Do cool things"
Agent: *does cool things*
`

func TestParse_ValidFullSKILL(t *testing.T) {
	agent, err := Parse(validSKILL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent.Title != "My Cool Agent" {
		t.Errorf("Title = %q, want %q", agent.Title, "My Cool Agent")
	}
	if agent.Name != "my-cool-agent" {
		t.Errorf("Name = %q, want %q", agent.Name, "my-cool-agent")
	}
	if !strings.Contains(agent.Description, "cool things") {
		t.Errorf("Description missing expected text: %q", agent.Description)
	}
	if !strings.Contains(agent.Trigger, "do cool things") {
		t.Errorf("Trigger missing expected text: %q", agent.Trigger)
	}
	if agent.Content == "" {
		t.Errorf("Content should not be empty")
	}
}

func TestParse_MissingTriggerSection(t *testing.T) {
	input := `# Agent Without Trigger

## Description
Something useful.

## Instructions
Do the thing.
`
	_, err := Parse(input)
	if err == nil {
		t.Fatal("expected error for missing Trigger section, got nil")
	}
	if !strings.Contains(err.Error(), "Trigger") {
		t.Errorf("error should mention 'Trigger', got: %v", err)
	}
}

func TestParse_MissingInstructionsSection(t *testing.T) {
	input := `# Agent Without Instructions

## Description
Something useful.

## Trigger
When user says "do it".
`
	_, err := Parse(input)
	if err == nil {
		t.Fatal("expected error for missing Instructions section, got nil")
	}
	if !strings.Contains(err.Error(), "Instructions") {
		t.Errorf("error should mention 'Instructions', got: %v", err)
	}
}

func TestParse_CodeFenceStripping(t *testing.T) {
	input := "```markdown\n" + validSKILL + "\n```"

	agent, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error after stripping code fences: %v", err)
	}
	if agent.Title != "My Cool Agent" {
		t.Errorf("Title = %q, want %q after fence stripping", agent.Title, "My Cool Agent")
	}
}

func TestParse_CodeFenceStripping_Generic(t *testing.T) {
	input := "```\n" + validSKILL + "```"

	agent, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error after stripping generic code fences: %v", err)
	}
	if agent.Title != "My Cool Agent" {
		t.Errorf("Title = %q, want %q", agent.Title, "My Cool Agent")
	}
}

func TestParse_KebabCaseNameGeneration(t *testing.T) {
	tests := []struct {
		title    string
		wantName string
	}{
		{"A11y Reviewer", "a11y-reviewer"},
		{"My Custom Agent", "my-custom-agent"},
		{"CSS   Validator", "css-validator"},
		{"API Doc Generator!", "api-doc-generator"},
		{"  Leading Spaces  ", "leading-spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := titleToName(tt.title)
			if got != tt.wantName {
				t.Errorf("titleToName(%q) = %q, want %q", tt.title, got, tt.wantName)
			}
		})
	}
}

func TestParse_MultipleH1HeadingsUsesFirst(t *testing.T) {
	input := `# First Title

## Description
Desc text here.

## Trigger
When triggered.

## Instructions
Do this.

# Second Title should be ignored
`
	agent, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent.Title != "First Title" {
		t.Errorf("Title = %q, want first H1 = %q", agent.Title, "First Title")
	}
}

func TestParse_ExtraSectionsPreservedInContent(t *testing.T) {
	input := `# Agent With Extra Sections

## Description
Does useful things.

## Trigger
When user needs help.

## Instructions
Follow these steps carefully.

## Rules
- Do not destroy.

## Examples
Example usage here.
`
	agent, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Extra sections preserved in Content
	if !strings.Contains(agent.Content, "## Rules") {
		t.Errorf("Rules section missing from Content: %q", agent.Content)
	}
	if !strings.Contains(agent.Content, "## Examples") {
		t.Errorf("Examples section missing from Content: %q", agent.Content)
	}
}

func TestParse_EmptyInput(t *testing.T) {
	_, err := Parse("")
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}

func TestParse_WhitespaceOnlyInput(t *testing.T) {
	_, err := Parse("   \n\t\n  ")
	if err == nil {
		t.Fatal("expected error for whitespace-only input, got nil")
	}
}

func TestParse_MissingTitle(t *testing.T) {
	input := `## Description
Something.

## Trigger
When triggered.

## Instructions
Do it.
`
	_, err := Parse(input)
	if err == nil {
		t.Fatal("expected error for missing H1 title, got nil")
	}
}
