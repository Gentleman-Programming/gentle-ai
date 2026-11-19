package opencodedefault

import (
	"os"
	"sort"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
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
		agents = nil
	}
	// Native V2 documents define agents under `agents`; scan both maps so
	// user-owned identities are discovered regardless of the document shape.
	nativeAgents, _ := root["agents"].(map[string]any)
	if agents == nil && nativeAgents == nil {
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
	// #4471: the ODD parity agents ported from Gentle Shell are managed like
	// JD/review-lens phases but are not counted as "configurable legacy SDD
	// identities" (that function also backs the telemetry runtime agent-class
	// allowlist, which does not need these three names).
	for _, phase := range opencode.GentleAIODDPhases() {
		reserved[phase] = true
	}
	var custom []string
	for _, scope := range []map[string]any{agents, nativeAgents} {
		for name, definition := range scope {
			if _, valid := definition.(map[string]any); !valid {
				continue
			}
			if !reserved[name] {
				custom = append(custom, name)
			}
		}
	}
	sort.Strings(custom)
	return custom, nil
}
