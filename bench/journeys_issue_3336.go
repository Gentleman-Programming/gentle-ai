package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Adapted from decode2's PR #3676 (60ee7efa): this proves only the #3499
// fresh-sync slice and deliberately does not migrate a preserved prompt (#3500).
func issue3336AssertFreshPreflight(s *Sandbox, observation Observation) error {
	if observation.ExitCode != 0 {
		return fmt.Errorf("fresh OpenCode sync failed: %s", firstLine(observation.Stderr))
	}
	content, err := os.ReadFile(filepath.Join(s.Home, ".config", "opencode", "opencode.json"))
	if err != nil {
		return err
	}
	var settings struct {
		Agent map[string]struct {
			Prompt string `json:"prompt"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(content, &settings); err != nil {
		return err
	}
	prompt := settings.Agent["gentle-orchestrator"].Prompt
	for _, want := range []string{"<!-- gentle-ai:sdd-session-preflight -->", "<!-- /gentle-ai:sdd-session-preflight -->", "Both -> `hybrid`", "fixed at 400 changed lines"} {
		if strings.Count(prompt, want) != 1 {
			return fmt.Errorf("generated prompt count for %q is not one", want)
		}
	}
	if strings.Index(prompt, "<!-- /gentle-ai:sdd-session-preflight -->") > strings.Index(prompt, "### SDD Init Guard (MANDATORY)") ||
		strings.Contains(prompt, "Both -> `both`") || strings.Contains(prompt, "Review: 400 lines, 800 lines, Other") {
		return fmt.Errorf("generated prompt violated canonical preflight placement or policy")
	}
	return nil
}

func issue3336Journeys() []Journey {
	return []Journey{{
		ID: "j3336-opencode-sdd-fresh-default-preflight", Title: "Fresh OpenCode SDD sync composes canonical session preflight",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/3499",
		Steps: []Step{{Name: "fixture: fresh repository", Fixture: baseRepo}, {
			Name: "public OpenCode SDD sync", Requires: &Capability{Verb: []string{"sync"}, Flags: []string{"--agents", "--sdd-mode", "--sdd-profile-strategy"}},
			Args: func(*Sandbox) ([]string, error) {
				return []string{"sync", "--agents", "opencode", "--sdd-mode", "multi", "--sdd-profile-strategy", "external-single-active"}, nil
			}, After: issue3336AssertFreshPreflight,
		}},
	}}
}
