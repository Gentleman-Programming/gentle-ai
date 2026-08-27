package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func issue3336AssertGeneratedPrompt(s *Sandbox, observation Observation) error {
	content, readErr := os.ReadFile(s.Home + "/.config/opencode/opencode.json")
	var settings map[string]any
	parseErr := json.Unmarshal(content, &settings)
	agents, _ := settings["agent"].(map[string]any)
	orchestrator, _ := agents["gentle-orchestrator"].(map[string]any)
	prompt, shapeOK := orchestrator["prompt"].(string)
	if observation.ExitCode != 0 || readErr != nil || parseErr != nil || !shapeOK || strings.Count(prompt, "<!-- gentle-ai:sdd-session-preflight -->") != 1 || strings.Count(prompt, "<!-- /gentle-ai:sdd-session-preflight -->") != 1 || !strings.Contains(prompt, "1. **Pace**") || !strings.Contains(prompt, "2. **Artifacts**") || !strings.Contains(prompt, "3. **PR strategy**") || !strings.Contains(prompt, "Both -> `hybrid`") || !strings.Contains(prompt, "fixed at 400 changed lines") || strings.Contains(prompt, "Both -> `both`") || strings.Contains(prompt, "4. **Review policy**") || strings.Contains(prompt, "Review: 400 lines") || strings.Contains(prompt, "800 lines") || strings.Contains(prompt, ", Other") || strings.Contains(prompt, "Other ->") || strings.Contains(prompt, "custom review budget") {
		return fmt.Errorf("sync failed or generated prompt violated canonical preflight")
	}
	return nil
}
func issue3336Journeys() []Journey {
	return []Journey{{ID: "j3336-opencode-sdd-fresh-default-preflight", Review: reviewUntouched, Title: "Fresh OpenCode SDD sync composes canonical session preflight", Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/3336", Steps: []Step{{Name: "fixture: fresh repository", Fixture: baseRepo}, {Name: "public OpenCode SDD sync", Requires: &Capability{Verb: []string{"sync"}, Flags: []string{"--agents", "--sdd-mode", "--sdd-profile-strategy"}}, Args: func(*Sandbox) ([]string, error) {
		return []string{"sync", "--agents", "opencode", "--sdd-mode", "multi", "--sdd-profile-strategy", "external-single-active"}, nil
	}, After: issue3336AssertGeneratedPrompt}}}}
}
