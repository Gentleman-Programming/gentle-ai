package agenthooks

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// InstallRetainedClaudeHooks installs review and telemetry hooks independently of SDD.
func InstallRetainedClaudeHooks(homeDir string, adapter agents.Adapter) (Result, error) {
	if adapter.Agent() != model.AgentClaudeCode {
		return Result{}, nil
	}
	path := adapter.SettingsPath(homeDir)
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
		data, err := os.ReadFile(path)
		if err != nil {
			return Result{}, err
		}
		if len(data) > 0 {
			if err := json.Unmarshal(data, &root); err != nil {
				return Result{}, fmt.Errorf("parse Claude settings %q: %w", path, err)
			}
		}
	}
	raw, exists := root["hooks"]
	hooks, ok := raw.(map[string]any)
	if exists && !ok {
		return Result{}, fmt.Errorf("Claude settings %q has unsupported hooks shape: want object", path)
	}
	if !ok {
		hooks = map[string]any{}
	}
	command := fmt.Sprintf("gentle-ai review stop-hook --agent %s", adapter.Agent())
	entries := []struct {
		key, matcher, command string
		timeout               int
		async                 bool
	}{
		{"Stop", "", command, 60, false},
		{"SessionStart", "startup|resume|clear|compact", command, 30, false},
		{"SubagentStop", "", "gentle-ai telemetry runtime claude --json", 5, true},
		{"Stop", "", "gentle-ai telemetry runtime claude --json", 5, true},
	}
	changed := false
	for _, e := range entries {
		raw, exists := hooks[e.key]
		list, ok := raw.([]any)
		if exists && !ok {
			return Result{}, fmt.Errorf("Claude settings %q has unsupported hooks.%s shape: want array", path, e.key)
		}
		if claudeHookListContains(list, e.command) {
			continue
		}
		hook := map[string]any{"type": "command", "command": e.command, "timeout": e.timeout}
		if e.async {
			hook["async"] = true
		}
		hooks[e.key] = append(list, map[string]any{"matcher": e.matcher, "hooks": []any{hook}})
		changed = true
	}
	if !changed {
		return Result{Files: []string{path}}, nil
	}
	root["hooks"] = hooks
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return Result{}, err
	}
	wr, err := filemerge.WriteFileAtomic(path, append(out, '\n'), 0644)
	if err != nil {
		return Result{}, err
	}
	return Result{Changed: wr.Changed, Files: []string{path}}, nil
}

func claudeHookListContains(entries []any, command string) bool {
	for _, entry := range entries {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		hooks, ok := item["hooks"].([]any)
		if !ok {
			continue
		}
		for _, hook := range hooks {
			h, ok := hook.(map[string]any)
			if ok && h["command"] == command {
				return true
			}
		}
	}
	return false
}
