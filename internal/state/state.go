package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const stateDir = ".gentle-ai"
const stateFile = "state.json"

// ModelAssignmentState is the JSON-serialisable form of a provider+model pair
// used by OpenCode-style model assignments. It mirrors model.ModelAssignment
// but lives in the state package to avoid an import cycle.
type ModelAssignmentState struct {
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
}

// InstallState holds the persisted user selections from the last install run.
type InstallState struct {
	InstalledAgents []string `json:"installed_agents"`

	// ClaudeModelAssignments maps SDD phase names (e.g. "sdd-explore") to a
	// Claude model alias ("opus", "sonnet", "haiku"). Persisted so that
	// `gentle-ai sync` preserves the user's model choices instead of falling
	// back to the "balanced" preset every time.
	ClaudeModelAssignments map[string]string `json:"claude_model_assignments,omitempty"`

	// ModelAssignments maps sub-agent names to provider/model pairs (OpenCode).
	ModelAssignments map[string]ModelAssignmentState `json:"model_assignments,omitempty"`
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
