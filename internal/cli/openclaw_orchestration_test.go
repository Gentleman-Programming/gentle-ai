package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/planner"
	"github.com/gentleman-programming/gentle-ai/internal/system"
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
			if !strings.Contains(string(body), tt.marker) {
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

func TestComponentSyncStepOpenClawWorkspaceScopedInjections(t *testing.T) {
	tests := []struct {
		name      string
		component model.ComponentID
		fileName  string
		marker    string
	}{
		{
			name:      "engram sync writes protocol to workspace AGENTS",
			component: model.ComponentEngram,
			fileName:  "AGENTS.md",
			marker:    "<!-- gentle-ai:engram-protocol -->",
		},
		{
			name:      "persona sync writes soul to workspace",
			component: model.ComponentPersona,
			fileName:  "SOUL.md",
			marker:    "<!-- gentle-ai:persona -->",
		},
		{
			name:      "sdd sync writes protocol to workspace AGENTS",
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
			body, err := os.ReadFile(workspaceFile)
			if err != nil {
				t.Fatalf("ReadFile(%q): %v", workspaceFile, err)
			}
			if !strings.Contains(string(body), tt.marker) {
				t.Fatalf("workspace file missing marker %q; got:\n%s", tt.marker, string(body))
			}
			if _, err := os.Stat(homeFile); !os.IsNotExist(err) {
				t.Fatalf("OpenClaw sync must not write %q; stat err=%v", homeFile, err)
			}
			if tt.component == model.ComponentEngram {
				assertOpenClawEngramMCPInGlobalConfig(t, home)
				assertNoOpenClawEngramMCPInWorkspaceConfig(t, workspace)
			}
		})
	}
}

func TestInstallRuntimeOpenClawUsesConfiguredActiveWorkspace(t *testing.T) {
	home := t.TempDir()
	activeWorkspace := t.TempDir()
	currentProject := t.TempDir()
	writeOpenClawConfigWithWorkspace(t, home, activeWorkspace)
	t.Chdir(currentProject)

	restoreLookPath := cmdLookPath
	t.Cleanup(func() { cmdLookPath = restoreLookPath })
	cmdLookPath = func(name string) (string, error) {
		return filepath.Join(home, "bin", name), nil
	}

	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentOpenClaw},
		Components: []model.ComponentID{model.ComponentPersona, model.ComponentSDD, model.ComponentEngram},
		Persona:    model.PersonaGentleman,
		StrictTDD:  true,
	}
	resolved := planner.ResolvedPlan{
		Agents:            []model.AgentID{model.AgentOpenClaw},
		OrderedComponents: selection.Components,
	}
	rt, err := newInstallRuntime(home, selection, resolved, system.PlatformProfile{PackageManager: "brew"})
	if err != nil {
		t.Fatalf("newInstallRuntime() error = %v", err)
	}

	for _, step := range rt.stagePlan().Apply {
		if err := step.Run(); err != nil {
			t.Fatalf("Run(%s) error = %v", step.ID(), err)
		}
	}

	assertOpenClawInstructionsInWorkspace(t, activeWorkspace)
	assertNoOpenClawInstructionsInCurrentProject(t, currentProject)
}

func TestSyncRuntimeOpenClawUsesConfiguredActiveWorkspace(t *testing.T) {
	home := t.TempDir()
	activeWorkspace := t.TempDir()
	currentProject := t.TempDir()
	writeOpenClawConfigWithWorkspace(t, home, activeWorkspace)
	t.Chdir(currentProject)

	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentOpenClaw},
		Components: []model.ComponentID{model.ComponentPersona, model.ComponentSDD, model.ComponentEngram},
		Persona:    model.PersonaGentleman,
		StrictTDD:  true,
	}
	rt, err := newSyncRuntime(home, selection)
	if err != nil {
		t.Fatalf("newSyncRuntime() error = %v", err)
	}
	for _, step := range rt.stagePlan().Apply {
		if err := step.Run(); err != nil {
			t.Fatalf("Run(%s) error = %v", step.ID(), err)
		}
	}

	assertOpenClawInstructionsInWorkspace(t, activeWorkspace)
	assertNoOpenClawInstructionsInCurrentProject(t, currentProject)
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

func assertOpenClawInstructionsInWorkspace(t *testing.T, workspace string) {
	t.Helper()
	agentsText := readOpenClawTestFile(t, filepath.Join(workspace, "AGENTS.md"))
	for _, want := range []string{"gentle-ai:engram-protocol", "gentle-ai:sdd-orchestrator", "gentle-ai:strict-tdd-mode"} {
		if !strings.Contains(agentsText, want) {
			t.Fatalf("active workspace AGENTS.md missing %q; got:\n%s", want, agentsText)
		}
	}

	soulText := readOpenClawTestFile(t, filepath.Join(workspace, "SOUL.md"))
	if !strings.Contains(soulText, "gentle-ai:persona") || !strings.Contains(soulText, "Senior Architect") {
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
