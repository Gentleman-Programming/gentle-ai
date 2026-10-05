package agentguidance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/opencodedefault"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	opencoderuntime "github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
)

func TestCodexODDRoutingInjectionPreservesUserTextAndResync(t *testing.T) {
	home := t.TempDir()
	paths, err := RoutingPaths(home, model.AgentCodex)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths[0]), 0o755); err != nil {
		t.Fatal(err)
	}
	const personal = "# Personal instructions\nNever deploy.\n"
	if err := os.WriteFile(paths[0], []byte(personal), 0o644); err != nil {
		t.Fatal(err)
	}
	options := RoutingOptions{
		CodexPhaseModelAssignments:  map[string]string{"odd-worker": "gpt-custom-worker"},
		CodexModelAssignments:       map[string]model.CodexEffort{"odd-worker": model.CodexEffortXHigh},
		CodexCarrilModelAssignments: map[string]string{"sdd-cheap": "gpt-custom-cheap", "sdd-strong": "gpt-custom-strong"},
	}
	first, err := InjectRoutingWithOptions(home, model.AgentCodex, options)
	if err != nil || !first.Changed {
		t.Fatalf("initial injection: %+v %v", first, err)
	}
	body, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{personal, "<!-- gentle-ai:agent-routing -->", "| `odd-worker` | `gpt-custom-worker` | `xhigh` |", "| `odd-explorer` | `gpt-custom-cheap` |", "| `odd-verify` | `gpt-custom-strong` |"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("installed guidance missing %q", want)
		}
	}
	second, err := InjectRoutingWithOptions(home, model.AgentCodex, options)
	if err != nil || second.Changed {
		t.Fatalf("identical re-sync: %+v %v", second, err)
	}
	updated := options
	updated.CodexPhaseModelAssignments = map[string]string{"odd-worker": "gpt-new-worker"}
	if _, err := InjectRoutingWithOptions(home, model.AgentCodex, updated); err != nil {
		t.Fatal(err)
	}
	body, err = os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), personal) || !strings.Contains(string(body), "gpt-new-worker") || strings.Contains(string(body), "gpt-custom-worker") {
		t.Fatalf("re-sync failed to preserve personal text and replace managed assignment")
	}
}

func TestClaudeODDRoutingRendersRoleModelTable(t *testing.T) {
	home := t.TempDir()
	options := RoutingOptions{ClaudePhaseAssignments: map[string]model.ClaudePhaseAssignment{
		"odd-worker": {Model: model.ClaudeModelOpus, Effort: model.ClaudeEffortHigh},
	}}
	if _, err := InjectRoutingWithOptions(home, model.AgentClaudeCode, options); err != nil {
		t.Fatal(err)
	}
	paths, err := RoutingPaths(home, model.AgentClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"### Claude ODD worker assignments",
		"Agent tool `model` parameter",
		"| `odd-explorer` | `sonnet` |",
		"| `odd-worker` | `opus` |",
		"| `odd-verify` | `sonnet` |",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("Claude routing missing %q", want)
		}
	}
	if strings.Contains(string(body), "reasoning_effort") || strings.Contains(string(body), "Codex ODD worker assignments") {
		t.Error("Claude routing must not carry the Codex effort table")
	}

	codex, err := InjectRoutingWithOptions(t.TempDir(), model.AgentCodex, options)
	if err != nil || len(codex.Files) == 0 {
		t.Fatalf("codex injection: %+v %v", codex, err)
	}
	codexBody, err := os.ReadFile(codex.Files[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(codexBody), "Claude ODD worker assignments") {
		t.Error("Codex routing gained the Claude table")
	}
}

func TestRemoteAuthorizationSectionPreservesUserText(t *testing.T) {
	const personal = "Personal instructions: do not deploy.\n"
	first := InjectRemoteAuthorization(personal)
	if !strings.HasPrefix(first, personal) || InjectRemoteAuthorization(first) != first {
		t.Fatal("section projection lost user text or was not idempotent")
	}
	routing, err := RenderRouting(model.AgentClaudeCode)
	if err != nil || strings.Contains(routing, "remote-authorization") {
		t.Fatal("routing-only renderer gained the remote boundary")
	}
}

