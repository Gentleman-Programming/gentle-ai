package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

func TestComponentApplyStepOpenClawWorkspaceScopedInjections(t *testing.T) {
	restoreLookPath := cmdLookPath
	t.Cleanup(func() { cmdLookPath = restoreLookPath })

	tests := []struct {
		name      string
		component model.ComponentID
		fileName  string
		marker    string
	}{
		{
			name:      "engram writes protocol to workspace AGENTS",
			component: model.ComponentEngram,
			fileName:  "AGENTS.md",
			marker:    "<!-- gentle-ai:engram-protocol -->",
		},
		{
			name:      "persona writes soul to workspace",
			component: model.ComponentPersona,
			fileName:  "SOUL.md",
			marker:    "<!-- gentle-ai:persona -->",
		},
		{
			name:      "sdd writes protocol to workspace AGENTS",
			component: model.ComponentSDD,
			fileName:  "AGENTS.md",
			marker:    "<!-- gentle-ai:sdd-orchestrator -->",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			workspace := t.TempDir()
			cmdLookPath = func(name string) (string, error) {
				return filepath.Join(home, "bin", name), nil
			}

			if tt.component == model.ComponentEngram {
				writeOpenClawConfigWithWorkspace(t, home, workspace)
			}

			step := componentApplyStep{
				id:           "component:" + string(tt.component),
				component:    tt.component,
				homeDir:      home,
				workspaceDir: workspace,
				agents:       []model.AgentID{model.AgentOpenClaw},
				selection:    model.Selection{Persona: model.PersonaGentleman},
				profile:      system.PlatformProfile{PackageManager: "brew"},
				scope:        ScopeWorkspace,
			}

			if err := step.Run(); err != nil {
				t.Fatalf("componentApplyStep.Run() error = %v", err)
			}

			workspaceFile := filepath.Join(workspace, tt.fileName)
			homeFile := filepath.Join(home, tt.fileName)
			body, err := os.ReadFile(workspaceFile)
			if err != nil {
				t.Fatalf("ReadFile(%q): %v", workspaceFile, err)
			}
			if !containsAnyOpenClawMarker(string(body), tt.marker) {
				t.Fatalf("workspace file missing marker %q; got:\n%s", tt.marker, string(body))
			}
			if _, err := os.Stat(homeFile); !os.IsNotExist(err) {
				t.Fatalf("OpenClaw orchestration must not write %q; stat err=%v", homeFile, err)
			}
			if tt.component == model.ComponentEngram {
				assertOpenClawEngramMCPInGlobalConfig(t, home)
				assertNoOpenClawEngramMCPInWorkspaceConfig(t, workspace)
			}
		})
	}
}

func TestComponentSyncStepOpenClawGlobalInjections(t *testing.T) {
	tests := []struct {
		name      string
		component model.ComponentID
		fileName  string
		marker    string
	}{
		{
			name:      "engram sync writes protocol to home AGENTS",
			component: model.ComponentEngram,
			fileName:  "AGENTS.md",
			marker:    "<!-- gentle-ai:engram-protocol -->",
		},
		{
			name:      "persona sync writes soul to home",
			component: model.ComponentPersona,
			fileName:  "SOUL.md",
			marker:    "<!-- gentle-ai:persona -->",
		},
		{
			name:      "sdd sync writes protocol to home AGENTS",
			component: model.ComponentSDD,
			fileName:  "AGENTS.md",
			marker:    "<!-- gentle-ai:sdd-orchestrator -->",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			workspace := t.TempDir()
			if tt.component == model.ComponentEngram {
				writeOpenClawConfigWithWorkspace(t, home, workspace)
			}

			step := componentSyncStep{
				id:           "sync:component:" + string(tt.component),
				component:    tt.component,
				homeDir:      home,
				workspaceDir: workspace,
				agents:       []model.AgentID{model.AgentOpenClaw},
				selection:    model.Selection{Persona: model.PersonaGentleman},
			}

			if err := step.Run(); err != nil {
				t.Fatalf("componentSyncStep.Run() error = %v", err)
			}

			workspaceFile := filepath.Join(workspace, tt.fileName)
			homeFile := filepath.Join(home, tt.fileName)
			body, err := os.ReadFile(homeFile)
			if err != nil {
				t.Fatalf("ReadFile(%q): %v", homeFile, err)
			}
			if !containsAnyOpenClawMarker(string(body), tt.marker) {
				t.Fatalf("home file missing marker %q", tt.marker)
			}
			if _, err := os.Stat(workspaceFile); !os.IsNotExist(err) {
				t.Fatalf("OpenClaw sync must not write %q; stat err=%v", workspaceFile, err)
			}
			if tt.component == model.ComponentEngram {
				assertOpenClawEngramMCPInGlobalConfig(t, home)
				assertNoOpenClawEngramMCPInWorkspaceConfig(t, workspace)
			}
		})
	}
}

