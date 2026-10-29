package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	opencodeagent "github.com/gentleman-programming/gentle-ai/v4/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func TestSyncKeepsRetiredPromptReferencedByCustomizedAgent(t *testing.T) {
	for _, version := range openCodeRuntimeVersions {
		t.Run(version, func(t *testing.T) {
			home := t.TempDir()
			setSyncTestHome(t, home)
			setOpenCodeTestHome(t, home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			t.Setenv("OPENCODE_CONFIG_DIR", "")
			stubOpenCodeRuntimeVersion(t, home, version)
			config := opencodeagent.ConfigPath(home)
			settings := filepath.Join(config, "opencode.json")
			prompt := filepath.Join(config, "prompts", "sdd", "sdd-apply.md")
			originalPrompt := releasedSDDRender(t, "opencode-prompt-sdd-apply.md")
			t.Chdir(t.TempDir())
			custom := map[string]any{
				"mode": "subagent", "hidden": true,
				"description": "Execute implementation tasks",
				"model":       "github-copilot/claude-opus-4.6",
				"options":     map[string]any{"temperature": json.Number("0.2")},
				"prompt":      "{file:" + filepath.ToSlash(prompt) + "}",
			}
			data, err := json.Marshal(map[string]any{"agent": map[string]any{"sdd-apply": custom}})
			if err != nil {
				t.Fatal(err)
			}
			mustWriteFile(t, settings, data)
			mustWriteFile(t, prompt, originalPrompt)
			selection := model.Selection{Agents: []model.AgentID{model.AgentOpenCode}}
			result, err := RunSyncWithSelection(home, selection)
			if err != nil {
				t.Fatal(err)
			}
			agents, _ := openCodeSettingsAgents(t, settings)
			if !reflect.DeepEqual(agents["sdd-apply"], custom) {
				t.Fatalf("custom agent changed: %#v", agents["sdd-apply"])
			}
			if got := readTextFile(t, prompt); got != string(originalPrompt) {
				t.Fatal("referenced prompt changed")
			}
			if !hasManualAction(result.ManualActions, prompt, "referenced") {
				t.Errorf("missing referenced-prompt notice: %v", result.ManualActions)
			}
			before := readTextFile(t, settings)
			again, err := RunSyncWithSelection(home, selection)
			if err != nil {
				t.Fatal(err)
			}
			if readTextFile(t, settings) != before || !again.NoOp {
				t.Fatalf("repeat sync was not a no-op: %+v", again.ChangedFiles)
			}
		})
	}
}

func TestSyncRejectsDanglingOpenCodeFileReferenceAndRollsBack(t *testing.T) {
	home := t.TempDir()
	setSyncTestHome(t, home)
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	stubOpenCodeRuntimeVersion(t, home, openCodeRuntimeVersions[0])
	t.Chdir(t.TempDir())
	config := opencodeagent.ConfigPath(home)
	settings := filepath.Join(config, "opencode.json")
	missing := filepath.Join(config, "missing.md")
	original := []byte(`{"agent":{"my-helper":{"prompt":"Before {file:./missing.md} after"}}}`)
	mustWriteFile(t, settings, original)
	_, err := RunSyncWithSelection(home, model.Selection{Agents: []model.AgentID{model.AgentOpenCode}})
	if err == nil {
		t.Fatal("sync accepted a dangling file reference")
	}
	if !strings.Contains(err.Error(), missing) || !strings.Contains(err.Error(), "file reference") {
		t.Fatalf("error does not identify broken reference: %v", err)
	}
	if got := readTextFile(t, settings); got != string(original) {
		t.Fatalf("failed sync changed original settings: %s", got)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("failed sync created missing target: %v", err)
	}
}