// This compares against the pre-existing merger, NOT native last-match semantics.
// That merger sorts maps; this slice must not claim to fix custom rule ordering.
func TestRemoteAuthorizationRetainsExistingPermissionMergeBehavior(t *testing.T) {
	const seed = `{"permission":{"bash":{"ssh *":"allow","*":"deny"}},"agent":{"gentle-orchestrator":{"prompt":"Personal instructions","permission":{"bash":"deny","task":"deny"}}}}`
	baseline, err := filemerge.MergeJSONObjects([]byte(seed), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	permissions := func(raw []byte) string {
		var root map[string]json.RawMessage
		if err := json.Unmarshal(raw, &root); err != nil {
			t.Fatal(err)
		}
		var agents map[string]map[string]json.RawMessage
		if err := json.Unmarshal(root["agent"], &agents); err != nil {
			t.Fatal(err)
		}
		return string(root["permission"]) + string(agents["gentle-orchestrator"]["permission"])
	}
	for _, agent := range []model.AgentID{model.AgentOpenCode, model.AgentKilocode} {
		t.Run(string(agent), func(t *testing.T) {
			home := t.TempDir()
			paths, err := RoutingPaths(home, agent)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(paths[0]), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(paths[0], []byte(seed), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := InjectRoutingWithOptions(home, agent, RoutingOptions{}); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(paths[0])
			if err != nil || permissions(got) != permissions(baseline) {
				t.Fatalf("permission projection changed relative to existing merger: %v", err)
			}
			if !strings.Contains(deliveredGuidance(t, paths[0]), "Personal instructions") {
				t.Fatal("personal orchestrator instructions lost")
			}
		})
	}
}

func TestRemoteAuthorizationPrimaryCarriers(t *testing.T) {
	covered := 0
	for _, agent := range catalog.AllAgents() {
		if agent.ID == model.AgentPi {
			continue // The install/sync step leaves package-owned Pi prompts untouched.
		}
		if agent.ID == model.AgentConductor {
			continue // Catalog-only: no standalone guidance carrier (see conductor_catalog_only_test.go).
		}
		covered++
		t.Run(string(agent.ID), func(t *testing.T) {
			home := t.TempDir()
			first, err := InjectRoutingWithOptions(home, agent.ID, RoutingOptions{})
			if err != nil {
				t.Fatal(err)
			}
			prompt := deliveredGuidance(t, first.Files[0])
			for _, required := range []string{
				"<!-- gentle-ai:remote-authorization -->",
				"destination", "operation", "credential/session",
				"SSH agents", "ControlMaster", "not a sandbox",
			} {
				if !strings.Contains(prompt, required) {
					t.Errorf("primary carrier missing %q", required)
				}
			}
			second, err := InjectRoutingWithOptions(home, agent.ID, RoutingOptions{})
			if err != nil || second.Changed {
				t.Fatalf("repeat injection = %+v, %v", second, err)
			}
		})
	}
	if covered != 15 {
		t.Fatalf("covered %d non-Pi clients, want 15", covered)
	}
}

func TestInjectRoutingDeliversOnlyODDWorkflow(t *testing.T) {
	t.Parallel()
	for _, agent := range catalog.AllAgents() {
		if agent.ID == model.AgentConductor {
			continue // Catalog-only: no standalone guidance target.
		}
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()
			result, err := InjectRoutingWithOptions(t.TempDir(), agent.ID, RoutingOptions{})
			if err != nil {
				t.Fatal(err)
			}
			block := managedRoutingBlock(deliveredGuidance(t, result.Files[0]))
			if !strings.Contains(block, "### ODD protocol") {
				t.Fatal("installed routing has no ODD protocol")
			}
			if strings.Contains(strings.ToLower(block), "sdd") {
				t.Fatalf("agent %q received an SDD route in installed routing", agent.ID)
			}
		})
	}
}

func TestInjectRoutingInstallsGuidanceForEverySupportedAgent(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		if agent.ID == model.AgentConductor {
			continue // Catalog-only: covered by TestInjectRoutingNoOpsForCatalogOnlyConductor.
		}
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			targetDir := t.TempDir()

			result, err := InjectRoutingWithOptions(targetDir, agent.ID, RoutingOptions{})
			if err != nil {
				t.Fatalf("InjectRouting(%q) error = %v", agent.ID, err)
			}
			if !result.Changed {
				t.Fatalf("InjectRouting(%q) reported no change on a fresh target", agent.ID)
			}
			if len(result.Files) == 0 {
				t.Fatalf("InjectRouting(%q) reported no written files", agent.ID)
			}
			for _, path := range result.Files {
				if !filepath.IsAbs(path) {
					t.Fatalf("InjectRouting(%q) reported non-absolute path %q", agent.ID, path)
				}
				if !strings.HasPrefix(path, targetDir) {
					t.Fatalf("InjectRouting(%q) wrote outside the target dir: %q", agent.ID, path)
				}
			}

			path := result.Files[0]
			// Read the scope the agent actually loads, not merely the bytes on
			// disk: adapters whose guidance lives inside a settings document
			// carry the block as an encoded string, never as raw markdown.
			written := deliveredGuidance(t, path)
			rendered, err := RenderRouting(agent.ID)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
			}
			if !strings.Contains(written, rendered) {
				t.Fatalf("InjectRouting(%q) did not write the rendered guidance:\n%s", agent.ID, written)
			}
			if !strings.Contains(written, "<!-- gentle-ai:"+RoutingSectionID+" -->") {
				t.Fatalf("InjectRouting(%q) did not open the managed section:\n%s", agent.ID, written)
			}
			if !strings.Contains(written, "<!-- /gentle-ai:"+RoutingSectionID+" -->") {
				t.Fatalf("InjectRouting(%q) did not close the managed section:\n%s", agent.ID, written)
			}
			if !strings.Contains(written, "First establish whether the requested outcome explicitly authorizes a change.") {
				t.Fatalf("InjectRouting(%q) did not deliver the outcome-authorization guard:\n%s", agent.ID, written)
			}
		})
	}
}

