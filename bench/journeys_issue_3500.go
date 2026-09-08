package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const issue3500ExternalPrompt = "EXTERNAL_3500_PREFIX\nKeep these external policy bytes exactly.\nEXTERNAL_3500_SUFFIX"

func issue3500SeedExternalOpenCodePrompt(sandbox *Sandbox) error {
	path := filepath.Join(sandbox.Home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	settings := map[string]any{"agent": map[string]any{"gentle-orchestrator": map[string]any{"prompt": issue3500ExternalPrompt}}}
	content, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func issue3500PreservedPrompt(sandbox *Sandbox) (string, error) {
	content, err := os.ReadFile(filepath.Join(sandbox.Home, ".config", "opencode", "opencode.json"))
	if err != nil {
		return "", err
	}
	var settings struct {
		Agent map[string]struct {
			Prompt string `json:"prompt"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(content, &settings); err != nil {
		return "", err
	}
	return settings.Agent["gentle-orchestrator"].Prompt, nil
}

func issue3500AssertFirstSync(sandbox *Sandbox, observation Observation) error {
	prompt, err := issue3500PreservedPrompt(sandbox)
	if err != nil || observation.ExitCode != 0 || !strings.Contains(prompt, issue3500ExternalPrompt) ||
		strings.Count(prompt, "<!-- gentle-ai:sdd-session-preflight -->") != 1 ||
		strings.Count(prompt, "<!-- /gentle-ai:sdd-session-preflight -->") != 1 ||
		!strings.Contains(prompt, "Both -> `hybrid`") || strings.Contains(prompt, "Both -> `both`") {
		return fmt.Errorf("external OpenCode sync did not preserve user bytes and compose one canonical hybrid preflight: %q (%v)", prompt, err)
	}
	sandbox.Scratch["issue3500-first-prompt"] = prompt
	return nil
}

func issue3500AssertSecondSync(sandbox *Sandbox, observation Observation) error {
	prompt, err := issue3500PreservedPrompt(sandbox)
	if err != nil || observation.ExitCode != 0 || prompt != sandbox.Scratch["issue3500-first-prompt"] {
		return fmt.Errorf("second external OpenCode sync changed the preserved prompt: %q (%v)", prompt, err)
	}
	return nil
}

func issue3500Journeys() []Journey {
	return []Journey{{
		ID:     "j3500-preserved-external-opencode-sync",
		Review: reviewUntouched,
		Title:  "#3500: external OpenCode sync preserves user prompt bytes and canonicalizes preflight once",
		Source: "#3500: the external-single-active public sync owns only the canonical preflight block",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "fixture: external OpenCode prompt", Fixture: issue3500SeedExternalOpenCodePrompt},
			{Name: "public external OpenCode sync", Requires: issue3500SyncCapability, Args: issue3500SyncArgs, After: issue3500AssertFirstSync},
			{Name: "second public external OpenCode sync is idempotent", Requires: issue3500SyncCapability, Args: issue3500SyncArgs, After: issue3500AssertSecondSync},
		},
	}}
}

var issue3500SyncCapability = &Capability{Verb: []string{"sync"}, Flags: []string{"--agents", "--sdd-mode", "--sdd-profile-strategy"}}

func issue3500SyncArgs(*Sandbox) ([]string, error) {
	return []string{"sync", "--agents", "opencode", "--sdd-mode", "multi", "--sdd-profile-strategy", "external-single-active"}, nil
}
