package agentguidance

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/opencodedefault"
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
	agents, ok := settings["agent"].(map[string]any)
	if !ok {
		return Result{}, nil
	}
	managed, ok := agents[opencodedefault.ManagedAgent].(map[string]any)
	if !ok {
		return Result{}, nil
	}
	prompt, ok := managed["prompt"].(string)
	if !ok {
		return Result{}, fmt.Errorf("OpenCode managed orchestrator prompt is not a string")
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
	managed["prompt"] = prompt
	overlay, err := filemerge.MergeJSONObjects(raw, mustBackgroundOverlay(managed))
	if err != nil {
		return Result{}, err
	}
	written, err := filemerge.WriteFileAtomic(settingsPath, overlay, 0o644)
	if err != nil {
		return Result{}, err
	}
	return Result{Changed: written.Changed, Files: []string{settingsPath}}, nil
}

func mustBackgroundOverlay(managed map[string]any) []byte {
	// Only the prompt is changed; all other user-owned settings remain intact.
	prompt := managed["prompt"].(string)
	overlay, _ := json.Marshal(map[string]any{"agent": map[string]any{opencodedefault.ManagedAgent: map[string]any{"prompt": prompt}}})
	return overlay
}