// TestInjectRoutingStaysContainedUnderHostileEnvironment pins containment
// against ambient config-redirection environment. CI runners (and real user
// shells) export XDG_CONFIG_HOME and APPDATA; an adapter that resolves paths
// from the environment instead of the passed installation root writes guidance
// outside the target dir — into the real user config — and the idempotency
// guarantee silently breaks because state leaks across runs.
//
// The OpenCode --version probe is stubbed to "not installed": a real opencode
// on PATH would be spawned by that probe, inherit the hostile
// XDG_CONFIG_HOME, and create its own config dir there. That is the external
// binary's behavior, not adapter path resolution, and it made this test pass
// or fail depending on whether the machine had opencode installed.
//
// No t.Parallel here: t.Setenv is process-wide and forbids it.
func TestInjectRoutingStaysContainedUnderHostileEnvironment(t *testing.T) {
	hostile := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(hostile, "xdg"))
	t.Setenv("APPDATA", filepath.Join(hostile, "AppData", "Roaming"))
	previousVersionRunner := opencoderuntime.VersionRunnerOverride
	opencoderuntime.VersionRunnerOverride = func(context.Context, opencoderuntime.Command) (opencoderuntime.CommandOutput, error) {
		return opencoderuntime.CommandOutput{}, exec.ErrNotFound
	}
	t.Cleanup(func() { opencoderuntime.VersionRunnerOverride = previousVersionRunner })

	for _, agent := range catalog.AllAgents() {
		if agent.ID == model.AgentConductor {
			continue // Catalog-only: no standalone guidance target.
		}
		t.Run(string(agent.ID), func(t *testing.T) {
			targetDir := t.TempDir()

			declared, err := RoutingPaths(targetDir, agent.ID)
			if err != nil {
				t.Fatalf("RoutingPaths(%q) error = %v", agent.ID, err)
			}
			for _, path := range declared {
				if !strings.HasPrefix(path, targetDir) {
					t.Fatalf("RoutingPaths(%q) declared a path outside the target dir: %q", agent.ID, path)
				}
			}

			result, err := InjectRoutingWithOptions(targetDir, agent.ID, RoutingOptions{})
			if err != nil {
				t.Fatalf("InjectRouting(%q) error = %v", agent.ID, err)
			}
			if !result.Changed {
				t.Fatalf("InjectRouting(%q) reported no change on a fresh target", agent.ID)
			}
			for _, path := range result.Files {
				if !strings.HasPrefix(path, targetDir) {
					t.Fatalf("InjectRouting(%q) wrote outside the target dir: %q", agent.ID, path)
				}
			}

			entries, err := os.ReadDir(hostile)
			if err != nil {
				t.Fatalf("ReadDir(hostile) error = %v", err)
			}
			if len(entries) != 0 {
				t.Fatalf("InjectRouting(%q) created %d entries under the hostile config root", agent.ID, len(entries))
			}
		})
	}
}

func TestInjectRoutingIsIdempotent(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()

	first, err := InjectRoutingWithOptions(targetDir, model.AgentClaudeCode, RoutingOptions{})
	if err != nil {
		t.Fatalf("first InjectRouting error = %v", err)
	}
	if !first.Changed {
		t.Fatalf("first InjectRouting reported no change")
	}
	afterFirst := readFile(t, first.Files[0])

	second, err := InjectRoutingWithOptions(targetDir, model.AgentClaudeCode, RoutingOptions{})
	if err != nil {
		t.Fatalf("second InjectRouting error = %v", err)
	}
	if second.Changed {
		t.Fatalf("second identical InjectRouting reported a change")
	}
	if got := readFile(t, first.Files[0]); got != afterFirst {
		t.Fatalf("second InjectRouting rewrote the file:\n%s", got)
	}
}

