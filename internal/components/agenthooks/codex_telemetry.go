package agenthooks

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// InstallCodexTelemetry installs runtime stop hooks independently of SDD selection.
func InstallCodexTelemetry(homeDir string, adapter agents.Adapter) (Result, error) {
	if adapter.Agent() != model.AgentCodex {
		return Result{}, nil
	}
	hooksPath := filepath.Join(adapter.GlobalConfigDir(homeDir), "hooks.json")
	var data []byte
	info, err := os.Lstat(hooksPath)
	if err != nil && !os.IsNotExist(err) {
		return Result{}, err
	}
	if err == nil {
		if !info.Mode().IsRegular() {
			return Result{}, fmt.Errorf("Codex hooks %q is not a regular file", hooksPath)
		}
		if data, err = os.ReadFile(hooksPath); err != nil {
			return Result{}, err
		}
	}
	root, err := decodeHookSettings(data)
	if err != nil {
		return Result{}, fmt.Errorf("parse Codex hooks %q: %w", hooksPath, err)
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
	out, err := encodeHookSettings(data, root, hooksMap)
	if err != nil {
		return Result{}, fmt.Errorf("rewrite Codex hooks %q: %w", hooksPath, err)
	}
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
