package opencode

import (
	"os"
	"path/filepath"
	"sort"
)

// DefaultSettingsPath returns the default path to the OpenCode settings file.
func DefaultSettingsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return DefaultSettingsPathForHome(home)

}

// DefaultSettingsPathForHome returns the XDG-compatible default OpenCode settings path.
func DefaultSettingsPathForHome(homeDir string) string {
	if homeDir == "" {
		return ""
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(xdg) && homeDir == os.Getenv("HOME") {
		return filepath.Join(xdg, "opencode", "opencode.json")
	}
	return filepath.Join(homeDir, ".config", "opencode", "opencode.json")
}

// ModelCost holds the per-million-token pricing.
type ModelCost struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

// ModelLimit holds context and output token limits.
type ModelLimit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

// Model represents a single model within a provider.
type Model struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Family    string     `json:"family"`
	ToolCall  bool       `json:"tool_call"`
	Reasoning bool       `json:"reasoning"`
	Cost      ModelCost  `json:"cost"`
	Limit     ModelLimit `json:"limit"`
	Variants  []string   `json:"-"`
}

// Provider represents a runtime model provider and its catalog.
type Provider struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	URL    string           `json:"url,omitempty"`
	Models map[string]Model `json:"models"`
}

// FilterModelsForSDD returns models from a provider that support tool_call (required for SDD phases).
// Results are sorted by model name.
func FilterModelsForSDD(provider Provider) []Model {
	var models []Model
	for _, m := range provider.Models {
		if m.ToolCall {
			models = append(models, m)
		}
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})

	return models
}

// EffortLevels returns the available reasoning effort levels for this model.
// Returns nil if the model has no variants (effort picker should be skipped).
func (m Model) EffortLevels() []string {
	if len(m.Variants) == 0 {
		return nil
	}
	return m.Variants
}

// SDDPhases returns the ordered list of SDD phase sub-agent names.
func SDDPhases() []string {
	return []string{
		"sdd-init",
		"sdd-explore",
		"sdd-research",
		"sdd-propose",
		"sdd-spec",
		"sdd-design",
		"sdd-tasks",
		"sdd-apply",
		"sdd-verify",
		"sdd-archive",
		"sdd-onboard",
	}
}

// JDPhases returns the ordered list of judgment-day sub-agent names.
// These are workflow-level agents (not SDD phases) used by the
// judgment-day skill for parallel adversarial review.
// They support independent model configuration for diversity of perspective.
func JDPhases() []string {
	return []string{
		"jd-judge-a",
		"jd-judge-b",
		"jd-fix-agent",
	}
}

const (
	ReviewRefuterAgent   = "review-refuter"
	ReviewValidatorAgent = "review-validator"
)

// ReviewLensPhases returns the ordered native bounded-review lens agents.
func ReviewLensPhases() []string {
	return []string{
		"review-risk",
		"review-readability",
		"review-reliability",
		"review-resilience",
	}
}

// ReviewPhases returns every agent invoked by the native review lifecycle.
func ReviewPhases() []string {
	phases := ReviewLensPhases()
	return append(phases, ReviewRefuterAgent, ReviewValidatorAgent)
}

// ConfigurableAgentPhases returns all agent names that support per-agent
// model configuration. This includes SDD, Judgment Day, and review agents.
// Used by the inject model assignment table builder and the configurable agent set
// in ReadCurrentModelAssignments. The TUI uses each role family separately
// for row layout control.
func ConfigurableAgentPhases() []string {
	phases := SDDPhases()
	phases = append(phases, JDPhases()...)
	phases = append(phases, ReviewPhases()...)
	return phases
}
