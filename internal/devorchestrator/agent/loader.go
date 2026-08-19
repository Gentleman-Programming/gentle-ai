package agent

import (
	"io/fs"
	"strings"
)

// ParseToolsLine parses a raw "tools: Read, Edit, Write, ..." value into a slice of tool names.
func ParseToolsLine(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	parts := strings.Split(raw, ",")
	tools := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			tools = append(tools, t)
		}
	}
	return tools
}

// DerivePermissions inspects the tool list from an agent's frontmatter and
// derives the Code and Git permission levels using the same logic that
// gentle-ai's canonical agent definitions imply:
//
//   - "Edit" or "Write" in the tool list → Code: "write"
//   - "Bash" present together with "Edit"/"Write" → Git: "write"
//   - No "Edit" or "Write" → Code: "read"
//   - No "Bash" or no "Edit"/"Write" → Git: "read"
//   - Empty or "[]" tool list → Code: "none", Git: "none"
func DerivePermissions(tools []string) Permissions {
	if len(tools) == 0 {
		return Permissions{Code: "none", Git: "none"}
	}
	hasEdit := false
	hasBash := false
	for _, t := range tools {
		switch t {
		case "Edit", "Write":
			hasEdit = true
		case "Bash":
			hasBash = true
		}
	}

	if hasEdit {
		gitPerm := "read"
		if hasBash {
			gitPerm = "write"
		}
		return Permissions{Code: "write", Git: gitPerm}
	}
	return Permissions{Code: "read", Git: "read"}
}

// ParseAgentFrontmatter extracts the name and tools line from a canonical
// claude/agents/*.md file using the same lightweight approach as
// internal/assets/claude_agents_frontmatter_test.go — no YAML library needed.
func ParseAgentFrontmatter(content string) (name string, tools []string, ok bool) {
	if !strings.HasPrefix(content, "---\n") {
		return "", nil, false
	}
	rest := strings.TrimPrefix(content, "---\n")
	closeIdx := strings.Index(rest, "\n---")
	if closeIdx == -1 {
		return "", nil, false
	}
	block := rest[:closeIdx]

	for _, line := range strings.Split(block, "\n") {
		if strings.HasPrefix(line, "name:") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		}
		if strings.HasPrefix(line, "tools:") {
			tools = ParseToolsLine(strings.TrimSpace(strings.TrimPrefix(line, "tools:")))
		}
	}
	return name, tools, name != ""
}

// LoadRegistryFromFS builds the agent Registry by reading the canonical
// claude/agents/*.md files from the given filesystem (expected to be
// internal/assets.FS). This is the single source of truth — no static
// map to keep in sync.
//
// For agents not found in the FS (e.g. non-dev agents like jd-judge-a),
// only the 12 devAgentCanonicalNames are loaded; others are silently skipped.
func LoadRegistryFromFS(agentsFS fs.FS, agentsDir string) (map[string]Contract, error) {
	entries, err := fs.ReadDir(agentsFS, agentsDir)
	if err != nil {
		return nil, err
	}

	registry := make(map[string]Contract, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		data, err := fs.ReadFile(agentsFS, agentsDir+"/"+e.Name())
		if err != nil {
			return nil, err
		}

		agentName, tools, ok := ParseAgentFrontmatter(string(data))
		if !ok || agentName == "" {
			continue
		}

		// Only register dev-agent roles (skip jd-*, review-*, sdd-* agents)
		if !isDevAgentRole(agentName) {
			continue
		}

		perms := DerivePermissions(tools)
		registry[agentName] = Contract{
			Name:  agentName,
			Tools: tools,
			Inputs: InputConstraint{
				AllowedArtifactTypes: defaultInputsForAgent(agentName),
				RequiresRepoProfile:  requiresRepoProfile(agentName),
				RequiresArchitecture: requiresArchitecture(agentName),
			},
			Output:      OutputConstraint{Type: defaultOutputTypeForAgent(agentName)},
			Permissions: perms,
		}
	}
	return registry, nil
}

// isDevAgentRole returns true for the 12 core dev-agent roles that the
// devorchestrator dispatches. Mirrors devAgentCanonicalNames in
// internal/assets/dev_agent_parity_test.go.
func isDevAgentRole(name string) bool {
	switch name {
	case "backend-implementer", "database-specialist", "dev-designer",
		"dev-explorer", "dev-orchestrator", "dev-proposer",
		"dev-specifier", "dev-task-planner", "dev-verifier",
		"frontend-implementer", "project-bootstrap", "solution-architect":
		return true
	}
	return false
}

// defaultInputsForAgent returns the canonical allowed artifact types for each agent.
// This reflects the "Artifact Contract" section of each agent's canonical .md.
func defaultInputsForAgent(name string) []string {
	switch name {
	case "dev-explorer", "dev-proposer":
		return []string{"requirement", "bug", "feature", "exploration"}
	case "dev-specifier":
		return []string{"requirement", "exploration", "proposal"}
	case "dev-designer":
		return []string{"requirement", "exploration", "proposal", "spec"}
	case "dev-task-planner":
		return []string{"spec", "design", "blueprint", "db-assessment"}
	case "solution-architect":
		return []string{"requirement", "exploration"}
	case "database-specialist":
		return []string{"spec", "design"}
	case "backend-implementer", "frontend-implementer":
		return []string{"task", "spec", "design-fragment"}
	case "dev-verifier":
		return []string{"spec", "design", "task", "diff", "test-results"}
	case "project-bootstrap":
		return []string{"blueprint"}
	default:
		return nil
	}
}

func defaultOutputTypeForAgent(name string) string {
	switch name {
	case "dev-explorer":
		return "EXPLORATION"
	case "dev-proposer":
		return "PROPOSAL"
	case "dev-specifier":
		return "SPEC"
	case "dev-designer":
		return "DESIGN"
	case "dev-task-planner":
		return "TASK"
	case "solution-architect":
		return "BLUEPRINT"
	case "database-specialist":
		return "DB-ASSESSMENT"
	case "backend-implementer", "frontend-implementer":
		return "COMMIT"
	case "dev-verifier":
		return "VERIFY-REPORT"
	case "project-bootstrap":
		return "REPO"
	default:
		return ""
	}
}

func requiresRepoProfile(name string) bool {
	switch name {
	case "dev-explorer", "dev-designer", "backend-implementer", "frontend-implementer":
		return true
	}
	return false
}

func requiresArchitecture(name string) bool {
	switch name {
	case "dev-designer", "solution-architect":
		return true
	}
	return false
}
