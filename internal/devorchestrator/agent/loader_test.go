package agent_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/agent"
)

func TestLoadRegistryFromFS_LoadsDevAgents(t *testing.T) {
	registry, err := agent.LoadRegistryFromFS(assets.FS, "claude/agents")
	if err != nil {
		t.Fatalf("LoadRegistryFromFS() error = %v", err)
	}

	expectedAgents := []string{
		"backend-implementer",
		"database-specialist",
		"dev-designer",
		"dev-explorer",
		"dev-proposer",
		"dev-specifier",
		"dev-task-planner",
		"dev-verifier",
		"frontend-implementer",
		"solution-architect",
		"project-bootstrap",
	}

	for _, name := range expectedAgents {
		c, ok := registry[name]
		if !ok {
			t.Errorf("agent %q not found in registry", name)
			continue
		}
		if c.Name != name {
			t.Errorf("agent %q: Name = %q, want %q", name, c.Name, name)
		}
		if len(c.Tools) == 0 {
			t.Errorf("agent %q: tools list is empty — frontmatter parse may have failed", name)
		}
		if c.Permissions.Code == "" {
			t.Errorf("agent %q: Code permission is empty", name)
		}
	}
}

func TestDerivePermissions_ExplorerIsReadOnly(t *testing.T) {
	// dev-explorer: tools: Read, Grep, Glob, WebFetch, WebSearch, mcp__...
	// No Edit, Write, or Bash → Code: read, Git: read
	registry, err := agent.LoadRegistryFromFS(assets.FS, "claude/agents")
	if err != nil {
		t.Fatalf("LoadRegistryFromFS() error = %v", err)
	}

	explorer, ok := registry["dev-explorer"]
	if !ok {
		t.Fatal("dev-explorer not found in registry")
	}
	if explorer.Permissions.Code != "read" {
		t.Errorf("dev-explorer Code = %q, want read", explorer.Permissions.Code)
	}
	if explorer.Permissions.Git != "read" {
		t.Errorf("dev-explorer Git = %q, want read", explorer.Permissions.Git)
	}
}

func TestDerivePermissions_ImplementerHasWriteAccess(t *testing.T) {
	// backend-implementer: tools: Read, Edit, Write, Glob, Grep, Bash, mcp__...
	// Has Edit + Bash → Code: write, Git: write
	registry, err := agent.LoadRegistryFromFS(assets.FS, "claude/agents")
	if err != nil {
		t.Fatalf("LoadRegistryFromFS() error = %v", err)
	}

	impl, ok := registry["backend-implementer"]
	if !ok {
		t.Fatal("backend-implementer not found in registry")
	}
	if impl.Permissions.Code != "write" {
		t.Errorf("backend-implementer Code = %q, want write", impl.Permissions.Code)
	}
	if impl.Permissions.Git != "write" {
		t.Errorf("backend-implementer Git = %q, want write", impl.Permissions.Git)
	}
}

func TestDerivePermissions_VerifierReadCodeNoBashWrite(t *testing.T) {
	// dev-verifier: tools: Read, Grep, Glob, Bash, mcp__...
	// Has Bash but NOT Edit/Write → Code: read, Git: read
	registry, err := agent.LoadRegistryFromFS(assets.FS, "claude/agents")
	if err != nil {
		t.Fatalf("LoadRegistryFromFS() error = %v", err)
	}

	verifier, ok := registry["dev-verifier"]
	if !ok {
		t.Fatal("dev-verifier not found in registry")
	}
	if verifier.Permissions.Code != "read" {
		t.Errorf("dev-verifier Code = %q, want read (Bash alone doesn't grant write)", verifier.Permissions.Code)
	}
}

func TestParseAgentFrontmatter_RoundTrip(t *testing.T) {
	content := `---
name: test-agent
description: A test agent for unit testing
model: {{CLAUDE_MODEL}}
tools: Read, Edit, Write, Bash, mcp__engram__mem_save
---

Body content here.
`
	name, tools, ok := agent.ParseAgentFrontmatter(content)
	if !ok {
		t.Fatal("ParseAgentFrontmatter returned ok=false")
	}
	if name != "test-agent" {
		t.Errorf("name = %q, want test-agent", name)
	}
	if len(tools) != 5 {
		t.Errorf("tools len = %d, want 5: %v", len(tools), tools)
	}

	perms := agent.DerivePermissions(tools)
	if perms.Code != "write" {
		t.Errorf("Code = %q, want write", perms.Code)
	}
	if perms.Git != "write" {
		t.Errorf("Git = %q, want write", perms.Git)
	}
}

func TestLoadRegistryFromFS_NonDevAgentsExcluded(t *testing.T) {
	registry, err := agent.LoadRegistryFromFS(assets.FS, "claude/agents")
	if err != nil {
		t.Fatalf("LoadRegistryFromFS() error = %v", err)
	}

	// These are not dev-agent roles and should NOT be in the registry
	excluded := []string{"jd-judge-a", "jd-judge-b", "jd-fix-agent",
		"review-readability", "review-reliability", "review-resilience", "review-risk",
		"sdd-apply", "sdd-explore", "sdd-verify", "sdd-archive"}

	for _, name := range excluded {
		if _, ok := registry[name]; ok {
			t.Errorf("non-dev agent %q should not be in registry but was found", name)
		}
	}
}

func TestDerivePermissions_EmptyTools(t *testing.T) {
	perms := agent.DerivePermissions(nil)
	if perms.Code != "none" || perms.Git != "none" {
		t.Errorf("empty tools: got Code=%q Git=%q, want none/none", perms.Code, perms.Git)
	}
}

func TestParseToolsLine(t *testing.T) {
	cases := []struct {
		raw   string
		count int
	}{
		{"Read, Edit, Write, Bash", 4},
		{"Read, Grep, Glob, WebFetch, WebSearch", 5},
		{"[]", 0},
		{"", 0},
	}
	for _, tc := range cases {
		got := agent.ParseToolsLine(tc.raw)
		if len(got) != tc.count {
			t.Errorf("ParseToolsLine(%q): got %d tools, want %d: %v", tc.raw, len(got), tc.count, got)
		}
		for _, tool := range got {
			if strings.TrimSpace(tool) != tool {
				t.Errorf("tool %q has unexpected whitespace", tool)
			}
		}
	}
}
