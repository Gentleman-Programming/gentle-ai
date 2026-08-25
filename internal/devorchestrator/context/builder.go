package context

import (
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/trace"
)

// Package represents the "Context Package", the universal delegation contract sent by the Orchestrator.
type Package struct {
	ExecutionID         string      `json:"execution_id" yaml:"execution_id"`
	Agent               string      `json:"agent" yaml:"agent"`
	Trace               trace.Node  `json:"trace" yaml:"trace"`
	Scope               Scope       `json:"scope" yaml:"scope"`
	Inputs              Inputs      `json:"inputs" yaml:"inputs"`
	Skills              []string    `json:"skills" yaml:"skills"`
	RepoProfile         string      `json:"repo_profile" yaml:"repo_profile"`
	ArchitectureProfile string      `json:"architecture_profile" yaml:"architecture_profile"`
	Permissions         Permissions `json:"permissions" yaml:"permissions"`
	ExpectedOutput      Output      `json:"expected_output" yaml:"expected_output"`
	// DBImpact carries the db.Impact classification ("none", "simple",
	// "high-risk") of the primary artifact, when one was evaluated. It is
	// devorchestrator-owned (not part of sddstatus.StatusV1Projection) and is
	// rendered into the agent prompt so agents -- especially
	// frontend-implementer -- are told about DB/schema impact explicitly
	// rather than only inferring it from an injected skill.
	DBImpact string `json:"db_impact,omitempty" yaml:"db_impact,omitempty"`
	// DesignRef carries the canonical Figma reference string (design.Ref.
	// Canonical()) of the primary artifact, when one was recognized. It is
	// rendered into the agent prompt so agents -- especially
	// frontend-implementer and solution-architect -- know WHICH design to
	// analyze, not only that the figma-analyzer skill was injected. Only the
	// reconstructed canonical form is ever assigned here, never raw input
	// bytes (design decision D-A).
	DesignRef string `json:"design_ref,omitempty" yaml:"design_ref,omitempty"`
}

type Scope struct {
	Repositories []string `json:"repositories" yaml:"repositories"`
	Architecture string   `json:"architecture,omitempty" yaml:"architecture,omitempty"`
}

type Inputs struct {
	Artifacts []string `json:"artifacts" yaml:"artifacts"`
}

type Permissions struct {
	Code string `json:"code" yaml:"code"`
	Git  string `json:"git" yaml:"git"`
}

type Output struct {
	Type string `json:"type" yaml:"type"`
	ID   string `json:"id" yaml:"id"`
}

// BuildRequest contains the necessary information to construct a Context Package.
type BuildRequest struct {
	ExecutionID         string
	AgentName           string
	Trace               trace.Node
	Repositories        []string
	ArchitectureID      string
	Artifacts           []string
	Skills              []string
	RepoProfile         string
	ArchitectureProfile string
	ExpectedType        string
	ExpectedID          string
	DBImpact            string
	DesignRef           string
}

// Build creates a new Context Package based on the provided request.
func Build(req BuildRequest) Package {
	return Package{
		ExecutionID: req.ExecutionID,
		Agent:       req.AgentName,
		Trace:       req.Trace,
		Scope: Scope{
			Repositories: req.Repositories,
			Architecture: req.ArchitectureID,
		},
		Inputs: Inputs{
			Artifacts: req.Artifacts,
		},
		Skills:              req.Skills,
		RepoProfile:         req.RepoProfile,
		ArchitectureProfile: req.ArchitectureProfile,
		DBImpact:            req.DBImpact,
		DesignRef:           req.DesignRef,
		Permissions: Permissions{
			Code: "read", // default to read, orchestrator overrides if execution
			Git:  "read",
		},
		ExpectedOutput: Output{
			Type: req.ExpectedType,
			ID:   req.ExpectedID,
		},
	}
}
