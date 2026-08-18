package agent

// Permissions define the access level a subagent has to the system.
// Values are derived from the canonical claude/agents/*.md tool list — never
// set manually. Use LoadRegistryFromFS to build the authoritative registry.
type Permissions struct {
	Code string
	Git  string
}

// InputConstraint defines the contextual inputs an agent is allowed to receive.
// AllowedArtifactTypes mirrors the "Reads:" section of each agent's Artifact Contract.
type InputConstraint struct {
	AllowedArtifactTypes []string
	RequiresRepoProfile  bool
	RequiresArchitecture bool
}

// OutputConstraint defines what the agent is expected to produce.
// Type mirrors the "Writes:" section of each agent's Artifact Contract.
type OutputConstraint struct {
	Type string
}

// Contract defines the strict parameters under which an agent operates.
// It is derived from the canonical internal/assets/claude/agents/<name>.md
// definition — the tools: frontmatter field is the authoritative source
// for Permissions. Do NOT maintain a parallel static map.
type Contract struct {
	Name        string
	Tools       []string // raw tools parsed from the canonical .md frontmatter
	Inputs      InputConstraint
	Output      OutputConstraint
	Permissions Permissions
}
