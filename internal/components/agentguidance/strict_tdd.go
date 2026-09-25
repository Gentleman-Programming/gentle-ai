package agentguidance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/opencodedefault"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

const strictTDDContent = "Strict TDD Mode: enabled"

// StrictTDDPath identifies the supported agent's native system-prompt carrier.
func StrictTDDPath(targetDir string, agent model.AgentID) string {
	adapter, err := agents.NewAdapter(agent)
	if err != nil || !adapter.SupportsSystemPrompt() || agent == model.AgentPi {
		return ""
	}
	switch adapter.SystemPromptStrategy() {
	case model.StrategyJinjaModules:
		return filepath.Join(adapter.GlobalConfigDir(targetDir), "strict-tdd-mode.md")
	case model.StrategyMarkdownSections, model.StrategyFileReplace, model.StrategyAppendToFile,
		model.StrategyInstructionsFile, model.StrategySteeringFile:
		if agent == model.AgentOpenCode || agent == model.AgentKilocode {
			return "" // Their managed prompt lives in settings, not this file.
		}
		return adapter.SystemPromptFile(targetDir)
	default:
		return ""
	}
}

// InjectStrictTDDWithOptions manages only its owned Markdown section or Kimi
// include module, using the same settings authority as routing for
// OpenCode-family agents. Unsupported carriers are left untouched; their
// integration belongs to another slice.
func InjectStrictTDDWithOptions(targetDir string, agent model.AgentID, enabled bool, options RoutingOptions) (Result, error) {
	if DeliversThroughOrchestratorPrompt(agent) {
		delivery, err := resolveRoutingDelivery(targetDir, agent, options)
		if err != nil {
			return Result{}, err
		}
		path := delivery.paths[0]
		raw, err := readBytesOrEmpty(path)
		if err != nil {
			return Result{}, err
		}
		settings, err := filemerge.UnmarshalJSONObject(raw)
		if err != nil {
			return Result{}, fmt.Errorf("%w %q: %w", ErrUnreadableSettings, path, err)
		}
		prompt, err := managedOrchestratorPrompt(settings, path)
		if err != nil {
			return Result{}, err
		}
		if !enabled && !strings.Contains(prompt, "<!-- gentle-ai:strict-tdd-mode -->") {
			return Result{}, nil
		}
		content := strictTDDContent
		if !enabled {
			content = ""
		}
		updated := filemerge.InjectMarkdownSection(prompt, "strict-tdd-mode", content)
		if updated == prompt {
			return Result{}, nil
		}
		overlay, err := json.Marshal(map[string]any{"agent": map[string]any{opencodedefault.ManagedAgent: map[string]any{"prompt": updated}}})
		if err != nil {
			return Result{}, err
		}
		merged, err := filemerge.MergeJSONObjects(raw, overlay)
		if err != nil {
			return Result{}, err
		}
		written, err := filemerge.WriteFileAtomic(path, merged, 0o644)
		if err != nil {
			return Result{}, err
		}
		return Result{Changed: written.Changed, Files: []string{path}}, nil
	}
	path := StrictTDDPath(targetDir, agent)
	if path == "" {
		return Result{}, nil
	}
	current, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return Result{}, err
	}
	exists := err == nil
	if agent == model.AgentKimi {
		if exists && string(current) != strictTDDContent {
			return Result{}, fmt.Errorf("strict TDD module %q is modified; refusing to overwrite or remove", path)
		}
		if !enabled {
			if !exists {
				return Result{}, nil
			}
			if err := os.Remove(path); err != nil {
				return Result{}, err
			}
			return Result{Changed: true, Files: []string{path}}, nil
		}
		written, err := filemerge.WriteFileAtomic(path, []byte(strictTDDContent), 0o644)
		if err != nil {
			return Result{}, err
		}
		return Result{Changed: written.Changed, Files: []string{path}}, nil
	}
	if !enabled && (!exists || !strings.Contains(string(current), "<!-- gentle-ai:strict-tdd-mode -->")) {
		return Result{}, nil
	}
	content := strictTDDContent
	if !enabled {
		content = ""
	}
	updated := filemerge.InjectMarkdownSection(string(current), "strict-tdd-mode", content)
	if updated == string(current) {
		return Result{}, nil
	}
	written, err := filemerge.WriteFileAtomic(path, []byte(updated), 0o644)
	if err != nil {
		return Result{}, err
	}
	return Result{Changed: written.Changed, Files: []string{path}}, nil
}
