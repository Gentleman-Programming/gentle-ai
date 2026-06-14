package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const stateDir = ".gentle-ai"
const stateFile = "state.json"

// ModelAssignmentState is the JSON-serialisable form of a provider+model pair
// used by OpenCode-style model assignments. It mirrors model.ModelAssignment
// but lives in the state package to avoid an import cycle.
// Effort is the reasoning effort level ("" | "low" | "medium" | "high");
// omitempty ensures backward-compatibility with existing state files.
type ModelAssignmentState struct {
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
	Effort     string `json:"effort,omitempty"`
}

// ClaudePhaseAssignmentState is the JSON-serialisable form of a Claude
// subagent model+effort assignment. Empty Effort means Claude Code default.
type ClaudePhaseAssignmentState struct {
	Model  string `json:"model"`
	Effort string `json:"effort,omitempty"`
}

// InstallState holds the persisted user selections from the last install run.
type InstallState struct {
	InstalledAgents []string `json:"installed_agents"`

	// ClaudeModelAssignments maps SDD phase names (e.g. "sdd-explore") to a
	// Claude model alias ("fable", "opus", "sonnet", "haiku"). Persisted so that
	// `gentle-ai sync` preserves the user's model choices instead of falling
	// back to the "balanced" preset every time.
	ClaudeModelAssignments map[string]string `json:"claude_model_assignments,omitempty"`

	// ClaudePhaseAssignments maps SDD phase names to Claude model+effort assignments.
	// It supersedes ClaudeModelAssignments while preserving backward compatibility.
	ClaudePhaseAssignments map[string]ClaudePhaseAssignmentState `json:"claude_phase_assignments,omitempty"`

	// KiroModelAssignments maps SDD phase names to a Kiro-native model alias.
	// Values like "opus", "sonnet", and "haiku" remain valid for state files
	// written before Kiro had its own picker options.
	KiroModelAssignments map[string]string `json:"kiro_model_assignments,omitempty"`

	// CodexModelAssignments maps SDD phase names to a Codex reasoning_effort value
	// (low|medium|high|xhigh). Persisted so that `gentle-ai sync` preserves the
	// user's per-phase effort preset instead of falling back to Recommended.
	CodexModelAssignments map[string]string `json:"codexModelAssignments,omitempty"`

	// CodexCarrilModelAssignments maps the three carril profile names
	// (sdd-strong|sdd-mid|sdd-cheap) to OpenAI subscription model IDs
	// (e.g. "gpt-5.5", "gpt-5.4-mini"). Persisted so that `gentle-ai sync`
	// regenerates profile files with the user's chosen model per tier.
	// Absent/empty = resolve to DefaultCarrilModels at runtime (backward-compat).
	CodexCarrilModelAssignments map[string]string `json:"codexCarrilModelAssignments,omitempty"`

	// CodexPhaseModelAssignments maps each of the 13 SDD phase names to the
	// model id the user assigned in the Custom per-phase picker (e.g. "gpt-5.5").
	// When non-nil, overrides the carril-level model selection for that phase.
	// Absent/nil = not using custom per-phase assignments (preset/carril behavior
	// unchanged for backward-compatibility).
	CodexPhaseModelAssignments map[string]string `json:"codexPhaseModelAssignments,omitempty"`

	// ModelAssignments maps sub-agent names to provider/model pairs (OpenCode).
	ModelAssignments map[string]ModelAssignmentState `json:"model_assignments,omitempty"`

	// Persona records the persona the user installed ("gentleman", "neutral",
	// "custom"). Persisted so that `gentle-ai sync` regenerates the same persona
	// the user originally chose instead of defaulting to Gentleman every time.
	// Empty for state files written before persona persistence was added —
	// callers fall back to PersonaGentleman in that case.
	Persona string `json:"persona,omitempty"`

	// LastUpdateCheck records the last time a successful remote update check was
	// performed. Used by the cooldown gate (UpdateCheckTTL = 6h) to avoid
	// hitting the GitHub API on every launch. Nil = never checked, so the
	// check will always run on first launch (safe back-compat for existing
	// state files that lack the field entirely).
	LastUpdateCheck *time.Time `json:"last_update_check,omitempty"`

	// PendingSync is set to true when a gentle-ai self-upgrade succeeded and
	// the process is about to exit (restart required). The next launch reads
	// this flag and runs sync automatically before entering the normal flow,
	// then clears the flag on success. On sync failure the flag is left set
	// so the following launch retries idempotently.
	// False (zero value) = no deferred sync pending. Omitted from JSON when
	// false for backward-compatibility with existing state files.
	PendingSync bool `json:"pending_sync,omitempty"`
}

// Path returns the absolute path to the state file for the given home directory.
func Path(homeDir string) string {
	return filepath.Join(homeDir, stateDir, stateFile)
}

// Read reads and unmarshals the state file from the given home directory.
// Returns an error if the file does not exist or cannot be decoded.
func Read(homeDir string) (InstallState, error) {
	data, err := os.ReadFile(Path(homeDir))
	if err != nil {
		return InstallState{}, err
	}
	var s InstallState
	if err := json.Unmarshal(data, &s); err != nil {
		return InstallState{}, err
	}
	return s, nil
}

// MergeAgents returns a new InstallState that combines existing with the
// provided newAgents. The new agents are appended to existing.InstalledAgents
// with deduplication. All other fields (ModelAssignments,
// ClaudeModelAssignments, KiroModelAssignments, Persona) are taken from
// existing and are never overwritten.
//
// This is the correct operation for an incremental `--agent X` install: the
// caller loads the persisted state, calls MergeAgents, and writes the result
// back. A full TUI install should use Write directly so that the TUI selection
// is the source of truth.
func MergeAgents(existing InstallState, newAgents []string) InstallState {
	seen := make(map[string]struct{}, len(existing.InstalledAgents))
	merged := make([]string, 0, len(existing.InstalledAgents)+len(newAgents))

	for _, a := range existing.InstalledAgents {
		if _, ok := seen[a]; !ok {
			seen[a] = struct{}{}
			merged = append(merged, a)
		}
	}
	for _, a := range newAgents {
		if _, ok := seen[a]; !ok {
			seen[a] = struct{}{}
			merged = append(merged, a)
		}
	}

	return InstallState{
		InstalledAgents:             merged,
		ModelAssignments:            existing.ModelAssignments,
		ClaudeModelAssignments:      existing.ClaudeModelAssignments,
		ClaudePhaseAssignments:      existing.ClaudePhaseAssignments,
		KiroModelAssignments:        existing.KiroModelAssignments,
		CodexModelAssignments:       existing.CodexModelAssignments,
		CodexCarrilModelAssignments: existing.CodexCarrilModelAssignments,
		CodexPhaseModelAssignments:  existing.CodexPhaseModelAssignments,
		Persona:                     existing.Persona,
		LastUpdateCheck:             existing.LastUpdateCheck,
		PendingSync:                 existing.PendingSync,
	}
}

// Write persists the full install state to disk under the given home directory.
// It creates the .gentle-ai directory if it does not already exist.
func Write(homeDir string, s InstallState) error {
	dir := filepath.Join(homeDir, stateDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(homeDir), append(data, '\n'), 0o644)
}
