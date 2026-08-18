package context

import (
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/trace"
)

// Package represents the "Context Package", the universal delegation contract sent by the Orchestrator.
type Package struct {
	ExecutionID    string      `json:"execution_id" yaml:"execution_id"`
	Agent          string      `json:"agent" yaml:"agent"`
	Trace          trace.Node  `json:"trace" yaml:"trace"`
	Scope          Scope       `json:"scope" yaml:"scope"`
	Inputs         Inputs      `json:"inputs" yaml:"inputs"`
	Skills         []string    `json:"skills" yaml:"skills"`
	RepoProfile    string      `json:"repo_profile" yaml:"repo_profile"`
	Permissions    Permissions `json:"permissions" yaml:"permissions"`
	ExpectedOutput Output      `json:"expected_output" yaml:"expected_output"`
}

type Scope struct {
	Repositories []string `json:"repositories" yaml:"repositories"`
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
	ExecutionID  string
	AgentName    string
	Trace        trace.Node
	Repositories []string
	Artifacts    []string
	Skills       []string
	RepoProfile  string
	ExpectedType string
	ExpectedID   string
}

// Build creates a new Context Package based on the provided request.
func Build(req BuildRequest) Package {
	return Package{
		ExecutionID: req.ExecutionID,
		Agent:       req.AgentName,
		Trace:       req.Trace,
		Scope: Scope{
			Repositories: req.Repositories,
		},
		Inputs: Inputs{
			Artifacts: req.Artifacts,
		},
		Skills:      req.Skills,
		RepoProfile: req.RepoProfile,
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
