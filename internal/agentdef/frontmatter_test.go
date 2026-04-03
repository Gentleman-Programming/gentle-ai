package agentdef_test

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/agentdef"
)

const architectMD = `---
name: "architect"
description: "Validates decisions against stack_config.json"
model: "opus"
context: "inline"
allowedTools:
  - Read
  - Grep
  - Bash
hooks:
  PreToolUse: "validators/check_stack.sh"
rules:
  require_plan_mode: true
  max_file_edits: 0
---

You are the Architect. Read MANDATORY_CHECKS.md before any action.
`

const noFrontmatterMD = `
You are an agent without frontmatter.
`

const missingNameMD = `---
description: "Missing name field"
model: "sonnet"
---

Body here.
`

func TestParseFrontmatter_FullDefinition(t *testing.T) {
	def, err := agentdef.ParseFrontmatter([]byte(architectMD))
	if err != nil {
		t.Fatalf("ParseFrontmatter() error: %v", err)
	}

	if def.Name != "architect" {
		t.Errorf("Name: got %q, want %q", def.Name, "architect")
	}
	if def.Model != "opus" {
		t.Errorf("Model: got %q, want %q", def.Model, "opus")
	}
	if def.Context != "inline" {
		t.Errorf("Context: got %q, want %q", def.Context, "inline")
	}
	if len(def.AllowedTools) != 3 {
		t.Errorf("AllowedTools: got %d, want 3", len(def.AllowedTools))
	}
	if def.AllowedTools[0] != "Read" {
		t.Errorf("AllowedTools[0]: got %q, want %q", def.AllowedTools[0], "Read")
	}
	if def.Hooks["PreToolUse"] != "validators/check_stack.sh" {
		t.Errorf("Hooks[PreToolUse]: got %q", def.Hooks["PreToolUse"])
	}
	if def.Rules["max_file_edits"] != 0 {
		t.Errorf("Rules[max_file_edits]: got %v, want 0", def.Rules["max_file_edits"])
	}
	if def.Rules["require_plan_mode"] != true {
		t.Errorf("Rules[require_plan_mode]: got %v, want true", def.Rules["require_plan_mode"])
	}
	if def.Body == "" {
		t.Error("Body should not be empty")
	}
}

func TestParseFrontmatter_NoFrontmatter_BodyOnly(t *testing.T) {
	// No frontmatter → name is empty → should error
	_, err := agentdef.ParseFrontmatter([]byte(noFrontmatterMD))
	// Body-only (no ---) means name is empty, so we expect a parse error or empty name
	// In our implementation, if there's no ---, the whole content is body with no fields set
	// This results in a missing name error.
	if err == nil {
		t.Error("expected error for missing name, got nil")
	}
}

func TestParseFrontmatter_MissingName_ReturnsError(t *testing.T) {
	_, err := agentdef.ParseFrontmatter([]byte(missingNameMD))
	if err == nil {
		t.Error("expected error for missing name field, got nil")
	}
}

func TestResolve_InheritsParentFields(t *testing.T) {
	parent := agentdef.AgentDef{
		Name:         "base-agent",
		Model:        "sonnet",
		AllowedTools: []string{"Read", "Grep"},
		Hooks:        map[string]string{"PreToolUse": "validators/base.sh"},
	}
	child := agentdef.AgentDef{
		Name:    "specialist",
		Model:   "", // empty → inherit from parent
		Extends: "base-agent",
		Hooks:   map[string]string{"PostToolUse": "validators/post.sh"},
	}

	all := []agentdef.AgentDef{parent, child}
	resolved := agentdef.Resolve(child, all)

	if resolved.Model != "sonnet" {
		t.Errorf("Model should be inherited from parent: got %q", resolved.Model)
	}
	if len(resolved.AllowedTools) != 2 {
		t.Errorf("AllowedTools should be inherited: got %d", len(resolved.AllowedTools))
	}
	// Child hooks merged with parent hooks
	if resolved.Hooks["PreToolUse"] != "validators/base.sh" {
		t.Errorf("PreToolUse hook should come from parent")
	}
	if resolved.Hooks["PostToolUse"] != "validators/post.sh" {
		t.Errorf("PostToolUse hook should come from child")
	}
}

func TestFindByName(t *testing.T) {
	defs := []agentdef.AgentDef{
		{Name: "architect"},
		{Name: "backend-developer"},
	}

	found, ok := agentdef.FindByName(defs, "architect")
	if !ok {
		t.Fatal("FindByName should find architect")
	}
	if found.Name != "architect" {
		t.Errorf("unexpected name: %s", found.Name)
	}

	_, ok = agentdef.FindByName(defs, "nonexistent")
	if ok {
		t.Error("FindByName should return false for nonexistent agent")
	}
}
