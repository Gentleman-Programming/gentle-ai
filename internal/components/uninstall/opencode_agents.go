package uninstall

import (
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/opencodeagents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// removeOpenCodeFamilyAgents rewrites only entries whose installed shape proves ownership.
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
			owned, err := opencodeagents.Shape(name, entry)
			if name == "gentleman" {
				owned, err = opencodeagents.GentlemanShape(agentID, entry)
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
