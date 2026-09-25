// Package agenthooks owns runtime hook installation independent of optional components.
package agenthooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

type Result struct {
	Changed bool
	Files   []string
}

const claudeLegacySkillRegistryCommand = `gentle-ai skill-registry refresh --quiet --no-gitignore --cwd "${CLAUDE_PROJECT_DIR:-$PWD}" || true`

func claudeSkillRegistryCommand(platform string) string {
	if platform == "windows" {
		return `powershell -NoProfile -Command 'if (Test-Path env:CLAUDE_PROJECT_DIR) { $dir = $env:CLAUDE_PROJECT_DIR } else { $dir = $PWD }; gentle-ai skill-registry refresh --quiet --no-gitignore --cwd "$dir"; exit 0'`
	}
	return claudeLegacySkillRegistryCommand
}

// pruneClaudeLegacySkillRegistryHooks removes only the retired POSIX command
// under UserPromptSubmit, preserving unrelated hooks and their outer entries.
func pruneClaudeLegacySkillRegistryHooks(entries []any) ([]any, bool) {
	var keptEntries []any
	changed := false
	for _, entry := range entries {
		item, ok := entry.(map[string]any)
		if !ok {
			keptEntries = append(keptEntries, entry)
			continue
		}
		hooks, ok := item["hooks"].([]any)
		if !ok {
			keptEntries = append(keptEntries, entry)
			continue
		}
		var keptHooks []any
		for _, hook := range hooks {
			h, ok := hook.(map[string]any)
			if ok && h["command"] == claudeLegacySkillRegistryCommand {
				changed = true
				continue
			}
			keptHooks = append(keptHooks, hook)
		}
		if len(keptHooks) == len(hooks) {
			keptEntries = append(keptEntries, entry)
			continue
		}
		if len(keptHooks) == 0 {
			continue
		}
		copyItem := make(map[string]any, len(item))
		for k, v := range item {
			copyItem[k] = v
		}
		copyItem["hooks"] = keptHooks
		keptEntries = append(keptEntries, copyItem)
	}
	return keptEntries, changed
}

// InstallSkillRegistry installs startup refresh hooks without selecting SDD.
func InstallSkillRegistry(homeDir string, adapter agents.Adapter) (Result, error) {
	return installSkillRegistry(homeDir, adapter, runtime.GOOS)
}

func installSkillRegistry(homeDir string, adapter agents.Adapter, platform string) (Result, error) {
	var path, event, command string
	switch adapter.Agent() {
	case model.AgentCodex:
		path = filepath.Join(adapter.GlobalConfigDir(homeDir), "hooks.json")
		event = "SessionStart"
		command = `gentle-ai skill-registry refresh --quiet --no-gitignore --cwd "$PWD" || true`
	case model.AgentClaudeCode:
		path = adapter.SettingsPath(homeDir)
		event = "UserPromptSubmit"
		command = claudeSkillRegistryCommand(platform)
	default:
		return Result{}, nil
	}
	if path == "" {
		return Result{}, nil
	}
	root := map[string]any{}
	info, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		return Result{}, err
	}
	if err == nil {
		if !info.Mode().IsRegular() {
			return Result{}, fmt.Errorf("hook settings %q is not a regular file", path)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return Result{}, readErr
		}
		if len(strings.TrimSpace(string(data))) > 0 {
			if jsonErr := json.Unmarshal(data, &root); jsonErr != nil {
				return Result{}, fmt.Errorf("parse hook settings %q: %w", path, jsonErr)
			}
		}
	}
	hooks, ok := root["hooks"].(map[string]any)
	if _, exists := root["hooks"]; exists && !ok {
		return Result{}, fmt.Errorf("hook settings %q has unsupported hooks shape: want object", path)
	}
	if !ok {
		hooks = map[string]any{}
	}
	entries, ok := hooks[event].([]any)
	if _, exists := hooks[event]; exists && !ok {
		return Result{}, fmt.Errorf("hook settings %q has unsupported hooks.%s shape: want array", path, event)
	}
	pruned := false
	if adapter.Agent() == model.AgentClaudeCode && platform == "windows" {
		entries, pruned = pruneClaudeLegacySkillRegistryHooks(entries)
		if pruned {
			if len(entries) == 0 {
				delete(hooks, event)
			} else {
				hooks[event] = entries
			}
		}
	}
	if claudeHookListContains(entries, command) {
		if !pruned {
			return Result{Files: []string{path}}, nil
		}
	} else {
		hook := map[string]any{"type": "command", "command": command}
		entry := map[string]any{"matcher": "", "hooks": []any{hook}}
		if adapter.Agent() == model.AgentCodex {
			entry["matcher"] = "startup|resume|clear|compact"
			hook["timeout"] = 30
			hook["statusMessage"] = "Refreshing skill registry"
		}
		hooks[event] = append(entries, entry)
	}
	root["hooks"] = hooks
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Result{}, err
	}
	wr, err := filemerge.WriteFileAtomic(path, append(out, '\n'), 0o644)
	if err != nil {
		return Result{}, err
	}
	return Result{Changed: wr.Changed, Files: []string{path}}, nil
}
