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
	}, nil, "", nil, 0, nil)

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

func TestRenderUninstallResultIncludesSelectedProfiles(t *testing.T) {
	out := RenderUninstallResult(componentuninstall.Result{}, nil, model.UninstallModePartial, []string{"cheap", "fast"}, 0, nil)

	if !strings.Contains(out, "Profiles removed") {
		t.Fatalf("RenderUninstallResult() should include profile summary heading; got:\n%s", out)
	}
	if !strings.Contains(out, "cheap") || !strings.Contains(out, "fast") {
		t.Fatalf("RenderUninstallResult() should include selected profile names; got:\n%s", out)
	}
}
