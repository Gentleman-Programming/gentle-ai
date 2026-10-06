package legacyassets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// ClaudeSDDPreflightHookCommand is the command v2.8.0 to v3.7.0 registered as
// a Claude Code PreToolUse(Agent) hook, bound to the installing runtime.
// v4.0.0 removed the command, so the hook fails on every subagent launch
// (#5157). It is matched, never printed as guidance.
const ClaudeSDDPreflightHookCommand = "gentle-ai sdd-preflight-hook --agent " + string(model.AgentClaudeCode)

// releasedClaudeSDDPreflightHook is the hook object every release wrote.
var releasedClaudeSDDPreflightHook = map[string]any{"type": "command", "command": ClaudeSDDPreflightHookCommand, "timeout": json.Number("30")}

// HookRetireResult reports whether the released hook was removed and whether
// a differing hook that runs the retired command was kept.
type HookRetireResult struct {
	Path            string
	Removed, Edited bool
}

// RetireClaudeSDDPreflightHook removes from hooks.PreToolUse of the Claude
// Code settings at path exactly the hook object releases wrote, then the
// matcher group it leaves empty when that group is the released
// {"matcher": "Agent", "hooks": [...]} shape, then an empty PreToolUse list.
// Every other key, hook, and group is preserved. A hook that runs the retired
// command in any other shape is kept and reported. The file is rewritten
// only when something was removed.
func RetireClaudeSDDPreflightHook(path string) (HookRetireResult, error) {
	result := HookRetireResult{Path: path}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("inspect Claude settings %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return result, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(raw, []byte(ClaudeSDDPreflightHookCommand)) {
		return result, err
	}
	if err := filemerge.RejectDuplicateJSONKeys(raw); err != nil {
		return result, fmt.Errorf("parse Claude settings %s: %w", path, err)
	}
	root, err := filemerge.DecodeStrictJSONObject(raw)
	if err != nil {
		return result, fmt.Errorf("parse Claude settings %s: %w", path, err)
	}
	hooks, _ := root["hooks"].(map[string]any)
	for event, value := range hooks {
		groups, _ := value.([]any)
		for _, group := range groups {
			entry, _ := group.(map[string]any)
			list, _ := entry["hooks"].([]any)
			for _, hook := range list {
				object, _ := hook.(map[string]any)
				if object["command"] == ClaudeSDDPreflightHookCommand && (event != "PreToolUse" || !reflect.DeepEqual(object, releasedClaudeSDDPreflightHook)) {
					result.Edited = true
				}
			}
		}
	}
	groups, _ := hooks["PreToolUse"].([]any)
	kept := groups[:0:0]
	for _, group := range groups {
		entry, ok := group.(map[string]any)
		list, _ := entry["hooks"].([]any)
		if !ok || list == nil {
			kept = append(kept, group)
			continue
		}
		remaining := list[:0:0]
		for _, hook := range list {
			if reflect.DeepEqual(hook, releasedClaudeSDDPreflightHook) {
				result.Removed = true
				continue
			}
			remaining = append(remaining, hook)
		}
		if len(remaining) == len(list) {
			kept = append(kept, group)
			continue
		}
		if len(remaining) == 0 && len(entry) == 2 && entry["matcher"] == "Agent" {
			continue
		}
		entry["hooks"] = remaining
		kept = append(kept, entry)
	}
	if !result.Removed {
		return result, nil
	}
	if len(kept) == 0 {
		delete(hooks, "PreToolUse")
	} else {
		hooks["PreToolUse"] = kept
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(root); err != nil {
		return result, err
	}
	if _, err := filemerge.WriteFileAtomic(path, out.Bytes(), info.Mode().Perm()); err != nil {
		return result, err
	}
	return result, nil
}

// ManualActions tells the user what to do with a retired hook that was kept.
func (r HookRetireResult) ManualActions() []string {
	if !r.Edited {
		return nil
	}
	return []string{fmt.Sprintf("Claude Code settings %s still have a hook that runs `gentle-ai sdd-preflight-hook`, a command removed in v4.0.0. It differs from the hook Gentle AI installed, so it was kept, and it fails on every run. If you no longer need it, move or delete that hook entry.", r.Path)}
}