func TestInjectRoutingPreservesUnmanagedUserContent(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()

	adapter, err := agents.NewAdapter(model.AgentClaudeCode)
	if err != nil {
		t.Fatalf("NewAdapter error = %v", err)
	}
	path := adapter.SystemPromptFile(targetDir)

	const (
		above = "# My own prompt\n\nHand-written rules that must survive.\n"
		below = "\n## My trailing notes\n\nAlso hand-written.\n"
	)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(path, []byte(above), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	if _, err := InjectRoutingWithOptions(targetDir, model.AgentClaudeCode, RoutingOptions{}); err != nil {
		t.Fatalf("InjectRouting error = %v", err)
	}

	// Append unmanaged content after the closing marker, then re-inject: both
	// the content above and below the managed block must survive verbatim.
	withTrailing := readFile(t, path) + below
	if err := os.WriteFile(path, []byte(withTrailing), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	result, err := InjectRoutingWithOptions(targetDir, model.AgentClaudeCode, RoutingOptions{})
	if err != nil {
		t.Fatalf("re-inject error = %v", err)
	}
	if result.Changed {
		t.Fatalf("re-injecting identical guidance reported a change")
	}

	final := readFile(t, path)
	if !strings.HasPrefix(final, above) {
		t.Fatalf("unmanaged content above the block was altered:\n%s", final)
	}
	if !strings.HasSuffix(final, below) {
		t.Fatalf("unmanaged content below the block was altered:\n%s", final)
	}
}

func TestInjectRoutingRejectsUnregisteredAgent(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()

	result, err := InjectRoutingWithOptions(targetDir, model.AgentID("totally-unregistered-agent"), RoutingOptions{})
	if err == nil {
		t.Fatalf("InjectRouting accepted an unregistered agent: %+v", result)
	}
	if result.Changed || len(result.Files) != 0 {
		t.Fatalf("InjectRouting reported work for an unregistered agent: %+v", result)
	}

	entries, readErr := os.ReadDir(targetDir)
	if readErr != nil {
		t.Fatalf("ReadDir error = %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("InjectRouting wrote %d entries for an unregistered agent", len(entries))
	}
}

func TestInjectRoutingRejectsBlankTargetDir(t *testing.T) {
	t.Parallel()

	result, err := InjectRoutingWithOptions("   ", model.AgentClaudeCode, RoutingOptions{})
	if err == nil {
		t.Fatalf("InjectRouting accepted a blank target dir: %+v", result)
	}
	if !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("InjectRouting error = %v, want it to wrap ErrInvalidTarget", err)
	}
}

// jinjaBootstrapper mirrors the optional adapter capability that rewrites a
// Jinja router template from its embedded asset on every install.
type jinjaBootstrapper interface {
	BootstrapTemplate(homeDir string) error
}

func TestInjectRoutingSurvivesJinjaTemplateBootstrap(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()

	adapter, err := agents.NewAdapter(model.AgentKimi)
	if err != nil {
		t.Fatalf("NewAdapter error = %v", err)
	}
	if adapter.SystemPromptStrategy() != model.StrategyJinjaModules {
		t.Fatalf("kimi no longer uses StrategyJinjaModules; this regression guard needs a new subject")
	}
	bootstrapper, ok := adapter.(jinjaBootstrapper)
	if !ok {
		t.Fatalf("kimi adapter no longer exposes BootstrapTemplate")
	}

	result, err := InjectRoutingWithOptions(targetDir, model.AgentKimi, RoutingOptions{})
	if err != nil {
		t.Fatalf("InjectRouting error = %v", err)
	}
	var guidancePath string
	for _, path := range result.Files {
		if filepath.Base(path) == routingModuleFile {
			guidancePath = path
		}
	}
	if guidancePath == "" {
		t.Fatalf("InjectRouting touched %v, want %s included", result.Files, routingModuleFile)
	}

	rendered, err := RenderRouting(model.AgentKimi)
	if err != nil {
		t.Fatalf("RenderRouting error = %v", err)
	}

	// A Jinja router template is rewritten from its embedded asset on every
	// install, so guidance stored inside it is destroyed by the next sync.
	if err := bootstrapper.BootstrapTemplate(targetDir); err != nil {
		t.Fatalf("BootstrapTemplate error = %v", err)
	}

	after := readFile(t, guidancePath)
	if !strings.Contains(after, rendered) {
		t.Fatalf("BootstrapTemplate destroyed the routing guidance in %q:\n%s", guidancePath, after)
	}

	// Surviving on disk is not enough: the router template must still pull the
	// module in, otherwise the agent never loads the guidance.
	entry := readFile(t, adapter.SystemPromptFile(targetDir))
	include := `{% include "` + filepath.Base(guidancePath) + `"`
	if !strings.Contains(entry, include) {
		t.Fatalf("router template %q does not include %q:\n%s", adapter.SystemPromptFile(targetDir), filepath.Base(guidancePath), entry)
	}
}

func TestInjectRoutingCurrentKimiReportsRenderedHubWrite(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()
	configDir := filepath.Join(targetDir, ".kimi-code")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	declared, err := RoutingPaths(targetDir, model.AgentKimi)
	if err != nil {
		t.Fatalf("RoutingPaths error = %v", err)
	}
	wantHub := filepath.Join(configDir, "AGENTS.md")
	wantModule := filepath.Join(configDir, routingModuleFile)
	if !samePathSet(declared, []string{wantModule, wantHub}) {
		t.Fatalf("RoutingPaths(current Kimi) = %v, want module and hub", declared)
	}

	result, err := InjectRoutingWithOptions(targetDir, model.AgentKimi, RoutingOptions{})
	if err != nil {
		t.Fatalf("InjectRouting error = %v", err)
	}
	if !result.Changed {
		t.Fatal("InjectRouting changed = false, want true for first hub/module write")
	}
	if !samePathSet(result.Files, declared) {
		t.Fatalf("InjectRouting files = %v, want declared %v", result.Files, declared)
	}

	hub := readFile(t, wantHub)
	if !strings.Contains(hub, "<!-- gentle-ai:kimi-agents-hub -->") || !strings.Contains(hub, "Organic Driven Development") {
		t.Fatalf("current Kimi hub does not contain rendered managed guidance:\n%s", hub)
	}
}

func TestInjectRoutingUsesAlwaysLoadedOrchestratorScope(t *testing.T) {
	t.Parallel()

	for _, agent := range []model.AgentID{model.AgentOpenCode, model.AgentKilocode} {
		t.Run(string(agent), func(t *testing.T) {
			t.Parallel()

			targetDir := t.TempDir()

			adapter, err := agents.NewAdapter(agent)
			if err != nil {
				t.Fatalf("NewAdapter error = %v", err)
			}

			result, err := InjectRoutingWithOptions(targetDir, agent, RoutingOptions{})
			if err != nil {
				t.Fatalf("InjectRouting(%q) error = %v", agent, err)
			}
			if !result.Changed {
				t.Fatalf("InjectRouting(%q) reported no change on a fresh target", agent)
			}

			settingsPath := adapter.SettingsPath(targetDir)
			if len(result.Files) != 1 || result.Files[0] != settingsPath {
				t.Fatalf("InjectRouting(%q) touched %v, want exactly %q", agent, result.Files, settingsPath)
			}

			rendered, err := RenderRouting(agent)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent, err)
			}

			prompt := orchestratorPrompt(t, settingsPath)
			if !strings.Contains(prompt, rendered) {
				t.Fatalf("orchestrator prompt for %q carries no routing guidance:\n%s", agent, prompt)
			}
			if !strings.Contains(prompt, "<!-- gentle-ai:"+RoutingSectionID+" -->") ||
				!strings.Contains(prompt, "<!-- /gentle-ai:"+RoutingSectionID+" -->") {
				t.Fatalf("orchestrator prompt for %q carries no managed section:\n%s", agent, prompt)
			}

			// The global prompt file is a different, non-always-loaded scope for
			// these adapters, so routing must never be delivered there.
			promptFile := adapter.SystemPromptFile(targetDir)
			if _, statErr := os.Stat(promptFile); !os.IsNotExist(statErr) {
				t.Fatalf("InjectRouting(%q) wrote the non-always-loaded scope %q (stat err = %v)", agent, promptFile, statErr)
			}
		})
	}
}

