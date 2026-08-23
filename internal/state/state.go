package state

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
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

// CodexOrchestratorAssignmentState is the persisted main-session assignment.
type CodexOrchestratorAssignmentState struct {
	Model  string `json:"model"`
	Effort string `json:"effort"`
}

// ClaudePhaseAssignmentState is the JSON-serialisable form of a Claude
// subagent model+effort assignment. Empty Effort means Claude Code default.
type ClaudePhaseAssignmentState struct {
	Model  string `json:"model"`
	Effort string `json:"effort,omitempty"`
}

// InstallState holds the persisted user selections from the last install run.
type InstallState struct {
	InstalledAgents        []string            `json:"installed_agents"`
	InstalledBinaryVersion string              `json:"installed_binary_version,omitempty"`
	ManagedAssetDigest     string              `json:"managed_asset_digest,omitempty"`
	SelectionConfigured    bool                `json:"selection_configured,omitempty"`
	Components             []model.ComponentID `json:"components,omitempty"`
	Skills                 []model.SkillID     `json:"skills,omitempty"`
	Preset                 model.PresetID      `json:"preset,omitempty"`
	SDDMode                model.SDDModeID     `json:"sdd_mode,omitempty"`
	StrictTDD              bool                `json:"strict_tdd,omitempty"`
	// CommunityTools records optional tools explicitly selected in the Gentle AI
	// installer. Configured distinguishes a completed empty selection from legacy
	// state files that predate persistence of this choice.
	CommunityTools           []string `json:"community_tools,omitempty"`
	CommunityToolsConfigured bool     `json:"community_tools_configured,omitempty"`

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

	// CodexOrchestratorAssignment is optional so legacy state preserves the user's top-level Codex configuration.
	CodexOrchestratorAssignment *CodexOrchestratorAssignmentState `json:"codexOrchestratorAssignment,omitempty"`

	// CodexCarrilModelAssignments maps the three carril profile names
	// (sdd-strong|sdd-mid|sdd-cheap) to OpenAI subscription model IDs
	// (e.g. "gpt-5.6-sol", "gpt-5.6-luna"). Persisted so that `gentle-ai sync`
	// regenerates profile files with the user's chosen model per tier.
	// Absent/empty = resolve to DefaultCarrilModels at runtime (backward-compat).
	CodexCarrilModelAssignments map[string]string `json:"codexCarrilModelAssignments,omitempty"`

	// CodexPhaseModelAssignments maps each of the 13 SDD phase names to the
	// model id the user assigned in the Custom per-phase picker (e.g. "gpt-5.6-sol").
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
	// PersonaPresent distinguishes an omitted legacy field from an explicit
	// empty persona, which must fail closed during sync validation.
	PersonaPresent bool `json:"-"`

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

	// RDDMode is the global, user-owned receipt-driven-development kill switch
	// ("on"|"off"). It lives in uncommitted user state precisely so that no
	// repository can ship, share, or force a review policy onto a clone.
	// Empty means the user never expressed a preference, which preserves the
	// historical enabled behavior for state files that predate the switch.
	// The value is deliberately kept as a plain string: reviewtransaction owns
	// validation and fails closed on anything it does not recognise.
	RDDMode string `json:"rdd_mode,omitempty"`

	// RDDModeRecordedAt is the candidate cutoff for the global mode. Re-enabling
	// must only affect future candidates, so the moment the current value was
	// recorded is authority, not a cosmetic audit field. Nil for state files
	// written before the switch existed.
	RDDModeRecordedAt *time.Time `json:"rdd_mode_recorded_at,omitempty"`

	BackgroundIntent model.OpenCodeBackgroundIntent `json:"opencode_background_subagents,omitempty"`

	// PiBackgroundIntent is the managed Pi background-subagent choice. It is
	// persisted separately from the OpenCode field because each key is part of
	// an independent state contract.
	PiBackgroundIntent model.PiBackgroundIntent `json:"pi_background_subagents,omitempty"`
}

