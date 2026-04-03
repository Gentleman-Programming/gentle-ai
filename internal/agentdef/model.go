// Package agentdef handles loading and parsing of project-scoped agent
// definitions from .claude/agents/*.md files with YAML frontmatter.
package agentdef

// AgentDef is a project-scoped agent definition loaded from an AGENT.md file.
// The frontmatter declares the agent's role, model, tool constraints, and hooks.
type AgentDef struct {
	// Name is the agent identifier used in swarms and slash commands.
	Name string
	// Description is a one-line summary shown in TUI and logs.
	Description string
	// Model overrides the session default for this agent (opus, sonnet, haiku).
	Model string
	// AllowedTools lists tools this agent may use. Empty = project defaults.
	AllowedTools []string
	// Context controls how the agent is invoked: "inline" or "fork".
	// "fork" maps to run_in_background=true in Agent tool calls.
	Context string
	// Extends is the name of a parent agent to inherit from (optional).
	Extends string
	// Hooks maps event names to script paths for this agent.
	Hooks map[string]string
	// Rules holds arbitrary behavioral constraints (e.g. max_file_edits: 0).
	Rules map[string]any
	// Body is the full agent instruction text (everything after the frontmatter).
	Body string
}