func TestGlobalInstallKeepsAmbientProjectUnchanged(t *testing.T) {
	testGlobalArtifactRoots(t, false)
}

func TestGlobalSyncKeepsAmbientProjectUnchanged(t *testing.T) {
	testGlobalArtifactRoots(t, true)
}

func testGlobalArtifactRoots(t *testing.T, sync bool) {
	t.Helper()
	for _, config := range []string{"missing", "malformed", "empty", "configured"} {
		t.Run(config, func(t *testing.T) {
			home, workspace, project := t.TempDir(), t.TempDir(), t.TempDir()
			t.Setenv("HOME", home)
			t.Chdir(project)
			for _, root := range []string{project, workspace} {
				mustWriteFile(t, filepath.Join(root, "go.mod"), []byte("module untouched\n"))
			}
			// Malformed runtime configuration must not affect unrelated artifact routing.
			if config == "configured" {
				writeOpenClawConfigWithWorkspace(t, home, workspace)
			} else if config != "missing" {
				body := `{}`
				if config == "malformed" {
					body = `{`
				}
				mustWriteFile(t, filepath.Join(home, ".openclaw", "openclaw.json"), []byte(body))
			}
			selection := model.Selection{
				Agents:     []model.AgentID{model.AgentOpenClaw, model.AgentWindsurf, model.AgentPi},
				Components: []model.ComponentID{model.ComponentPersona, model.ComponentSDD, model.ComponentSkills},
				Skills:     []model.SkillID{model.SkillGoTesting},
				Persona:    model.PersonaGentleman, StrictTDD: true,
			}
			if config != "malformed" {
				selection.Components = append(selection.Components, model.ComponentEngram, model.ComponentContext7)
				previous := cmdLookPath
				t.Cleanup(func() { cmdLookPath = previous })
				cmdLookPath = func(name string) (string, error) { return filepath.Join(home, "bin", name), nil }
			}
			if sync {
				rt, err := newSyncRuntime(home, selection)
				if err != nil {
					t.Fatal(err)
				}
				for _, step := range rt.stagePlan().Apply {
					if err := step.Run(); err != nil {
						t.Fatalf("%s: %v", step.ID(), err)
					}
				}
			} else {
				runInstallInjectionSteps(t, newTestInstallRuntime(t, home, selection))
			}
			if config != "malformed" {
				assertOpenClawEngramMCPInGlobalConfig(t, home)
				assertOpenClawInstructionsInWorkspace(t, home)
				if _, err := os.Stat(filepath.Join(home, ".codeium", "windsurf", "mcp_config.json")); err != nil {
					t.Fatal(err)
				}
			}
			if config == "configured" {
				root := readJSONMap(t, filepath.Join(home, ".openclaw", "openclaw.json"))
				defaults := objectAtOpenClawTest(t, objectAtOpenClawTest(t, root, "agents"), "defaults")
				if defaults["workspace"] != workspace {
					t.Fatalf("runtime workspace changed: %v", defaults)
				}
			}
			for _, root := range []string{project, workspace} {
				entries, err := os.ReadDir(root)
				if err != nil || len(entries) != 1 || entries[0].Name() != "go.mod" {
					t.Errorf("ambient root changed: %s: %v (%v)", root, entries, err)
				}
				if got := readTextFile(t, filepath.Join(root, "go.mod")); got != "module untouched\n" {
					t.Errorf("ambient content changed: %q", got)
				}
			}
			for _, path := range []string{"AGENTS.md", "SOUL.md", ".openclaw/skills/go-testing/SKILL.md", ".codeium/windsurf/memories/global_rules.md", ".codeium/windsurf/skills/go-testing/SKILL.md", ".pi/gentle-ai/persona.json"} {
				if _, err := os.Stat(filepath.Join(home, path)); err != nil {
					t.Errorf("missing global artifact %s: %v", path, err)
				}
			}
		})
	}
}

