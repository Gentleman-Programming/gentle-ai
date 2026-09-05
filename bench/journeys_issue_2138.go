package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const issue2138OpenCodeSettings = ".config/opencode/opencode.json"

func issue2138Journeys() []Journey {
	return []Journey{{
		ID:     "j2138-opencode-native-fallback-boundary",
		Review: reviewUntouched,
		Title:  "Zero-config OpenCode install emits bounded native fallback agents",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/2138",
		Steps: []Step{
			{Name: "fixture: zero-config OpenCode runtime", Fixture: issue2138ZeroConfigFixture},
			{Name: "public multi-mode install", Requires: issue2138InstallCapability, Args: issue2138InstallArgs("multi"), After: issue2138AssertGeneratedFallbacks("multi")},
			{Name: "fixture: restore zero-config state", Fixture: issue2138ResetConfig},
			{Name: "public single-mode install", Requires: issue2138InstallCapability, Args: issue2138InstallArgs("single"), After: issue2138AssertGeneratedFallbacks("single")},
		},
	}}
}

var issue2138InstallCapability = &Capability{
	Verb:  []string{"install"},
	Flags: []string{"--agent", "--component", "--sdd-mode", "--scope"},
}

func issue2138InstallArgs(mode string) func(*Sandbox) ([]string, error) {
	return func(*Sandbox) ([]string, error) {
		return []string{"install", "--agent", "opencode", "--component", "sdd", "--sdd-mode", mode, "--scope", "global"}, nil
	}
}

func issue2138ZeroConfigFixture(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	binDir := filepath.Join(sandbox.Root, "opencode-bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	binaryName := "opencode"
	content := "#!/bin/sh\nexit 0\n"
	if runtime.GOOS == "windows" {
		binaryName = "opencode.exe"
		content = "@echo off\r\nexit /b 0\r\n"
	}
	path := filepath.Join(binDir, binaryName)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o755); err != nil {
		return err
	}
	sandbox.PathOverride = binDir
	if _, err := os.Stat(filepath.Join(sandbox.Home, issue2138OpenCodeSettings)); !os.IsNotExist(err) {
		return fmt.Errorf("zero-config fixture found existing OpenCode settings: %v", err)
	}
	return nil
}

func issue2138ResetConfig(sandbox *Sandbox) error {
	return os.RemoveAll(filepath.Dir(filepath.Join(sandbox.Home, issue2138OpenCodeSettings)))
}

func issue2138AssertGeneratedFallbacks(mode string) func(*Sandbox, Observation) error {
	return func(sandbox *Sandbox, observation Observation) error {
		if observation.ExitCode != 0 {
			return fmt.Errorf("public %s install failed: stdout=%s stderr=%s", mode, observation.Stdout, observation.Stderr)
		}
		content, err := os.ReadFile(filepath.Join(sandbox.Home, issue2138OpenCodeSettings))
		if err != nil {
			return fmt.Errorf("read generated %s OpenCode settings: %w", mode, err)
		}
		var root map[string]any
		if err := json.Unmarshal(content, &root); err != nil {
			return fmt.Errorf("parse generated %s OpenCode settings: %w", mode, err)
		}
		agents, ok := root["agent"].(map[string]any)
		if !ok {
			return fmt.Errorf("generated %s OpenCode settings have no agent object", mode)
		}
		// Fallback agents use permission boundaries so global sensitive-path read
		// rules remain authoritative without deprecated agent-local tools maps.
		wantPermissions := map[string]map[string]string{
			"general": {"task": "deny"},
			"explore": {"write": "deny", "edit": "deny", "bash": "deny", "task": "deny"},
		}
		for name, permissionsWant := range wantPermissions {
			raw, ok := agents[name].(map[string]any)
			if !ok {
				return fmt.Errorf("generated %s settings omit fallback agent %q", mode, name)
			}
			if raw["mode"] != "subagent" || raw["hidden"] != true {
				return fmt.Errorf("generated %s fallback agent %q has mode=%v hidden=%v", mode, name, raw["mode"], raw["hidden"])
			}
			prompt, _ := raw["prompt"].(string)
			if strings.TrimSpace(prompt) == "" {
				return fmt.Errorf("generated %s fallback agent %q has an empty prompt", mode, name)
			}
			if name == "general" && (!strings.Contains(prompt, "empirical verification") || !strings.Contains(prompt, "Do NOT launch child sub-agents")) {
				return fmt.Errorf("generated %s general prompt lost its auxiliary-task boundary", mode)
			}
			if name == "explore" && (!strings.Contains(prompt, "gentle-pi") || !strings.Contains(prompt, "Do not create, edit, or delete files")) {
				return fmt.Errorf("generated %s explore prompt lost its read-only boundary", mode)
			}
			description, _ := raw["description"].(string)
			if name == "explore" && strings.Contains(strings.ToLower(description+" "+prompt), "web search") {
				return fmt.Errorf("generated %s explore fallback advertises unavailable web search", mode)
			}
			if _, exists := raw["tools"]; exists {
				return fmt.Errorf("generated %s fallback agent %q emits deprecated tools", mode, name)
			}
			permissions, ok := raw["permission"].(map[string]any)
			if !ok {
				return fmt.Errorf("generated %s fallback agent %q has no permission boundary", mode, name)
			}
			if len(permissions) != len(permissionsWant) {
				return fmt.Errorf("generated %s fallback agent %q permissions=%v, want %v", mode, name, permissions, permissionsWant)
			}
			for permission, want := range permissionsWant {
				got, exists := permissions[permission].(string)
				if !exists || got != want {
					return fmt.Errorf("generated %s fallback agent %q permission %s=%v, want %s", mode, name, permission, permissions[permission], want)
				}
			}
		}
		return nil
	}
}