func TestInjectRoutingPreservesUnmanagedOrchestratorSettings(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()

	adapter, err := agents.NewAdapter(model.AgentOpenCode)
	if err != nil {
		t.Fatalf("NewAdapter error = %v", err)
	}
	settingsPath := adapter.SettingsPath(targetDir)

	const existingPrompt = "# Existing orchestrator policy\n\nHand-written rules that must survive.\n"
	existing := map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"model":   "anthropic/claude-opus-4",
		"agent": map[string]any{
			opencodedefault.ManagedAgent: map[string]any{
				"prompt":      existingPrompt,
				"description": "kept",
			},
			"some-other-agent": map[string]any{"prompt": "untouched"},
		},
	}
	writeJSON(t, settingsPath, existing)

	if _, err := InjectRoutingWithOptions(targetDir, model.AgentOpenCode, RoutingOptions{}); err != nil {
		t.Fatalf("InjectRouting error = %v", err)
	}

	rendered, err := RenderRouting(model.AgentOpenCode)
	if err != nil {
		t.Fatalf("RenderRouting error = %v", err)
	}

	prompt := orchestratorPrompt(t, settingsPath)
	if !strings.HasPrefix(prompt, existingPrompt) {
		t.Fatalf("existing orchestrator prompt was altered:\n%s", prompt)
	}
	if !strings.Contains(prompt, rendered) {
		t.Fatalf("routing guidance was not appended to the existing orchestrator prompt:\n%s", prompt)
	}

	settings := readJSON(t, settingsPath)
	if settings["model"] != "anthropic/claude-opus-4" {
		t.Fatalf("unmanaged settings key was dropped: %+v", settings)
	}
	if settings["$schema"] != "https://opencode.ai/config.json" {
		t.Fatalf("unmanaged settings key was dropped: %+v", settings)
	}
	agentsMap, ok := settings["agent"].(map[string]any)
	if !ok {
		t.Fatalf("agent map was dropped: %+v", settings)
	}
	other, ok := agentsMap["some-other-agent"].(map[string]any)
	if !ok || other["prompt"] != "untouched" {
		t.Fatalf("sibling agent definition was altered: %+v", agentsMap)
	}
	managed, ok := agentsMap[opencodedefault.ManagedAgent].(map[string]any)
	if !ok || managed["description"] != "kept" {
		t.Fatalf("sibling orchestrator field was dropped: %+v", agentsMap)
	}
}

