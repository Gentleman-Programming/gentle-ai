package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/planner"
)

func TestRenderDryRunIncludesPlatformDecision(t *testing.T) {
	result := InstallResult{
		Selection: model.Selection{Persona: model.PersonaGentleman, Preset: model.PresetFullGentleman},
		Resolved: planner.ResolvedPlan{
			Agents:            []model.AgentID{model.AgentClaudeCode},
			OrderedComponents: []model.ComponentID{model.ComponentEngram},
		},
		Review: planner.ReviewPayload{
			PlatformDecision: planner.PlatformDecision{
				OS:             "linux",
				LinuxDistro:    "ubuntu",
				PackageManager: "apt",
				Supported:      true,
			},
		},
	}

	output := RenderDryRun(result)

	want := "Platform decision: os=linux distro=ubuntu package-manager=apt status=supported"
	if !strings.Contains(output, want) {
		t.Fatalf("RenderDryRun() missing platform decision\noutput=%s", output)
	}
}

func TestPlannedSyncPathsIncludesPiArtifactsWhenPiSubagentsPresent(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()

	if err := os.MkdirAll(filepath.Join(home, ".config", "pi-coding-agent"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, ".pi", "extensions", "pi-subagents"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentPiCodingAgent},
		Components: []model.ComponentID{model.ComponentSDD},
		SDDMode:    model.SDDModeMulti,
	}

	planned, err := plannedSyncPaths(home, workspace, selection)
	if err != nil {
		t.Fatalf("plannedSyncPaths() error = %v", err)
	}

	if !containsPathSuffix(planned, filepath.Join(".pi", "agents", "sdd-apply.md")) {
		t.Fatalf("plannedSyncPaths() missing .pi/agents/sdd-apply.md\npaths=%v", planned)
	}
	if !containsPathSuffix(planned, filepath.Join(".pi", "agents", "sdd.chain.md")) {
		t.Fatalf("plannedSyncPaths() missing .pi/agents/sdd.chain.md\npaths=%v", planned)
	}
}

func TestPlannedSyncPathsUsesProjectRootForPiArtifactsWhenWorkspaceNested(t *testing.T) {
	home := t.TempDir()
	projectRoot := t.TempDir()
	workspace := filepath.Join(projectRoot, "pkg", "feature")

	if err := os.MkdirAll(filepath.Join(home, ".config", "pi-coding-agent"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, ".pi", "extensions", "pi-subagents"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}

	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentPiCodingAgent},
		Components: []model.ComponentID{model.ComponentSDD},
		SDDMode:    model.SDDModeMulti,
	}

	planned, err := plannedSyncPaths(home, workspace, selection)
	if err != nil {
		t.Fatalf("plannedSyncPaths() error = %v", err)
	}

	if !containsPathSuffix(planned, filepath.Join(projectRoot, ".pi", "agents", "sdd-apply.md")) {
		t.Fatalf("plannedSyncPaths() missing project-root PI artifact path\npaths=%v", planned)
	}
	if containsPathSuffix(planned, filepath.Join(workspace, ".pi", "agents", "sdd-apply.md")) {
		t.Fatalf("plannedSyncPaths() should not use nested workspace path for PI artifacts\npaths=%v", planned)
	}
}

func TestPlannedSyncPathsUsesMonorepoRootMarkersForPiArtifacts(t *testing.T) {
	home := t.TempDir()
	projectRoot := t.TempDir()
	workspace := filepath.Join(projectRoot, "apps", "web")

	if err := os.MkdirAll(filepath.Join(home, ".config", "pi-coding-agent"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "turbo.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(turbo.json) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, ".pi", "extensions", "pi-subagents"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}

	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentPiCodingAgent},
		Components: []model.ComponentID{model.ComponentSDD},
		SDDMode:    model.SDDModeMulti,
	}

	planned, err := plannedSyncPaths(home, workspace, selection)
	if err != nil {
		t.Fatalf("plannedSyncPaths() error = %v", err)
	}

	if !containsPathSuffix(planned, filepath.Join(projectRoot, ".pi", "agents", "sdd-apply.md")) {
		t.Fatalf("plannedSyncPaths() missing project-root PI artifact path for monorepo marker\npaths=%v", planned)
	}
	if containsPathSuffix(planned, filepath.Join(workspace, ".pi", "agents", "sdd-apply.md")) {
		t.Fatalf("plannedSyncPaths() should not use nested workspace path when monorepo marker exists\npaths=%v", planned)
	}
}

func TestPlannedSyncPathsExcludesPiArtifactsWhenPiSubagentsAbsent(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()

	if err := os.MkdirAll(filepath.Join(home, ".config", "pi-coding-agent"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentPiCodingAgent},
		Components: []model.ComponentID{model.ComponentSDD},
		SDDMode:    model.SDDModeMulti,
	}

	planned, err := plannedSyncPaths(home, workspace, selection)
	if err == nil {
		t.Fatal("plannedSyncPaths() expected error when PI multi-mode is requested without pi-subagents")
	}

	if containsPathSuffix(planned, filepath.Join(".pi", "agents", "sdd-apply.md")) {
		t.Fatalf("plannedSyncPaths() should not include PI artifacts when pi-subagents is absent\npaths=%v", planned)
	}
}

func TestRenderSyncReportDryRunIncludesPlannedPaths(t *testing.T) {
	report := RenderSyncReport(SyncResult{
		DryRun: true,
		Agents: []model.AgentID{model.AgentPiCodingAgent},
		Selection: model.Selection{
			Components: []model.ComponentID{model.ComponentSDD},
		},
		PlannedFiles: []string{
			"/tmp/work/.pi/agents/sdd-apply.md",
			"/tmp/work/.pi/agents/sdd.chain.md",
		},
	})

	if !strings.Contains(report, "Planned paths:") {
		t.Fatalf("RenderSyncReport() missing planned paths header\nreport=%s", report)
	}
	if !strings.Contains(report, ".pi/agents/sdd-apply.md") {
		t.Fatalf("RenderSyncReport() missing planned PI agent path\nreport=%s", report)
	}
	if !strings.Contains(report, ".pi/agents/sdd.chain.md") {
		t.Fatalf("RenderSyncReport() missing planned PI chain path\nreport=%s", report)
	}
}

func containsPathSuffix(paths []string, suffix string) bool {
	for _, path := range paths {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}
