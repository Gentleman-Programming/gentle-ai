package agent

// Permissions define the access level a subagent has to the system.
type Permissions struct {
	Code string
	Git  string
}

// InputConstraint defines the contextual inputs an agent is allowed to receive.
type InputConstraint struct {
	AllowedArtifactTypes []string
	RequiresRepoProfile  bool
	RequiresArchitecture bool
}

// OutputConstraint defines what the agent is expected to produce.
type OutputConstraint struct {
	Type string
}

// Contract defines the strict parameters under which an agent operates.
type Contract struct {
	Name        string
	Inputs      InputConstraint
	Output      OutputConstraint
	Permissions Permissions
}

// Registry holds the official Agent Technical Contracts.
// Any agent not in this registry should be rejected by the orchestrator in strict mode.
var Registry = map[string]Contract{
	"dev-explorer": {
		Name: "dev-explorer",
		Inputs: InputConstraint{
			AllowedArtifactTypes: []string{"requirement", "bug", "feature"},
			RequiresRepoProfile:  true,
		},
		Output: OutputConstraint{Type: "EXPLORATION"},
		Permissions: Permissions{
			Code: "read",
			Git:  "read",
		},
	},
	"dev-proposer": {
		Name: "dev-proposer",
		Inputs: InputConstraint{
			AllowedArtifactTypes: []string{"requirement", "exploration"},
		},
		Output: OutputConstraint{Type: "PROPOSAL"},
		Permissions: Permissions{
			Code: "none", // shouldn't read raw code, relies on exploration
			Git:  "none",
		},
	},
	"dev-specifier": {
		Name: "dev-specifier",
		Inputs: InputConstraint{
			AllowedArtifactTypes: []string{"requirement", "exploration", "proposal"},
		},
		Output: OutputConstraint{Type: "SPEC"},
		Permissions: Permissions{
			Code: "none",
			Git:  "none",
		},
	},
	"dev-designer": {
		Name: "dev-designer",
		Inputs: InputConstraint{
			AllowedArtifactTypes: []string{"requirement", "exploration", "proposal", "spec"},
			RequiresRepoProfile:  true,
			RequiresArchitecture: true,
		},
		Output: OutputConstraint{Type: "DESIGN"},
		Permissions: Permissions{
			Code: "read",
			Git:  "read",
		},
	},
	"dev-task-planner": {
		Name: "dev-task-planner",
		Inputs: InputConstraint{
			AllowedArtifactTypes: []string{"spec", "design", "blueprint", "db-assessment"},
		},
		Output: OutputConstraint{Type: "TASK"},
		Permissions: Permissions{
			Code: "none",
			Git:  "none",
		},
	},
	"solution-architect": {
		Name: "solution-architect",
		Inputs: InputConstraint{
			AllowedArtifactTypes: []string{"requirement", "exploration"},
			RequiresArchitecture: true,
		},
		Output: OutputConstraint{Type: "BLUEPRINT"},
		Permissions: Permissions{
			Code: "read",
			Git:  "read",
		},
	},
	"database-specialist": {
		Name: "database-specialist",
		Inputs: InputConstraint{
			AllowedArtifactTypes: []string{"spec", "design"},
		},
		Output: OutputConstraint{Type: "DB-ASSESSMENT"},
		Permissions: Permissions{
			Code: "write", // Might write migrations
			Git:  "write",
		},
	},
	"backend-implementer": {
		Name: "backend-implementer",
		Inputs: InputConstraint{
			AllowedArtifactTypes: []string{"task", "spec", "design-fragment"},
			RequiresRepoProfile:  true,
		},
		Output: OutputConstraint{Type: "COMMIT"},
		Permissions: Permissions{
			Code: "write",
			Git:  "write",
		},
	},
	"frontend-implementer": {
		Name: "frontend-implementer",
		Inputs: InputConstraint{
			AllowedArtifactTypes: []string{"task", "spec", "design-fragment"},
			RequiresRepoProfile:  true,
		},
		Output: OutputConstraint{Type: "COMMIT"},
		Permissions: Permissions{
			Code: "write",
			Git:  "write",
		},
	},
	"dev-verifier": {
		Name: "dev-verifier",
		Inputs: InputConstraint{
			AllowedArtifactTypes: []string{"spec", "design", "task", "diff", "test-results"},
		},
		Output: OutputConstraint{Type: "VERIFY-REPORT"},
		Permissions: Permissions{
			Code: "read", // Analyzes diffs and tests but doesn't write code
			Git:  "read",
		},
	},
}