func TestInjectRoutingRejectsMalformedOrchestratorSettings(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()

	adapter, err := agents.NewAdapter(model.AgentOpenCode)
	if err != nil {
		t.Fatalf("NewAdapter error = %v", err)
	}
	settingsPath := adapter.SettingsPath(targetDir)

	const corrupt = "this is not json at all"
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(corrupt), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	result, err := InjectRoutingWithOptions(targetDir, model.AgentOpenCode, RoutingOptions{})
	if err == nil {
		t.Fatalf("InjectRouting accepted unreadable settings: %+v", result)
	}
	if result.Changed || len(result.Files) != 0 {
		t.Fatalf("InjectRouting reported work for unreadable settings: %+v", result)
	}
	if got := readFile(t, settingsPath); got != corrupt {
		t.Fatalf("InjectRouting clobbered unreadable settings:\n%s", got)
	}
}

func TestInjectRoutingKeepsMarkdownSectionAgentsOnTheirPromptFile(t *testing.T) {
	t.Parallel()

	for _, agent := range markdownSectionAgents(t) {
		t.Run(string(agent), func(t *testing.T) {
			t.Parallel()

			targetDir := t.TempDir()

			adapter, err := agents.NewAdapter(agent)
			if err != nil {
				t.Fatalf("NewAdapter error = %v", err)
			}
			path := adapter.SystemPromptFile(targetDir)

			const (
				above = "# My own prompt\n\nHand-written rules that must survive.\n"
				below = "\n## My trailing notes\n\nAlso hand-written.\n"
			)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("MkdirAll error = %v", err)
			}
			if err := os.WriteFile(path, []byte(above), 0o644); err != nil {
				t.Fatalf("WriteFile error = %v", err)
			}

			result, err := InjectRoutingWithOptions(targetDir, agent, RoutingOptions{})
			if err != nil {
				t.Fatalf("InjectRouting(%q) error = %v", agent, err)
			}
			if len(result.Files) != 1 || result.Files[0] != path {
				t.Fatalf("InjectRouting(%q) touched %v, want exactly %q", agent, result.Files, path)
			}

			withTrailing := readFile(t, path) + below
			if err := os.WriteFile(path, []byte(withTrailing), 0o644); err != nil {
				t.Fatalf("WriteFile error = %v", err)
			}

			second, err := InjectRoutingWithOptions(targetDir, agent, RoutingOptions{})
			if err != nil {
				t.Fatalf("re-inject(%q) error = %v", agent, err)
			}
			if second.Changed {
				t.Fatalf("re-injecting identical guidance for %q reported a change", agent)
			}

			final := readFile(t, path)
			if !strings.HasPrefix(final, above) {
				t.Fatalf("unmanaged content above the block was altered for %q:\n%s", agent, final)
			}
			if !strings.HasSuffix(final, below) {
				t.Fatalf("unmanaged content below the block was altered for %q:\n%s", agent, final)
			}
		})
	}
}

