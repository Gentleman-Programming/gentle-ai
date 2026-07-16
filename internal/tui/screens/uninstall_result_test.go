package screens

import (
	"strings"
	"testing"

	componentuninstall "github.com/gentleman-programming/gentle-ai/internal/components/uninstall"
	"github.com/gentleman-programming/gentle-ai/internal/model"
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

func TestUninstallEngramScopeOptions_NoCleanupFirst(t *testing.T) {
	withProject := UninstallEngramScopeOptions(true)
	if len(withProject) != 3 {
		t.Fatalf("with project: len = %d, want 3", len(withProject))
	}
	if withProject[0].Scope != model.EngramUninstallScopeNone || withProject[0].Label != "No cleanup" {
		t.Fatalf("first option = %+v, want No cleanup / none", withProject[0])
	}
	if withProject[1].Scope != model.EngramUninstallScopeProject {
		t.Fatalf("second option scope = %q, want project", withProject[1].Scope)
	}
	if withProject[2].Scope != model.EngramUninstallScopeGlobal {
		t.Fatalf("third option scope = %q, want global", withProject[2].Scope)
	}

	withoutProject := UninstallEngramScopeOptions(false)
	if len(withoutProject) != 2 {
		t.Fatalf("without project: len = %d, want 2", len(withoutProject))
	}
	if withoutProject[0].Scope != model.EngramUninstallScopeNone {
		t.Fatalf("without project first = %q, want none", withoutProject[0].Scope)
	}
	if withoutProject[1].Scope != model.EngramUninstallScopeGlobal {
		t.Fatalf("without project second = %q, want global", withoutProject[1].Scope)
	}
}

func TestRenderUninstallProfiles_ShowsNoCleanupWhenEngramScopeVisible(t *testing.T) {
	out := RenderUninstallProfiles([]string{"cheap"}, nil, true, true, model.EngramUninstallScopeNone, 0)

	if !strings.Contains(out, "No cleanup") {
		t.Fatalf("RenderUninstallProfiles() should show No cleanup; got:\n%s", out)
	}
	if !strings.Contains(out, "Project-only cleanup") {
		t.Fatalf("RenderUninstallProfiles() should show Project-only when project data available; got:\n%s", out)
	}
	if !strings.Contains(out, "Global cleanup") {
		t.Fatalf("RenderUninstallProfiles() should show Global cleanup; got:\n%s", out)
	}
}

func TestRenderUninstallProfiles_HidesEngramWhenNotShown(t *testing.T) {
	out := RenderUninstallProfiles([]string{"cheap"}, nil, false, false, model.EngramUninstallScopeNone, 0)

	if strings.Contains(out, "No cleanup") || strings.Contains(out, "Select Engram cleanup scope") {
		t.Fatalf("RenderUninstallProfiles() should hide Engram scope when showEngramScope=false; got:\n%s", out)
	}
}

func TestRenderUninstallConfirm_NoneScopeTruthful(t *testing.T) {
	out := RenderUninstallConfirm(
		model.UninstallModePartial,
		[]model.AgentID{model.AgentOpenCode},
		[]model.ComponentID{model.ComponentEngram, model.ComponentSDD},
		nil,
		model.EngramUninstallScopeNone,
		true,
		0,
		false,
		0,
	)

	if !strings.Contains(out, "Engram cleanup scope") {
		t.Fatalf("confirm should include Engram scope heading; got:\n%s", out)
	}
	if !strings.Contains(out, "None") {
		t.Fatalf("confirm should label none scope as None; got:\n%s", out)
	}
	if strings.Contains(out, "Removes global Engram") {
		t.Fatalf("confirm for none must not imply Global cleanup; got:\n%s", out)
	}
	if strings.Contains(out, "• .engram/") {
		t.Fatalf("confirm for none must not warn that .engram/ will be deleted; got:\n%s", out)
	}
}

func TestRenderUninstallResult_NoneScopeNeverImpliesGlobal(t *testing.T) {
	// Even if result somehow listed engram paths, none must not print Global.
	out := RenderUninstallResult(componentuninstall.Result{
		RemovedDirectories: []string{"/tmp/workspace/.engram"},
	}, nil, model.UninstallModePartial, nil, model.EngramUninstallScopeNone, true, nil, nil)

	if strings.Contains(out, "Engram scope: Global") {
		t.Fatalf("result for none must not claim Global; got:\n%s", out)
	}
}
