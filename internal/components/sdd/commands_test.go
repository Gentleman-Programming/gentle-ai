package sdd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
)

func TestOpenCodeCommandsIncludesCoreWorkflow(t *testing.T) {
	commands := OpenCodeCommands()
	if len(commands) != 11 {
		t.Fatalf("OpenCodeCommands() length = %d", len(commands))
	}

	if commands[0].Name != "sdd-init" {
		t.Fatalf("first command = %q", commands[0].Name)
	}

	seen := map[string]bool{}
	for _, command := range commands {
		seen[command.Name] = true
	}

	for _, name := range []string{
		"sdd-init", "sdd-new", "sdd-continue", "sdd-status", "sdd-explore", "sdd-research", "sdd-ff",
		"sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
	} {
		if !seen[name] {
			t.Fatalf("OpenCodeCommands() missing %q", name)
		}
	}
	if commands[5].Name != "sdd-research" {
		t.Fatalf("command after sdd-explore = %q, want sdd-research", commands[5].Name)
	}
}

func TestOpenCodeResearchCommandHasExplicitTaskPermissionAndDefaultDenial(t *testing.T) {
	command := assets.MustRead("opencode/commands/sdd-research.md")
	for _, required := range []string{"agent: gentle-orchestrator", "hidden `sdd-research` sub-agent", "SDD Session Preflight must already be complete"} {
		if !strings.Contains(command, required) {
			t.Fatalf("OpenCode research command missing %q", required)
		}
	}

	for _, path := range []string{"opencode/sdd-overlay-single.json", "opencode/sdd-overlay-multi.json"} {
		t.Run(path, func(t *testing.T) {
			var root map[string]any
			if err := json.Unmarshal([]byte(assets.MustRead(path)), &root); err != nil {
				t.Fatalf("unmarshal %s: %v", path, err)
			}
			agents := root["agent"].(map[string]any)
			orchestrator := agents["gentle-orchestrator"].(map[string]any)
			permission := orchestrator["permission"].(map[string]any)
			tasks := permission["task"].(map[string]any)["__replace__"].(map[string]any)
			if got := tasks["sdd-research"]; got != "allow" {
				t.Fatalf("%s sdd-research task permission = %v, want allow", path, got)
			}

			research, ok := agents["sdd-research"].(map[string]any)
			if !ok || research["mode"] != "subagent" || research["hidden"] != true {
				t.Fatalf("%s missing hidden sdd-research subagent", path)
			}
			tools := research["tools"].(map[string]any)
			for _, denied := range []string{"webfetch", "websearch", "task"} {
				if got := tools[denied]; got != false {
					t.Fatalf("%s research tool %q = %v, want explicit false", path, denied, got)
				}
			}
			prompt := research["prompt"].(string)
			if prompt == "__PROMPT_FILE_sdd-research__" {
				prompt = assets.MustRead("skills/sdd-research/SKILL.md")
			}
			for _, required := range []string{
				"Evidence grants: documentation=[]; open-web=[].",
				"Persistence tools are not evidence grants.",
				"Unsupported or undeclared classes deny admission and emit no claims.",
			} {
				if !strings.Contains(prompt, required) {
					t.Fatalf("%s research prompt missing %q", path, required)
				}
			}
		})
	}
}