func TestInjectRoutingIsIdempotentForEverySupportedAgent(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		if agent.ID == model.AgentConductor {
			continue // Catalog-only: no standalone guidance target.
		}
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			targetDir := t.TempDir()

			first, err := InjectRoutingWithOptions(targetDir, agent.ID, RoutingOptions{})
			if err != nil {
				t.Fatalf("first InjectRouting(%q) error = %v", agent.ID, err)
			}
			if !first.Changed {
				t.Fatalf("first InjectRouting(%q) reported no change", agent.ID)
			}
			afterFirst := readFile(t, first.Files[0])

			second, err := InjectRoutingWithOptions(targetDir, agent.ID, RoutingOptions{})
			if err != nil {
				t.Fatalf("second InjectRouting(%q) error = %v", agent.ID, err)
			}
			if second.Changed {
				t.Fatalf("second identical InjectRouting(%q) reported a change", agent.ID)
			}
			if got := readFile(t, first.Files[0]); got != afterFirst {
				t.Fatalf("second InjectRouting(%q) rewrote the file:\n%s", agent.ID, got)
			}
		})
	}
}

// Only the three policy evidence sizes are exempt; other token vocabulary
// remains forbidden. Rune-aware boundaries reject word continuations, including
// Unicode numbers/marks, and numeric prefixes such as 1.2k or -2k. This shared
// test helper changes neither rendered guidance nor runtime behavior.
func routingBlockWithoutEvidenceSizes(block string) string {
	isContinuation := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) ||
			(unicode.IsSymbol(r) && r != '`') || unicode.Is(unicode.Pc, r) || unicode.Is(unicode.Pd, r) || unicode.Is(unicode.Cf, r) || r == utf8.RuneError
	}
	for _, phrase := range []string{"10k tokens", "2k tokens", "150k parent-context tokens"} {
		for offset := 0; offset < len(block); {
			next := strings.Index(block[offset:], phrase)
			if next < 0 {
				break
			}
			start := offset + next
			end := start + len(phrase)
			left, right := true, true
			if start > 0 {
				r, _ := utf8.DecodeLastRuneInString(block[:start])
				left = !isContinuation(r) && r != '.' && r != ',' && r != '+'
			}
			if end < len(block) {
				r, _ := utf8.DecodeRuneInString(block[end:])
				right = !isContinuation(r)
			}
			if left && right {
				block = block[:start] + "evidence size" + block[end:]
				offset = start + len("evidence size")
			} else {
				offset = end
			}
		}
	}
	return block
}

func TestInjectedRoutingTokenGuardSelectiveExceptions(t *testing.T) {
	for _, test := range []struct {
		name      string
		block     string
		forbidden bool
	}{
		{"inline budget", "approximately 10k tokens of evidence", false},
		{"handoff budget", "at most approximately 2k tokens with path:line evidence", false},
		{"context budget", "approximately 150k parent-context tokens, advisory", false},
		{"combined budgets", "10k tokens; 2k tokens; 150k parent-context tokens", false},
		{"repeated budget", "10k tokens and 10k tokens", false},
		{"punctuated budgets", "(10k tokens), `2k tokens`; [150k parent-context tokens].", false},
		{"authorization vocabulary", "authorization token", true},
		{"budget does not hide other vocabulary", "10k tokens and access token", true},
		{"unapproved evidence size", "20k tokens", true},
		{"prefixed size", "110k tokens", true},
		{"decimal prefix", "1.2k tokens", true},
		{"decimal prefix inline", "1.10k tokens", true},
		{"decimal prefix context", "1.150k parent-context tokens", true},
		{"fraction without integer", ".2k tokens", true},
		{"negative size", "-2k tokens", true},
		{"positive sign", "+2k tokens", true},
		{"Unicode minus", "−2k tokens", true},
		{"comma prefix", "1,2k tokens", true},
		{"Unicode letter prefix", "é2k tokens", true},
		{"Unicode letter suffix", "10k tokensé", true},
		{"Unicode number prefix", "٢2k tokens", true},
		{"Unicode number suffix", "10k tokens٢", true},
		{"Unicode combining prefix", "\u03012k tokens", true},
		{"Unicode combining suffix", "10k tokens\u0301", true},
		{"Unicode format continuation", "10k tokens\u200d", true},
		{"embedded word", "budget2k tokens", true},
		{"hyphenated continuation", "10k tokens-extra", true},
		{"singular token", "10k token", true},
		{"suffixed word", "10k tokens_extra", true},
		{"other retired vocabulary", "10k tokens and work-start", true},
	} {
		for _, guard := range []struct {
			name  string
			check func(string, string) string
		}{
			{"injection", func(block, _ string) string { return routingBlockWithoutEvidenceSizes(block) }},
			{"routing", routingVocabularyForGuard},
		} {
			t.Run(guard.name+"/"+test.name, func(t *testing.T) {
				forbidden := false
				for _, word := range retiredRemoteControlPlaneVocabulary {
					checked := strings.ToLower(guard.check(test.block, word))
					forbidden = forbidden || strings.Contains(checked, strings.ToLower(word))
				}
				if forbidden != test.forbidden {
					t.Errorf("guard rejects %q = %t, want %t", test.block, forbidden, test.forbidden)
				}
			})
		}
	}
}