func TestExplicitWorkspaceInstallOverridesOpenClawConfig(t *testing.T) {
	home, configured, workspace := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(workspace)
	mustWriteFile(t, filepath.Join(workspace, "go.mod"), []byte("module untouched\n"))
	writeOpenClawConfigWithWorkspace(t, home, configured)
	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentOpenClaw, model.AgentWindsurf, model.AgentPi},
		Components: []model.ComponentID{model.ComponentPersona, model.ComponentSDD, model.ComponentSkills},
		Skills:     []model.SkillID{model.SkillGoTesting}, Persona: model.PersonaGentleman,
	}
	rt, err := newInstallRuntime(home, ScopeWorkspace, ChannelStable, selection, planner.ResolvedPlan{Agents: selection.Agents, OrderedComponents: selection.Components}, system.PlatformProfile{})
	if err != nil {
		t.Fatal(err)
	}
	runInstallInjectionSteps(t, rt)
	for _, path := range []string{"AGENTS.md", "SOUL.md", ".openclaw/skills/go-testing/SKILL.md", ".windsurf/workflows/sdd-new.md", ".pi/gentle-ai/persona.json"} {
		if _, err := os.Stat(filepath.Join(workspace, path)); err != nil {
			t.Errorf("missing workspace artifact %s: %v", path, err)
		}
		if _, err := os.Stat(filepath.Join(home, path)); !os.IsNotExist(err) {
			t.Errorf("unexpected home artifact %s: %v", path, err)
		}
	}
	entries, err := os.ReadDir(configured)
	if err != nil || len(entries) != 0 {
		t.Fatalf("configured workspace changed: %v (%v)", entries, err)
	}
}

func TestOpenClawConfigDoesNotRedirectProjectToolRuntimeCwd(t *testing.T) {
	home, configured, project := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(project)
	writeOpenClawConfigWithWorkspace(t, home, configured)
	selection := model.Selection{
		Agents:         []model.AgentID{model.AgentOpenClaw, model.AgentPi},
		CommunityTools: []model.CommunityToolID{model.CommunityToolCodeGraph},
	}
	install := newTestInstallRuntime(t, home, selection)
	sync, err := newSyncRuntime(home, selection)
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if install.workspaceDir != cwd || sync.workspaceDir != cwd {
		t.Fatalf("project tool cwd redirected: install=%s sync=%s want=%s", install.workspaceDir, sync.workspaceDir, cwd)
	}
	tools := 0
	for _, step := range install.stagePlan().Apply {
		switch step := step.(type) {
		case communityToolInstallStep:
			tools++
			if step.workspaceDir != cwd {
				t.Errorf("%s cwd = %s", step.ID(), step.workspaceDir)
			}
		case piCodeGraphReconcileStep:
			tools++
			if step.workspaceDir != cwd {
				t.Errorf("Pi CodeGraph cwd = %s", step.workspaceDir)
			}
		}
	}
	if tools != 2 {
		t.Fatalf("project tool steps = %d, want CodeGraph and Pi reconciliation", tools)
	}
}

func TestRunSyncOpenClawSkillsUseGlobalRoot(t *testing.T) {
	home := t.TempDir()
	activeWorkspace := t.TempDir()
	currentProject := t.TempDir()
	writeOpenClawConfigWithWorkspace(t, home, activeWorkspace)
	t.Chdir(currentProject)

	skillIDs := []model.SkillID{
		model.SkillGoTesting,
		model.SkillBranchPR,
		model.SkillWorkUnitCommits,
	}
	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentOpenClaw},
		Components: []model.ComponentID{model.ComponentSkills},
		Skills:     skillIDs,
	}

	result, err := RunSyncWithSelection(home, selection)
	if err != nil {
		t.Fatalf("RunSyncWithSelection() error = %v", err)
	}
	if !result.Verify.Ready {
		t.Fatalf("post-sync verification ready = false, report = %#v", result.Verify)
	}

	for _, skillID := range skillIDs {
		homeSkill := filepath.Join(home, ".openclaw", "skills", string(skillID), "SKILL.md")
		if _, err := os.Stat(homeSkill); err != nil {
			t.Errorf("global OpenClaw skill %q missing: %v", homeSkill, err)
		}
		workspaceSkill := filepath.Join(activeWorkspace, ".openclaw", "skills", string(skillID), "SKILL.md")
		if _, err := os.Stat(workspaceSkill); !os.IsNotExist(err) {
			t.Errorf("OpenClaw sync wrote configured workspace skill %q; stat err=%v", workspaceSkill, err)
		}
	}
}

