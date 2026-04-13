package agentbuilder

import (
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// SDDIntegrationMode defines how a generated agent integrates with SDD phases.
type SDDIntegrationMode string

const (
	SDDStandalone   SDDIntegrationMode = "standalone"
	SDDNewPhase     SDDIntegrationMode = "new-phase"
	SDDPhaseSupport SDDIntegrationMode = "phase-support"
)

// SDDIntegration describes how the agent connects to the SDD workflow.
type SDDIntegration struct {
	Mode        SDDIntegrationMode `json:"mode"`
	TargetPhase string             `json:"target_phase"`
	PhaseName   string             `json:"phase_name,omitempty"`
}

// GeneratedAgent holds the result of a generation run before installation.
type GeneratedAgent struct {
	Name        string
	Title       string
	Description string
	Trigger     string
	Content     string
	SDDConfig   *SDDIntegration
}

// RegistryEntry is a single record persisted in the custom-agent registry.
type RegistryEntry struct {
	Name             string          `json:"name"`
	Title            string          `json:"title"`
	Description      string          `json:"description"`
	CreatedAt        time.Time       `json:"created_at"`
	GenerationEngine model.AgentID   `json:"generation_engine"`
	SDDIntegration   *SDDIntegration `json:"sdd_integration,omitempty"`
	InstalledAgents  []model.AgentID `json:"installed_agents"`
}

// Registry is the top-level structure persisted to disk.
type Registry struct {
	Version int             `json:"version"`
	Agents  []RegistryEntry `json:"agents"`
}

// InstallResult captures the outcome of writing one agent's SKILL.md file.
type InstallResult struct {
	AgentID model.AgentID
	Path    string
	Success bool
	Err     error
}
