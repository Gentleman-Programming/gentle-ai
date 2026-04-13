package model

type Selection struct {
	Agents                 []AgentID
	Components             []ComponentID
	Skills                 []SkillID
	Persona                PersonaID
	Preset                 PresetID
	SDDMode                SDDModeID
	StrictTDD              bool
	ModelAssignments       map[string]ModelAssignment  // key = sub-agent name (e.g., "sdd-init")
	ClaudeModelAssignments map[string]ClaudeModelAlias // key = phase name; value = opus|sonnet|haiku
	KiroModelAssignments   map[string]ClaudeModelAlias // key = phase name; value = opus|sonnet|haiku (Kiro-only)
	Profiles               []Profile                   // named SDD profiles to generate/update during sync
}

func (s Selection) HasAgent(agent AgentID) bool {
	for _, current := range s.Agents {
		if current == agent {
			return true
		}
	}

	return false
}

func (s Selection) HasComponent(component ComponentID) bool {
	for _, current := range s.Components {
		if current == component {
			return true
		}
	}

	return false
}

// SyncOverrides holds optional overrides applied to the sync selection.
// Used when the TUI "Configure Models" flow needs to persist model assignments
// without re-running the full install pipeline.
//
// Nil fields mean "no override" — the sync uses defaults from BuildSyncSelection.
// A non-nil but empty map means "reset to defaults" (explicit clear).
type SyncOverrides struct {
	ModelAssignments       map[string]ModelAssignment  // nil = no override; empty map = reset to defaults
	ClaudeModelAssignments map[string]ClaudeModelAlias // nil = no override; empty map = reset to defaults
	KiroModelAssignments   map[string]ClaudeModelAlias // nil = no override; empty map = reset to defaults
	SDDMode                SDDModeID                   // "" = no override; when non-empty, overrides the sync's default SDD mode
	StrictTDD              *bool                       // nil = no override; non-nil = override strict TDD mode
	Profiles               []Profile                   // NEW: profile creation/updates during sync
}