func writeOpenClawConfigWithWorkspace(t *testing.T, home, workspace string) {
	t.Helper()
	configDir := filepath.Join(home, ".openclaw")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(openclaw config dir) error = %v", err)
	}
	config := `{"agents":{"defaults":{"workspace":` + quoteJSON(workspace) + `}},"mcp":{"servers":{"context7":{"command":"context7"}}}}`
	if err := os.WriteFile(filepath.Join(configDir, "openclaw.json"), []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile(openclaw.json) error = %v", err)
	}
}

func quoteJSON(value string) string {
	return `"` + strings.ReplaceAll(value, `\`, `\\`) + `"`
}

// containsAnyOpenClawMarker tolerates both the upstream "gentle-ai:" managed
// markers and this fork's "axiom:" rebrand of the same markers, so ported
// upstream assertions keep working regardless of which one an asset emits.
func containsAnyOpenClawMarker(body, marker string) bool {
	if strings.Contains(body, marker) {
		return true
	}
	axiomMarker := strings.ReplaceAll(marker, "gentle-ai:", "axiom:")
	return strings.Contains(body, axiomMarker)
}

func assertOpenClawInstructionsInWorkspace(t *testing.T, workspace string) {
	t.Helper()
	agentsText := readOpenClawTestFile(t, filepath.Join(workspace, "AGENTS.md"))
	for _, want := range []string{"gentle-ai:engram-protocol", "gentle-ai:sdd-orchestrator", "gentle-ai:strict-tdd-mode"} {
		if !containsAnyOpenClawMarker(agentsText, want) {
			t.Fatalf("active workspace AGENTS.md missing %q; got:\n%s", want, agentsText)
		}
	}

	soulText := readOpenClawTestFile(t, filepath.Join(workspace, "SOUL.md"))
	if !containsAnyOpenClawMarker(soulText, "gentle-ai:persona") || !strings.Contains(soulText, "Senior Architect") {
		t.Fatalf("active workspace SOUL.md missing Gentle AI persona; got:\n%s", soulText)
	}
}

func assertOpenClawEngramMCPInGlobalConfig(t *testing.T, home string) {
	t.Helper()
	configPath := filepath.Join(home, ".openclaw", "openclaw.json")
	root := readJSONMap(t, configPath)
	servers := objectAtOpenClawTest(t, objectAtOpenClawTest(t, root, "mcp"), "servers")
	engramServer := objectAtOpenClawTest(t, servers, "engram")
	command, ok := engramServer["command"].(string)
	if !ok || filepath.Base(command) != "engram" {
		t.Fatalf("global OpenClaw Engram command = %#v, want engram executable", engramServer["command"])
	}
}

func assertNoOpenClawEngramMCPInWorkspaceConfig(t *testing.T, workspace string) {
	t.Helper()
	configPath := filepath.Join(workspace, ".openclaw", "openclaw.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return
	} else if err != nil {
		t.Fatalf("Stat(%q) error = %v", configPath, err)
	}

	root := readJSONMap(t, configPath)
	mcp, ok := root["mcp"].(map[string]any)
	if !ok {
		return
	}
	servers, ok := mcp["servers"].(map[string]any)
	if !ok {
		return
	}
	if _, ok := servers["engram"]; ok {
		t.Fatalf("workspace OpenClaw config %q must not contain mcp.servers.engram", configPath)
	}
}

func assertNoOpenClawInstructionsInCurrentProject(t *testing.T, project string) {
	t.Helper()
	for _, name := range []string{"AGENTS.md", "SOUL.md", "TOOLS.md"} {
		if _, err := os.Stat(filepath.Join(project, name)); !os.IsNotExist(err) {
			t.Fatalf("OpenClaw instruction routing must not write %s in current project; stat err=%v", name, err)
		}
	}
}

func readOpenClawTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(content)
}

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	content := readOpenClawTestFile(t, path)
	var root map[string]any
	if err := json.Unmarshal([]byte(content), &root); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v; content:\n%s", path, err, content)
	}
	return root
}

func objectAtOpenClawTest(t *testing.T, root map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := root[key]
	if !ok {
		t.Fatalf("missing object key %q in %#v", key, root)
	}
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("key %q has type %T, want object", key, value)
	}
	return object
}
