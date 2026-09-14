package pi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// ModelsConfigPath returns the canonical path to ~/.pi/gentle-ai/models.json.
func ModelsConfigPath(homeDir string) string {
	return filepath.Join(ConfigPath(homeDir), "gentle-ai", "models.json")
}

// SubagentsConfigPath returns the path to ~/.pi/agent/subagents.json.
func SubagentsConfigPath(homeDir string) string {
	return filepath.Join(AgentConfigPath(homeDir), "subagents.json")
}

// WriteModelsConfig writes the canonical agent routing configuration consumed by gentle-pi.
func WriteModelsConfig(homeDir string, assignments map[string]model.PiAgentModelEntry) error {
	path := ModelsConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating gentle-ai config directory: %w", err)
	}

	data, err := json.MarshalIndent(assignments, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling pi models config: %w", err)
	}
	data = append(data, '\n')

	return atomicWrite(path, data)
}

// UpdateSubagentsModelProfiles updates the model_profiles field in ~/.pi/agent/subagents.json
// while preserving all other configuration and comments.
func UpdateSubagentsModelProfiles(homeDir string, assignments map[string]model.PiAgentModelEntry) error {
	path := SubagentsConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating pi agent directory: %w", err)
	}

	raw := make(map[string]any)
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &raw)
	}

	profiles := make(map[string]map[string]string)
	if existing, ok := raw["model_profiles"].(map[string]any); ok {
		for k, v := range existing {
			if entry, ok := v.(map[string]any); ok {
				subProfile := make(map[string]string)
				for subK, subV := range entry {
					if s, ok := subV.(string); ok {
						subProfile[subK] = s
					}
				}
				profiles[k] = subProfile
			}
		}
	}

	for agent, entry := range assignments {
		if entry.Model == "" {
			continue
		}
		agentProfile := profiles[agent]
		if agentProfile == nil {
			agentProfile = make(map[string]string)
		}
		agentProfile["model"] = entry.Model
		if entry.Thinking != "" {
			agentProfile["effort"] = entry.Thinking
		}
		profiles[agent] = agentProfile
	}

	raw["model_profiles"] = profiles

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling subagents.json: %w", err)
	}
	data = append(data, '\n')

	return atomicWrite(path, data)
}

func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing temporary file %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("moving %q to %q: %w", tmp, path, err)
	}
	return nil
}