// UnmarshalJSON preserves whether the persisted persona field was present.
// The value itself is still decoded into the public InstallState field.
func (s *InstallState) UnmarshalJSON(data []byte) error {
	type plainInstallState InstallState
	var decoded plainInstallState
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	*s = InstallState(decoded)
	_, s.PersonaPresent = fields["persona"]
	return nil
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

func (s *InstallState) SetSelection(selection model.Selection) {
	s.SelectionConfigured = true
	s.Components = append([]model.ComponentID(nil), selection.Components...)
	s.Skills = append([]model.SkillID(nil), selection.Skills...)
	s.Preset, s.SDDMode, s.StrictTDD = selection.Preset, selection.SDDMode, selection.StrictTDD
}

func (s InstallState) RestoreSelection(selection *model.Selection) {
	if !s.SelectionConfigured {
		return
	}
	selection.Components = append([]model.ComponentID(nil), s.Components...)
	selection.Skills = append([]model.SkillID(nil), s.Skills...)
	selection.Preset, selection.SDDMode, selection.StrictTDD = s.Preset, s.SDDMode, s.StrictTDD
}

// MergeAgents returns a new InstallState that combines existing with the
// provided newAgents. The new agents are appended to existing.InstalledAgents
// with deduplication. All other persisted selections, including community
// tools, model assignments, and persona, are preserved from existing.
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
		InstalledBinaryVersion:      existing.InstalledBinaryVersion,
		ManagedAssetDigest:          existing.ManagedAssetDigest,
		SelectionConfigured:         existing.SelectionConfigured,
		Components:                  existing.Components,
		Skills:                      existing.Skills,
		Preset:                      existing.Preset,
		SDDMode:                     existing.SDDMode,
		StrictTDD:                   existing.StrictTDD,
		CommunityTools:              existing.CommunityTools,
		CommunityToolsConfigured:    existing.CommunityToolsConfigured,
		ModelAssignments:            existing.ModelAssignments,
		ClaudeModelAssignments:      existing.ClaudeModelAssignments,
		ClaudePhaseAssignments:      existing.ClaudePhaseAssignments,
		KiroModelAssignments:        existing.KiroModelAssignments,
		CodexModelAssignments:       existing.CodexModelAssignments,
		CodexOrchestratorAssignment: existing.CodexOrchestratorAssignment,
		CodexCarrilModelAssignments: existing.CodexCarrilModelAssignments,
		CodexPhaseModelAssignments:  existing.CodexPhaseModelAssignments,
		Persona:                     existing.Persona,
		PersonaPresent:              existing.PersonaPresent,
		LastUpdateCheck:             existing.LastUpdateCheck,
		PendingSync:                 existing.PendingSync,
		RDDMode:                     existing.RDDMode,
		RDDModeRecordedAt:           existing.RDDModeRecordedAt,

		BackgroundIntent:   existing.BackgroundIntent,
		PiBackgroundIntent: existing.PiBackgroundIntent,
	}
}

// Write persists the full install state to disk under the given home directory.
// It creates the .gentle-ai directory if it does not already exist.
func Write(homeDir string, s InstallState) error {
	dir := filepath.Join(homeDir, stateDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := marshal(s)
	if err != nil {
		return err
	}
	_, err = filemerge.WriteFileAtomic(Path(homeDir), data, 0o644)
	return err
}

// WriteReconciled persists install state and treats an atomic-write error as
// successful when the requested bytes are visible on disk after the error.
func WriteReconciled(homeDir string, s InstallState) error {
	err := Write(homeDir, s)
	if err == nil {
		return nil
	}

	data, marshalErr := marshal(s)
	if marshalErr == nil {
		if visible, readErr := os.ReadFile(Path(homeDir)); readErr == nil && bytes.Equal(visible, data) {
			log.Printf("state: write returned %v but requested state is visible; treating persistence as successful", err)
			return nil
		}
	}
	return err
}

func marshal(s InstallState) ([]byte, error) {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
