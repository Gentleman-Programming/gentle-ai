package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// The step retires settings and prompts in one run, settings first. When it
// keeps an entry it cannot prove it wrote, it must tell prompt retirement
// which files that entry still loads: the runtime refuses to start on a
// {file:} reference that resolves to nothing (#5623).
func TestRetiredOpenCodeSDDSettingsStepKeepsPromptOfPreservedEntry(t *testing.T) {
	config := t.TempDir()
	prompts := filepath.Join(config, "prompts", "sdd")
	if err := os.MkdirAll(prompts, 0o755); err != nil {
		t.Fatal(err)
	}
	// The released render: a prompt some release wrote, so retirement deletes
	// it unless the entry that loads it is kept (see the legacyassets golden
	// corpus for the same fixture).
	released, err := os.ReadFile(filepath.Join("..", "components", "legacyassets", "testdata", "v3.7.0", "opencode-prompt-sdd-apply.md"))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(prompts, "sdd-apply.md")
	if err := os.WriteFile(file, released, 0o644); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(config, "opencode.json")
	// `options` is a field no release wrote, so this entry cannot be proven
	// Gentle AI's and the step keeps it, prompt reference included.
	document := `{
  "agent": {
    "sdd-apply": {
      "mode": "subagent",
      "hidden": true,
      "description": "Implement one SDD phase's work",
      "prompt": "{file:./prompts/sdd/sdd-apply.md}",
      "model": "github-copilot/claude-opus-4.6",
      "options": {"reasoningEffort": "high"}
    }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}

	state := &runtimeState{}
	step := retiredOpenCodeSDDSettingsStep{
		id: "test:retire-sdd-settings",
		target: retiredOpenCodeSDDTarget{
			agent:    model.AgentOpenCode,
			settings: settingsPath,
			prompts:  prompts,
		},
		state: state,
	}
	if err := step.Run(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(file); err != nil {
		t.Fatalf("step deleted a prompt the entry it kept still loads: %v", err)
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "sdd-apply") {
		t.Fatalf("kept entry removed:\n%s", data)
	}
	want := file
	found := false
	for _, action := range state.retiredSDDActions {
		if strings.Contains(action, want) && strings.Contains(action, "still loads it") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no action reports the kept prompt %s: %v", want, state.retiredSDDActions)
	}
}
