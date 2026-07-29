package model

// ModelAssignment represents a provider/model pair assigned to an SDD phase sub-agent.
// Effort specifies the reasoning effort level for models that support it.
// An empty string means "use provider default" and is backward-compatible with
// existing persisted state.
type ModelAssignment struct {
	ProviderID string // e.g., "anthropic"
	ModelID    string // e.g., "claude-sonnet-4-20250514"
	Effort     string // "" = provider default; "low" | "medium" | "high"
}

// SplitModelSpec splits a provider-qualified model spec at its first separator.
func SplitModelSpec(spec string) (providerID, modelID string, ok bool) {
	for i := 0; i < len(spec); i++ {
		if spec[i] != '/' && spec[i] != ':' {
			continue
		}
		if i == 0 || i == len(spec)-1 {
			return "", "", false
		}
		return spec[:i], spec[i+1:], true
	}
	return "", "", false
}

// FullID returns the provider-qualified model identifier (e.g., "anthropic/claude-sonnet-4-20250514").
func (m ModelAssignment) FullID() string {
	return m.ProviderID + "/" + m.ModelID
}
