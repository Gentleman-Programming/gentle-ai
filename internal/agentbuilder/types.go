package agentbuilder

import (
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// GeneratedAgent holds the result of a generation run before installation.
type GeneratedAgent struct {
	Name        string
	Title       string
	Description string
	Trigger     string
	Content     string
}

// RegistryEntry is a single record persisted in the custom-agent registry.
type RegistryEntry struct {
	Name             string          `json:"name"`
	Title            string          `json:"title"`
	Description      string          `json:"description"`
	CreatedAt        time.Time       `json:"created_at"`
	GenerationEngine model.AgentID   `json:"generation_engine"`
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
