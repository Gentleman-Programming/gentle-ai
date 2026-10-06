package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	opencodeactivation "github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
)

// Issue #5035: every writer of the OpenCode routing step (legacy agent
// migration, strict TDD retirement, background policy, review provider roles,
// ODD parity agents, default_agent, share) rewrites opencode.jsonc through the
// JSONC-preserving merge, so user comments and trailing commas outside the
// touched values survive a full install and a following sync.
func TestOpenCodeRoutingWritersPreserveJSONCThroughInstallAndSync(t *testing.T) {
	for _, version := range []string{"1.18.30", "2.0.23"} {
		t.Run(version, func(t *testing.T) {
			home := t.TempDir()
			setOpenCodeTestHome(t, home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
			t.Setenv("OPENCODE_CONFIG_DIR", "")
			t.Setenv("DO_NOT_TRACK", "1")
			old := opencodeactivation.VersionRunnerOverride
			t.Cleanup(func() { opencodeactivation.VersionRunnerOverride = old })
			opencodeactivation.VersionRunnerOverride = func(context.Context, opencodeactivation.Command) (opencodeactivation.CommandOutput, error) {
				return opencodeactivation.CommandOutput{Stdout: []byte(version)}, nil
			}
			configDir := filepath.Join(home, "xdg", "opencode")
			if strings.HasPrefix(version, "2.") {
				mustWriteFile(t, filepath.Join(configDir, "node_modules", "@opencode", "plugin", "package.json"), []byte(`{"version":"2.0.4"}`))
			}

			// The seeded prompt carries a retired strict TDD section and a
			// background policy block, so both retirements rewrite the document;
			// the v3.7.0 marker on gentleman drives the legacy agent migration.
			prompt := "# User orchestrator notes\n\n<!-- gentle-ai:strict-tdd-mode -->\nStrict TDD Mode: enabled\n<!-- /gentle-ai:strict-tdd-mode -->\n\n<!-- gentle-ai:opencode-background-subagents -->\nold policy\n<!-- /gentle-ai:opencode-background-subagents -->\n"
			promptJSON, err := json.Marshal(prompt)
			if err != nil {
				t.Fatal(err)
			}
			settingsPath := filepath.Join(configDir, "opencode.jsonc")
			before := "// user header note\n{\n  // user provider note\n  \"provider\": {\n    \"local\": {\"models\": {\"m\": {},},},\n  },\n  /* user theme note */\n  \"theme\": \"default\",\n  \"agent\": {\n    \"gentle-orchestrator\": {\"prompt\": " + string(promptJSON) + "},\n    \"gentleman\": {\"__managed_by\": \"gentle-ai/sdd\", \"model\": \"local/m\", \"prompt\": \"old\"},\n  },\n  // user trailing note\n}\n"
			mustWriteFile(t, settingsPath, []byte(before))

			rt := newTestInstallRuntime(t, home, model.Selection{Agents: []model.AgentID{model.AgentOpenCode}})
			if result := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(rt.stagePlan()); result.Err != nil {
				t.Fatalf("install: %v", result.Err)
			}
			assertOpenCodeJSONCSurvived(t, settingsPath, "install")

			if _, err := RunSyncWithSelection(home, model.Selection{Agents: []model.AgentID{model.AgentOpenCode}}); err != nil {
				t.Fatalf("sync: %v", err)
			}
			assertOpenCodeJSONCSurvived(t, settingsPath, "sync")
		})
	}
}

func assertOpenCodeJSONCSurvived(t *testing.T, settingsPath, phase string) {
	t.Helper()
	after := readTextFile(t, settingsPath)
	for _, want := range []string{
		"// user header note",
		"// user provider note",
		`"m": {},`,
		"/* user theme note */",
		`"theme": "default",`,
		"// user trailing note",
	} {
		if !strings.Contains(after, want) {
			t.Fatalf("%s destroyed JSONC comment or trailing comma %q:\n%s", phase, want, after)
		}
	}
	settings, err := filemerge.UnmarshalJSONObject([]byte(after))
	if err != nil {
		t.Fatalf("%s left unparseable settings: %v\n%s", phase, err, after)
	}
	if settings["default_agent"] != "gentle-orchestrator" || settings["share"] != "disabled" {
		t.Fatalf("%s skipped default_agent/share: %v %v", phase, settings["default_agent"], settings["share"])
	}
	agents := settings["agent"].(map[string]any)
	orchestrator := agents["gentle-orchestrator"].(map[string]any)
	gotPrompt, _ := orchestrator["prompt"].(string)
	if strings.Contains(gotPrompt, "strict-tdd-mode") || strings.Contains(gotPrompt, "old policy") || !strings.Contains(gotPrompt, "# User orchestrator notes") {
		t.Fatalf("%s left retired prompt sections or lost user notes:\n%s", phase, gotPrompt)
	}
	if gentleman, _ := agents["gentleman"].(map[string]any); gentleman["__managed_by"] != nil || gentleman["model"] != "local/m" {
		t.Fatalf("%s did not migrate the v3.7.0 gentleman agent: %#v", phase, gentleman)
	}
	if _, ok := agents["review-validator"]; !ok {
		t.Fatalf("%s skipped review provider roles: %#v", phase, agents)
	}
}

// The routing step rewrites default_agent with the JSONC-preserving merge, which
// refuses an escaped spelling of, or comments inside, that value. The prepare
// gate must refuse the same document before any managed file is mutated,
// instead of failing midway through apply.
func TestOpenCodePrepareRefusesUnsafeDefaultAgentBeforeAnyMutation(t *testing.T) {
	for _, tc := range []struct{ name, content string }{
		{"escaped default_agent", `{"default\u005fagent": "build", "agent": {}}`},
		{"comment inside default_agent", `{"default_agent": /* mine */ "build", "agent": {}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			setOpenCodeTestHome(t, home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
			t.Setenv("OPENCODE_CONFIG_DIR", "")
			settingsPath := filepath.Join(home, "xdg", "opencode", "opencode.jsonc")
			mustWriteFile(t, settingsPath, []byte(tc.content))

			rt := newTestInstallRuntime(t, home, model.Selection{Agents: []model.AgentID{model.AgentOpenCode}})
			result := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(rt.stagePlan())
			if result.Err == nil {
				t.Fatal("install accepted unsafe default_agent")
			}
			assertInstallPrepareRefusal(t, result, rt.backupRoot, home, settingsPath, tc.content)

			sync, err := newSyncRuntimeWithScope(home, model.Selection{Agents: []model.AgentID{model.AgentOpenCode}}, ScopeGlobal)
			if err != nil {
				t.Fatal(err)
			}
			gate := sync.stagePlan().Prepare[0]
			if gate.ID() != "prepare:opencode-settings-validation" {
				t.Fatalf("first sync prepare step = %q, want the settings validation", gate.ID())
			}
			if err := gate.Run(); err == nil {
				t.Fatal("sync validation accepted unsafe default_agent")
			}
		})
	}
}

// A workspace sync skips the routing step, so persisted model assignments are
// its only agent writer and nothing writes default_agent: an unsafe
// default_agent there must not block it.
func TestWorkspaceSyncWithModelAssignmentsIgnoresUnsafeDefaultAgent(t *testing.T) {
	home := t.TempDir()
	setOpenCodeTestHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(workspace)
	mustWriteFile(t, filepath.Join(workspace, "opencode.jsonc"), []byte(`{"default_agent": /* mine */ "build", "agent": {}}`))

	selection := model.Selection{
		Agents:           []model.AgentID{model.AgentOpenCode},
		ModelAssignments: map[string]model.ModelAssignment{"gentle-orchestrator": {ProviderID: "anthropic", ModelID: "claude-sonnet-5"}},
	}
	rt, err := newSyncRuntimeWithScope(home, selection, ScopeWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	gate := rt.stagePlan().Prepare[0]
	if gate.ID() != "prepare:opencode-settings-validation" {
		t.Fatalf("first prepare step = %q, want the settings validation", gate.ID())
	}
	if err := gate.Run(); err != nil {
		t.Fatalf("workspace sync refused default_agent that no writer touches: %v", err)
	}
}
