package opencodedefault

import (
	"os"
	"sort"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/opencode"
)

// DiscoverCustomAgents returns user-owned agent keys in OpenCode settings, sorted
// by key. Invalid settings or agent sections cannot provide trusted identities.
func DiscoverCustomAgents(settingsPath string) ([]string, error) {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	root, err := filemerge.UnmarshalJSONObject(data)
	if err != nil {
		return nil, nil
	}
	agents, ok := root["agent"].(map[string]any)
	if !ok {
		return nil, nil
	}

	reserved := map[string]bool{
		"build": true, "plan": true, "general": true, "explore": true,
		"gentle-reviewer": true, "gentle-worker": true,
		"gentle-orchestrator": true, "sdd-orchestrator": true,
	}
	for _, phase := range opencode.ConfigurableAgentPhases() {
		reserved[phase] = true
	}
	var custom []string
	for name, definition := range agents {
		if _, valid := definition.(map[string]any); !valid {
			continue
		}
		if !reserved[name] {
			custom = append(custom, name)
		}
	}
	sort.Strings(custom)
	return custom, nil
}