func TestInjectRoutingDeliversNoRetiredControlPlaneVocabulary(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		if agent.ID == model.AgentConductor {
			continue // Catalog-only: no standalone guidance target.
		}
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			targetDir := t.TempDir()

			result, err := InjectRoutingWithOptions(targetDir, agent.ID, RoutingOptions{})
			if err != nil {
				t.Fatalf("InjectRouting(%q) error = %v", agent.ID, err)
			}

			block := managedRoutingBlock(deliveredGuidance(t, result.Files[0]))
			if strings.TrimSpace(block) == "" {
				t.Fatalf("InjectRouting(%q) delivered no managed routing block", agent.ID)
			}
			lowered := strings.ToLower(routingBlockWithoutEvidenceSizes(block))
			for _, forbidden := range retiredRemoteControlPlaneVocabulary {
				if strings.Contains(lowered, strings.ToLower(forbidden)) {
					t.Fatalf("InjectRouting(%q) delivered retired vocabulary %q:\n%s", agent.ID, forbidden, block)
				}
			}
		})
	}
}

// markdownSectionAgents lists every supported agent whose guidance is delivered
// as a marker section inside its own system prompt file — that is, everything
// except the Jinja router and the orchestrator-scoped OpenCode family.
func markdownSectionAgents(t *testing.T) []model.AgentID {
	t.Helper()

	var selected []model.AgentID
	for _, agent := range catalog.AllAgents() {
		if agent.ID == model.AgentOpenCode || agent.ID == model.AgentKilocode {
			continue
		}
		if agent.ID == model.AgentConductor {
			continue // Catalog-only: inherits Claude Code config, no prompt file of its own.
		}
		adapter, err := agents.NewAdapter(agent.ID)
		if err != nil {
			t.Fatalf("NewAdapter(%q) error = %v", agent.ID, err)
		}
		if adapter.SystemPromptStrategy() == model.StrategyJinjaModules {
			continue
		}
		selected = append(selected, agent.ID)
	}
	if len(selected) != supportedAgentCount-4 {
		t.Fatalf("selected %d markdown-section agents, want %d", len(selected), supportedAgentCount-4)
	}
	return selected
}

// deliveredGuidance returns the guidance text an agent actually loads from the
// written file: a settings document carries it as an encoded prompt string,
// every other target carries it as raw markdown.
func deliveredGuidance(t *testing.T, path string) string {
	t.Helper()

	if strings.HasSuffix(path, ".json") {
		return orchestratorPrompt(t, path)
	}
	return readFile(t, path)
}

func orchestratorPrompt(t *testing.T, path string) string {
	t.Helper()

	var settings struct {
		Agent map[string]struct {
			Prompt string `json:"prompt"`
		} `json:"agent"`
	}
	if err := json.Unmarshal([]byte(readFile(t, path)), &settings); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", path, err)
	}
	return settings.Agent[opencodedefault.ManagedAgent].Prompt
}

// managedRoutingBlock returns only the content Gentle AI owns, so assertions
// about the block never accidentally inspect surrounding user content.
func managedRoutingBlock(content string) string {
	open := "<!-- gentle-ai:" + RoutingSectionID + " -->"
	closing := "<!-- /gentle-ai:" + RoutingSectionID + " -->"

	start := strings.Index(content, open)
	end := strings.Index(content, closing)
	if start < 0 || end <= start {
		return ""
	}
	return content[start+len(open) : end]
}

func writeJSON(t *testing.T, path string, value map[string]any) {
	t.Helper()

	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()

	decoded := map[string]any{}
	if err := json.Unmarshal([]byte(readFile(t, path)), &decoded); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", path, err)
	}
	return decoded
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(data)
}
