package model

type Selection struct {
	Agents                      []AgentID
	Components                  []ComponentID
	Skills                      []SkillID
	Persona                     PersonaID
	Preset                      PresetID
	SDDMode                     SDDModeID
	SDDProfileStrategy          SDDProfileStrategyID
	StrictTDD                   bool
	CodexMultiAgent             bool                             // deprecated: Codex now always writes features.multi_agent = true; retained for state/back-compat
	ModelAssignments            map[string]ModelAssignment       // key = sub-agent name (e.g., "sdd-init")
	ClaudeModelAssignments      map[string]ClaudeModelAlias      // key = phase name; value = fable|opus|sonnet|haiku
	ClaudePhaseAssignments      map[string]ClaudePhaseAssignment // key = phase name; value = Claude model+effort
	KiroModelAssignments        map[string]KiroModelAlias        // key = phase name; value = Kiro-native model alias
	CodexModelAssignments       map[string]CodexEffort           // key = phase name; value = low|medium|high|xhigh
	CodexCarrilModelAssignments map[string]string                // key = carril profile (sdd-strong|sdd-mid|sdd-cheap); value = model id
	CodexPhaseModelAssignments  map[string]string                // key = phase name; value = model id (Custom per-phase picker only)
	Profiles                    []Profile                        // named SDD profiles to generate/update during sync
	OpenCodePlugins             []OpenCodeCommunityPluginID      // optional community OpenCode TUI plugins
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
	// TargetAgents forces TUI sync to run the adapter(s) affected by the
	// override, even when persisted install state omits them. This is used by
	// model/profile configurators, where the user picked a concrete target agent.
	TargetAgents                []AgentID
	ModelAssignments            map[string]ModelAssignment       // nil = no override; empty map = reset to defaults
	ClaudeModelAssignments      map[string]ClaudeModelAlias      // nil = no override; empty map = reset to defaults
	ClaudePhaseAssignments      map[string]ClaudePhaseAssignment // nil = no override; empty map = reset to defaults
	KiroModelAssignments        map[string]KiroModelAlias        // nil = no override; empty map = reset to defaults
	CodexModelAssignments       map[string]CodexEffort           // nil = no override; empty map = reset to defaults
	CodexCarrilModelAssignments map[string]string                // nil = no override; empty map = reset to defaults
	CodexPhaseModelAssignments  map[string]string                // nil = no override (partial sync); non-nil empty = clear (preset selected); non-nil non-empty = custom per-phase assignments
	SDDMode                     SDDModeID                        // "" = no override; when non-empty, overrides the sync's default SDD mode
	SDDProfileStrategy          SDDProfileStrategyID             // "" = auto; otherwise explicit sync profile strategy
	StrictTDD                   *bool                            // nil = no override; non-nil = override strict TDD mode
	Profiles                    []Profile                        // NEW: profile creation/updates during sync
}
