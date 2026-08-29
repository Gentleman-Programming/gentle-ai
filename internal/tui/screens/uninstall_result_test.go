package screens

import (
	"strings"
	"testing"

	componentuninstall "github.com/gentleman-programming/gentle-ai/v2/internal/components/uninstall"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestRenderUninstallResultIncludesManualCleanup(t *testing.T) {
	out := RenderUninstallResult(componentuninstall.Result{
		RemovedDirectories: []string{"/tmp/skills"},
		ManualActions: []string{
			"Remove manually if no longer needed: /tmp/skills (directory still contains non-managed files)",
		},
	}, nil, "", nil, model.EngramUninstallScopeGlobal, false, nil, nil)

	if !strings.Contains(out, "Manual cleanup required") {
		t.Fatalf("RenderUninstallResult() should include manual cleanup heading; got:\n%s", out)
	}
	if !strings.Contains(out, "/tmp/skills") {
		t.Fatalf("RenderUninstallResult() should include manual cleanup item; got:\n%s", out)
	}
}

func TestRenderUninstallConfirmIncludesSelectedProfiles(t *testing.T) {
	out := RenderUninstallConfirm(
		model.UninstallModePartial,
		[]model.AgentID{model.AgentOpenCode},
		[]model.ComponentID{model.ComponentSDD},
		[]string{"cheap"},
		model.EngramUninstallScopeGlobal,
		false,
		0,
		false,
		0,
	)

	if !strings.Contains(out, "Profiles to remove") {
		t.Fatalf("RenderUninstallConfirm() should include profile section; got:\n%s", out)
	}
	if !strings.Contains(out, "cheap") {
		t.Fatalf("RenderUninstallConfirm() should include selected profile name; got:\n%s", out)
	}
}

func TestRenderUninstallConfirmIncludesEngramProjectScopeDetails(t *testing.T) {
	out := RenderUninstallConfirm(
		model.UninstallModePartial,
		[]model.AgentID{model.AgentOpenCode},
		[]model.ComponentID{model.ComponentEngram},
		nil,
		model.EngramUninstallScopeProject,
		true,
		0,
		false,
		0,
	)

	if !strings.Contains(out, "Engram cleanup scope") {
		t.Fatalf("RenderUninstallConfirm() should include Engram cleanup scope heading; got:\n%s", out)
	}
	if !strings.Contains(out, "Project-only") {
		t.Fatalf("RenderUninstallConfirm() should include project-only scope label; got:\n%s", out)
	}
	if !strings.Contains(out, ".engram/") {
		t.Fatalf("RenderUninstallConfirm() should mention .engram project data removal; got:\n%s", out)
	}
}

func TestRenderUninstallResultIncludesSelectedProfiles(t *testing.T) {
	out := RenderUninstallResult(componentuninstall.Result{}, nil, model.UninstallModePartial, []string{"cheap", "fast"}, model.EngramUninstallScopeGlobal, false, nil, nil)

	if !strings.Contains(out, "Profiles removed") {
		t.Fatalf("RenderUninstallResult() should include profile summary heading; got:\n%s", out)
	}
	if !strings.Contains(out, "cheap") || !strings.Contains(out, "fast") {
		t.Fatalf("RenderUninstallResult() should include selected profile names; got:\n%s", out)
	}
}

func TestRenderUninstallResultIncludesEngramScopeSummary(t *testing.T) {
	out := RenderUninstallResult(componentuninstall.Result{
		RemovedDirectories: []string{"/tmp/workspace/.engram"},
	}, nil, model.UninstallModePartial, nil, model.EngramUninstallScopeProject, true, nil, nil)

	if !strings.Contains(out, "Engram scope: Project-only") {
		t.Fatalf("RenderUninstallResult() should include Engram project scope summary; got:\n%s", out)
	}
}

func TestRenderUninstallConfirmEngramWorkspaceWarningMatchesCleanupEffect(t *testing.T) {
	tests := []struct {
		name             string
		components       []model.ComponentID
		profiles         []string
		scope            model.EngramUninstallScope
		projectAvailable bool
		wantWarning      bool
	}{
		{name: "Engram-only project cleanup", components: []model.ComponentID{model.ComponentEngram}, scope: model.EngramUninstallScopeProject, projectAvailable: true, wantWarning: true},
		{name: "no cleanup", components: []model.ComponentID{model.ComponentEngram}, scope: model.EngramUninstallScopeNone, projectAvailable: true},
		{name: "project cleanup with unrelated selection", components: []model.ComponentID{model.ComponentPersona}, scope: model.EngramUninstallScopeProject, projectAvailable: true, wantWarning: true},
		{name: "global cleanup", components: []model.ComponentID{model.ComponentEngram}, scope: model.EngramUninstallScopeGlobal, projectAvailable: true},
		{name: "profile-only removal", components: []model.ComponentID{model.ComponentSDD, model.ComponentEngram}, profiles: []string{"cheap"}, scope: model.EngramUninstallScopeNone, projectAvailable: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := RenderUninstallConfirm(
				model.UninstallModePartial,
				[]model.AgentID{model.AgentOpenCode},
				tt.components,
				tt.profiles,
				tt.scope,
				tt.projectAvailable,
				0,
				false,
				0,
			)

			gotWarning := strings.Contains(out, "• .engram/ (persistent memory context)")
			if gotWarning != tt.wantWarning {
				t.Fatalf(".engram warning present = %t, want %t; got:\n%s", gotWarning, tt.wantWarning, out)
			}
		})
	}
}
