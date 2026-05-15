package model

// EngramDataDirOp is the operation the user chose for their Engram data directory.
// String-based enum — matches the AgentID / ComponentID convention.
type EngramDataDirOp string

const (
	EngramDataDirOpNone   EngramDataDirOp = ""
	EngramDataDirOpKeep   EngramDataDirOp = "keep"
	EngramDataDirOpCopy   EngramDataDirOp = "copy"
	EngramDataDirOpMove   EngramDataDirOp = "move"
	EngramDataDirOpSet    EngramDataDirOp = "set"
	EngramDataDirOpDelete EngramDataDirOp = "delete"
	EngramDataDirOpFresh  EngramDataDirOp = "fresh"
)

type Selection struct {
	Agents                 []AgentID
	Components             []ComponentID
	Skills                 []SkillID
	Persona                PersonaID
	Preset                 PresetID
	SDDMode                SDDModeID
	SDDProfileStrategy     SDDProfileStrategyID
	StrictTDD              bool
	ModelAssignments       map[string]ModelAssignment  // key = sub-agent name (e.g., "sdd-init")
	ClaudeModelAssignments map[string]ClaudeModelAlias // key = phase name; value = opus|sonnet|haiku
	KiroModelAssignments   map[string]ClaudeModelAlias // key = phase name; value = opus|sonnet|haiku (Kiro-only)
	Profiles               []Profile                   // named SDD profiles to generate/update during sync
	OpenCodePlugins        []OpenCodeCommunityPluginID // optional community OpenCode TUI plugins
	EngramDataDir          string                      // custom Engram data directory path; "" means default ~/.engram
	EngramDataDirOp        EngramDataDirOp             // operation chosen at install time; "" means no change
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
	SDDProfileStrategy     SDDProfileStrategyID        // "" = auto; otherwise explicit sync profile strategy
	StrictTDD              *bool                       // nil = no override; non-nil = override strict TDD mode
	Profiles               []Profile                   // NEW: profile creation/updates during sync
}
