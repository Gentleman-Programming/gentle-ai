package agenthooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// InstallCodexTelemetry installs runtime stop hooks independently of SDD selection.
func InstallCodexTelemetry(homeDir string, adapter agents.Adapter) (Result, error) {
	if adapter.Agent() != model.AgentCodex {
		return Result{}, nil
	}
	hooksPath := filepath.Join(adapter.GlobalConfigDir(homeDir), "hooks.json")
	root := map[string]any{}
	info, err := os.Lstat(hooksPath)
	if err != nil && !os.IsNotExist(err) {
		return Result{}, err
	}
	if err == nil {
		if !info.Mode().IsRegular() {
			return Result{}, fmt.Errorf("Codex hooks %q is not a regular file", hooksPath)
		}
		data, readErr := os.ReadFile(hooksPath)
		if readErr != nil {
			return Result{}, readErr
		}
		if len(strings.TrimSpace(string(data))) > 0 {
			if err := json.Unmarshal(data, &root); err != nil {
				return Result{}, fmt.Errorf("parse Codex hooks %q: %w", hooksPath, err)
			}
		}
	}
	hooksRaw, hasHooks := root["hooks"]
	hooksMap, _ := hooksRaw.(map[string]any)
	if hasHooks && hooksMap == nil {
		return Result{}, fmt.Errorf("Codex hooks %q has unsupported hooks shape: want object", hooksPath)
	}
	if hooksMap == nil {
		hooksMap = map[string]any{}
	}
	changed := false
	const telemetryCommand = `gentle-ai telemetry runtime codex --json`
	for _, event := range []string{"SubagentStop", "Stop"} {
		if codexHookCommandExists(hooksMap, event, telemetryCommand) {
			continue
		}
		raw, exists := hooksMap[event]
		entries, _ := raw.([]any)
		if exists && entries == nil {
			return Result{}, fmt.Errorf("Codex hooks %q has unsupported hooks.%s shape: want array", hooksPath, event)
		}
		entries = append(entries, map[string]any{"hooks": []any{map[string]any{
			"type": "command", "command": telemetryCommand, "async": true, "timeout": 4,
		}}})
		hooksMap[event] = entries
		changed = true
	}
	if !changed {
		return Result{Files: []string{hooksPath}}, nil
	}
	root["hooks"] = hooksMap
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return Result{}, err
	}
	out = append(out, '\n')
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0o755); err != nil {
		return Result{}, err
	}
	wr, err := filemerge.WriteFileAtomic(hooksPath, out, 0o644)
	if err != nil {
		return Result{}, err
	}
	return Result{Changed: wr.Changed, Files: []string{hooksPath}}, nil
}

func codexHookCommandExists(hooksMap map[string]any, event, command string) bool {
	entries, _ := hooksMap[event].([]any)
	for _, entry := range entries {
		entryMap, _ := entry.(map[string]any)
		hooks, _ := entryMap["hooks"].([]any)
		for _, hook := range hooks {
			hookMap, _ := hook.(map[string]any)
			if hookMap["command"] == command {
				return true
			}
		}
	}
	return false
}
