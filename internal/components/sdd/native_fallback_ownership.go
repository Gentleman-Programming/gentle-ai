package sdd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

const nativeFallbackOwnershipSchema = "gentle-ai.opencode-native-fallbacks"

var writeNativeFallbackOwnershipFile = filemerge.WriteFileAtomic

type nativeFallbackAssignment struct {
	Model   string `json:"model"`
	Variant string `json:"variant"`
}

type nativeFallbackOwnership struct {
	Schema  string                              `json:"schema"`
	Version int                                 `json:"version"`
	Agents  map[string]nativeFallbackAssignment `json:"agents"`
}

func nativeFallbackOwnershipPath(settingsPath string) string {
	return filepath.Join(filepath.Dir(settingsPath), ".gentle-ai-native-fallbacks.json")
}

// NativeFallbackOwnershipPath returns the sidecar managed with OpenCode SDD settings.
func NativeFallbackOwnershipPath(settingsPath string) string {
	return nativeFallbackOwnershipPath(settingsPath)
}

func emptyNativeFallbackOwnership() nativeFallbackOwnership {
	return nativeFallbackOwnership{Schema: nativeFallbackOwnershipSchema, Version: 1, Agents: map[string]nativeFallbackAssignment{}}
}

func readNativeFallbackOwnership(settingsPath string, existing map[string]bool) (nativeFallbackOwnership, error) {
	owned := emptyNativeFallbackOwnership()
	raw, err := os.ReadFile(nativeFallbackOwnershipPath(settingsPath))
	if os.IsNotExist(err) {
		return owned, nil
	}
	if err != nil {
		return owned, fmt.Errorf("read OpenCode native fallback ownership: %w", err)
	}
	var decoded nativeFallbackOwnership
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.Schema != nativeFallbackOwnershipSchema || decoded.Version != 1 {
		return owned, nil
	}
	if decoded.Agents == nil {
		decoded.Agents = map[string]nativeFallbackAssignment{}
	}
	owned = decoded
	current, err := readNativeFallbackAssignments(settingsPath)
	if err != nil {
		return emptyNativeFallbackOwnership(), err
	}
	for role, assignment := range owned.Agents {
		if !existing[role] || current[role] != assignment {
			delete(owned.Agents, role)
		}
	}
	return owned, nil
}

func readNativeFallbackAssignments(settingsPath string) (map[string]nativeFallbackAssignment, error) {
	raw, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		return map[string]nativeFallbackAssignment{}, nil
	}
	if err != nil {
		return nil, err
	}
	var root struct {
		Agent map[string]nativeFallbackAssignment `json:"agent"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return map[string]nativeFallbackAssignment{}, nil
	}
	if root.Agent == nil {
		return map[string]nativeFallbackAssignment{}, nil
	}
	return root.Agent, nil
}

func nativeFallbackDefaults(sddMode model.SDDModeID, assignments map[string]model.ModelAssignment, rootModel string) map[string]nativeFallbackAssignment {
	defaults := make(map[string]nativeFallbackAssignment)
	if sddMode == model.SDDModeMulti {
		if assignment := assignments["sdd-mid"]; assignment.ProviderID != "" && assignment.ModelID != "" {
			defaults["general"] = nativeFallbackAssignment{Model: assignment.FullID(), Variant: assignment.Effort}
			defaults["explore"] = defaults["general"]
		}
		if assignment := assignments["sdd-explore"]; assignment.ProviderID != "" && assignment.ModelID != "" {
			defaults["explore"] = nativeFallbackAssignment{Model: assignment.FullID(), Variant: assignment.Effort}
		}
		return defaults
	}
	if rootModel != "" {
		for _, role := range []string{"general", "explore"} {
			if _, exists := defaults[role]; !exists {
				defaults[role] = nativeFallbackAssignment{Model: rootModel, Variant: "medium"}
			}
		}
	}
	return defaults
}

func (o *nativeFallbackOwnership) reconcile(assignments map[string]model.ModelAssignment, existing map[string]bool, managed map[string]nativeFallbackAssignment) {
	for _, role := range []string{"general", "explore"} {
		if assignment := assignments[role]; assignment.ProviderID != "" && assignment.ModelID != "" {
			delete(o.Agents, role)
			continue
		}
		if assignment, ok := managed[role]; ok {
			o.Agents[role] = assignment
			continue
		}
		if existing[role] && o.Agents[role].Model == "" {
			delete(o.Agents, role)
		}
	}
}

func (o nativeFallbackOwnership) write(settingsPath string) (bool, error) {
	path := nativeFallbackOwnershipPath(settingsPath)
	if len(o.Agents) == 0 {
		if err := os.Remove(path); err == nil {
			return true, nil
		} else if !os.IsNotExist(err) {
			return false, err
		}
		return false, nil
	}
	raw, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return false, err
	}
	raw = append(raw, '\n')
	current, err := os.ReadFile(path)
	if err == nil && bytes.Equal(current, raw) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	result, err := writeNativeFallbackOwnershipFile(path, raw, 0o644)
	return result.Changed, err
}
