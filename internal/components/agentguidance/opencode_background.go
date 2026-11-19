package agentguidance

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/opencodedefault"
	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
)

const (
	backgroundStart = "<!-- gentle-ai:opencode-background-subagents -->"
	backgroundEnd   = "<!-- /gentle-ai:opencode-background-subagents -->"
)

// ApplyOpenCodeBackgroundPolicy updates only the managed orchestrator prompt.
// An absent agent is not created merely to carry background policy.
func ApplyOpenCodeBackgroundPolicy(settingsPath string, enabled bool) (Result, error) {
	raw, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		return Result{}, nil
	}
	if err != nil {
		return Result{}, err
	}
	settings, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		return Result{}, fmt.Errorf("parse OpenCode settings: %w", err)
	}
	// Native documents carry the orchestrator prompt in agents.<name>.system;
	// V2 wins over the legacy map when both exist.
	useNative := opencode.NativeConfig(settings)
	field := "prompt"
	var managed map[string]any
	if useNative {
		nativeAgents, _ := settings["agents"].(map[string]any)
		managed, _ = nativeAgents[opencodedefault.ManagedAgent].(map[string]any)
		if managed == nil {
			return Result{}, nil
		}
		field = "system"
	} else {
		agents, ok := settings["agent"].(map[string]any)
		if !ok {
			return Result{}, nil
		}
		managed, ok = agents[opencodedefault.ManagedAgent].(map[string]any)
		if !ok {
			return Result{}, nil
		}
	}
	prompt, ok := managed[field].(string)
	if !ok {
		return Result{}, fmt.Errorf("OpenCode managed orchestrator %s is not a string", field)
	}
	start, end := strings.Index(prompt, backgroundStart), strings.Index(prompt, backgroundEnd)
	if (start < 0) != (end < 0) || strings.Count(prompt, backgroundStart) > 1 || strings.Count(prompt, backgroundEnd) > 1 || (start >= 0 && end <= start) {
		return Result{}, fmt.Errorf("invalid managed OpenCode background policy markers")
	}
	if start < 0 && !enabled {
		return Result{}, nil
	}
	if start >= 0 {
		prompt = strings.TrimRight(prompt[:start], "\n") + prompt[end+len(backgroundEnd):]
	}
	if enabled {
		policy := strings.TrimSpace(assets.MustRead("opencode/background-subagents.md"))
		if !strings.HasPrefix(policy, backgroundStart) || !strings.HasSuffix(policy, backgroundEnd) {
			return Result{}, fmt.Errorf("invalid embedded OpenCode background policy")
		}
		prompt = strings.TrimRight(prompt, "\n") + "\n\n" + policy + "\n"
	}
	managed[field] = prompt
	overlay, err := filemerge.MergeJSONObjectsForPath(settingsPath, raw, mustBackgroundOverlay(managed, field, useNative))
	if err != nil {
		return Result{}, err
	}
	written, err := filemerge.WriteFileAtomic(settingsPath, overlay, filemerge.ExistingFileMode(settingsPath, 0o644))
	if err != nil {
		return Result{}, err
	}
	return Result{Changed: written.Changed, Files: []string{settingsPath}}, nil
}

func mustBackgroundOverlay(managed map[string]any, field string, native bool) []byte {
	// Only the managed prompt field is changed; all other user-owned settings
	// remain intact.
	overlayKey := "agent"
	if native {
		overlayKey = "agents"
	}
	overlay, _ := json.Marshal(map[string]any{overlayKey: map[string]any{opencodedefault.ManagedAgent: map[string]any{field: managed[field]}}})
	return overlay
}
