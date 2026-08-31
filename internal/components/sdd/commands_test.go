package sdd

import (
	"encoding/json"
	"reflect"
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

func TestOpenCodeReviewValidatorPermissionContract(t *testing.T) {
	// The validator processes untrusted candidate input, so its shell surface
	// is pinned to the one provider-issued read-only inspection command its
	// briefing names (reviewerprovider.targetedValidatorPromptInstruction);
	// every other bash invocation is denied by the wildcard.
	wantPermission := map[string]any{
		"write": "deny",
		"edit":  "deny",
		"task":  "deny",
		"bash": map[string]any{
			"gentle-ai review inspect-candidate --purpose targeted-validation *": "allow",
			"*": "deny",
		},
	}
	for _, path := range []string{"opencode/sdd-overlay-single.json", "opencode/sdd-overlay-multi.json"} {
		t.Run(path, func(t *testing.T) {
			var root map[string]any
			if err := json.Unmarshal([]byte(assets.MustRead(path)), &root); err != nil {
				t.Fatalf("unmarshal %s: %v", path, err)
			}
			validator := root["agent"].(map[string]any)["review-validator"].(map[string]any)
			if _, exists := validator["tools"]; exists {
				t.Fatalf("%s validator emits deprecated tools: %#v", path, validator)
			}
			permission, ok := validator["permission"].(map[string]any)
			if !ok || !reflect.DeepEqual(permission, wantPermission) {
				t.Fatalf("%s validator permission = %#v, want %#v", path, validator["permission"], wantPermission)
			}
			if _, exists := permission["read"]; exists {
				t.Fatalf("%s validator overrides global read policy: %#v", path, permission)
			}
		})
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
			if _, exists := research["tools"]; exists {
				t.Fatalf("%s research agent emits deprecated tools: %#v", path, research)
			}
			researchPermission, ok := research["permission"].(map[string]any)
			if !ok {
				t.Fatalf("%s research permission = %#v, want object", path, research["permission"])
			}
			for _, denied := range []string{"bash", "webfetch", "websearch", "task"} {
				if got := researchPermission[denied]; got != "deny" {
					t.Fatalf("%s research permission %q = %v, want deny", path, denied, got)
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
