package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const issue2138OpenCodeSettings = ".config/opencode/opencode.json"

// The retired general/explore fallback was SDD-owned. The retained OpenCode
// install instead supplies provider-issued, hidden read-only review roles.
func issue2138Journeys() []Journey {
	return []Journey{{
		ID: "j2138-opencode-review-role-boundary", Review: reviewUntouched,
		Title:  "Zero-config OpenCode install bounds provider-issued review roles",
		Source: "#2138: current OpenCode routing installs review-refuter and review-validator, not SDD general/explore fallbacks",
		Steps: []Step{
			{Name: "fixture: zero-config OpenCode runtime", Fixture: issue2138ZeroConfigFixture},
			{Name: "public persona install", Requires: &Capability{Verb: []string{"install"}, Flags: []string{"--agent", "--component", "--scope"}}, Args: func(*Sandbox) ([]string, error) {
				return []string{"install", "--agent", "opencode", "--component", "persona", "--scope", "global"}, nil
			}, After: issue2138AssertReviewRoles},
		},
	}}
}

func issue2138ZeroConfigFixture(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(sandbox.Home, issue2138OpenCodeSettings)); !os.IsNotExist(err) {
		return fmt.Errorf("zero-config fixture found existing OpenCode settings: %v", err)
	}
	return nil
}

func issue2138AssertReviewRoles(sandbox *Sandbox, observation Observation) error {
	if observation.ExitCode != 0 {
		return fmt.Errorf("public persona install failed: stdout=%s stderr=%s", observation.Stdout, observation.Stderr)
	}
	content, err := os.ReadFile(filepath.Join(sandbox.Home, issue2138OpenCodeSettings))
	if err != nil {
		return fmt.Errorf("read installed OpenCode settings: %w", err)
	}
	var settings struct {
		Agent map[string]struct {
			Mode, Prompt string
			Hidden       bool
			Permission   map[string]any
		} `json:"agent"`
	}
	if err := json.Unmarshal(content, &settings); err != nil {
		return fmt.Errorf("parse installed OpenCode settings: %w", err)
	}
	for _, name := range []string{"review-refuter", "review-validator"} {
		role, ok := settings.Agent[name]
		if !ok || role.Mode != "subagent" || !role.Hidden || strings.TrimSpace(role.Prompt) == "" {
			return fmt.Errorf("installed review role %s missing hidden subagent prompt", name)
		}
		for _, capability := range []string{"write", "edit", "task"} {
			if role.Permission[capability] != "deny" {
				return fmt.Errorf("role %s permits %s: %v", name, capability, role.Permission[capability])
			}
		}
		if !strings.Contains(role.Prompt, "frozen candidate") || !strings.Contains(role.Prompt, "Do not edit files or delegate") {
			return fmt.Errorf("role %s lost provider-bound read-only prompt", name)
		}
		if name == "review-validator" {
			bash, ok := role.Permission["bash"].(map[string]any)
			if !ok || bash["gentle-ai review inspect-candidate --purpose targeted-validation *"] != "allow" || bash["*"] != "deny" {
				return fmt.Errorf("validator bash boundary = %v", role.Permission["bash"])
			}
		}
	}
	return nil
}
