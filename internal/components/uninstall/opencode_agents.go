package uninstall

import (
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/opencodeagents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// removeOpenCodeGentleman is scoped to the persona component, independently
// of whether the other components of this runtime are being removed.
func removeOpenCodeGentleman(path string, agentID model.AgentID) operation {
	return operation{typeID: opRewriteFile, path: path, apply: func(path string) (bool, bool, error) {
		raw, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			return false, false, nil
		}
		if err != nil {
			return false, false, err
		}
		root, err := filemerge.UnmarshalJSONObject(raw)
		if err != nil {
			return false, false, err
		}
		agents, _ := root["agent"].(map[string]any)
		entry, _ := agents["gentleman"].(map[string]any)
		if entry == nil {
			return false, false, nil
		}
		owned, err := opencodeagents.GentlemanShape(agentID, entry)
		if err != nil || !owned {
			return false, false, err
		}
		delete(agents, "gentleman")
		encoded, err := filemerge.MarshalJSONPreservingPermissions(raw, root)
		if err != nil {
			return false, false, err
		}
		result, err := filemerge.WriteFileAtomic(path, append(encoded, '\n'), filemerge.ExistingFileMode(path, 0o644))
		return result.Changed, false, err
	}}
}

// removeOpenCodeFamilyAgents rewrites only runtime-owned entries with a
// managed shape or the legacy v3.7.0 ownership marker.
func removeOpenCodeFamilyAgents(path string, agentID model.AgentID) operation {
	return operation{typeID: opRewriteFile, path: path, apply: func(path string) (bool, bool, error) {
		raw, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			return false, false, nil
		}
		if err != nil {
			return false, false, err
		}
		root, err := filemerge.UnmarshalJSONObject(raw)
		if err != nil {
			return false, false, err
		}
		agents, _ := root["agent"].(map[string]any)
		if agents == nil {
			return false, false, nil
		}
		changed := false
		removed := map[string]bool{}
		for name, value := range agents {
			entry, ok := value.(map[string]any)
			if !ok {
				continue
			}
			if entry["__managed_by"] == "gentle-ai/sdd" {
				// Mirror the migration ownership rules without requiring a sync.
				if opencodeagents.LegacyOwned(agentID, name) {
					delete(agents, name)
					removed[name] = true
				} else {
					delete(entry, "__managed_by")
				}
				changed = true
				continue
			}
			var owned bool
			var err error
			switch {
			case name == "gentleman":
				owned, err = opencodeagents.GentlemanShape(agentID, entry)
			case opencodeagents.UninstallRole(agentID, name):
				owned, err = opencodeagents.Shape(name, entry)
			}
			if err != nil {
				return false, false, err
			}
			if owned {
				delete(agents, name)
				removed[name] = true
				changed = true
			}
		}
		orchestrator, _ := agents["gentle-orchestrator"].(map[string]any)
		if orchestrator != nil {
			permission, _ := orchestrator["permission"].(map[string]any)
			task, _ := permission["task"].(map[string]any)
			for name := range removed {
				if _, ok := task[name]; ok {
					delete(task, name)
					changed = true
				}
			}
			if len(task) == 0 && task != nil {
				delete(permission, "task")
			}
			if len(permission) == 0 && permission != nil {
				delete(orchestrator, "permission")
			}
			if prompt, ok := orchestrator["prompt"].(string); ok {
				clean, stripped := removeMarkdownSections(prompt, "orchestrator", "sdd-orchestrator", "agent-routing")
				if stripped {
					orchestrator["prompt"] = clean
					changed = true
				}
				if stripped && strings.TrimSpace(clean) == "" && managedOrchestratorSkeleton(orchestrator) {
					delete(agents, "gentle-orchestrator")
				}
			}
		}
		if !changed {
			return false, false, nil
		}
		encoded, err := filemerge.MarshalJSONPreservingPermissions(raw, root)
		if err != nil {
			return false, false, err
		}
		result, err := filemerge.WriteFileAtomic(path, append(encoded, '\n'), filemerge.ExistingFileMode(path, 0o644))
		return result.Changed, false, err
	}}
}

func managedOrchestratorSkeleton(entry map[string]any) bool {
	for key := range entry {
		switch key {
		case "prompt", "model", "variant":
		default:
			return false
		}
	}
	return true
}
