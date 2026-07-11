package screens

import (
	"errors"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/components/opencodeplugin"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// ─── RenderOpenCodePluginUninstallSelect ────────────────────────────────────

func TestRenderOpenCodePluginUninstallSelectListsInstalledPlugins(t *testing.T) {
	installed := []model.OpenCodeCommunityPluginID{
		model.OpenCodePluginSubAgentStatusline,
		model.OpenCodePluginSDDEngramManage,
	}
	out := RenderOpenCodePluginUninstallSelect(installed, 0)

	for _, want := range []string{
		"Uninstall OpenCode Community Plugins",
		"Sub-agent Statusline",
		"SDD Engram Manager",
		"Continue",
		"Back",
		"↑/↓: navigate",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("select screen missing %q; output:\n%s", want, out)
		}
	}
}

func TestRenderOpenCodePluginUninstallSelectHighlightsCursor(t *testing.T) {
	installed := []model.OpenCodeCommunityPluginID{
		model.OpenCodePluginSubAgentStatusline,
		model.OpenCodePluginSDDEngramManage,
	}
	out := RenderOpenCodePluginUninstallSelect(installed, 1)
	if !strings.Contains(out, "▸") {
		t.Fatalf("cursor highlight should render with ▸ prefix; output:\n%s", out)
	}
}

func TestRenderOpenCodePluginUninstallSelectEmptyReturnsEmptyString(t *testing.T) {
	out := RenderOpenCodePluginUninstallSelect(nil, 0)
	if out != "" {
		t.Fatalf("empty install list should return \"\"; got:\n%s", out)
	}
}

func TestOpenCodePluginUninstallOptionCountIncludesContinueAndBack(t *testing.T) {
	installed := []model.OpenCodeCommunityPluginID{
		model.OpenCodePluginSubAgentStatusline,
		model.OpenCodePluginSDDEngramManage,
	}
	if got, want := OpenCodePluginUninstallOptionCount(installed), 4; got != want {
		t.Fatalf("OpenCodePluginUninstallOptionCount() = %d, want %d", got, want)
	}
}

// ─── RenderOpenCodePluginUninstallConfirm ───────────────────────────────────

func TestRenderOpenCodePluginUninstallConfirmIdleStateMentionsEnter(t *testing.T) {
	out := RenderOpenCodePluginUninstallConfirm(model.OpenCodePluginSDDEngramManage, false, 0)
	for _, want := range []string{
		"SDD Engram Manager",
		"Press enter to confirm",
		"esc to cancel",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("confirm screen missing %q; output:\n%s", want, out)
		}
	}
}

func TestRenderOpenCodePluginUninstallConfirmRunningShowsSpinner(t *testing.T) {
	out := RenderOpenCodePluginUninstallConfirm(model.OpenCodePluginSubAgentStatusline, true, 2)
	for _, want := range []string{
		"Uninstalling Sub-agent Statusline",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("running confirm screen missing %q; output:\n%s", want, out)
		}
	}
	if !strings.ContainsAny(out, "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏") {
		t.Fatalf("running confirm screen should include a spinner frame; output:\n%s", out)
	}
}

// ─── RenderOpenCodePluginUninstallResult ────────────────────────────────────

func TestRenderOpenCodePluginUninstallResultSuccessSurfacesLayers(t *testing.T) {
	out := RenderOpenCodePluginUninstallResult(opencodeplugin.UninstallResult{
		PluginID:           model.OpenCodePluginSubAgentStatusline,
		ChangedTUI:         true,
		ChangedPackageJSON: true,
		ChangedNodeModules: true,
		CacheEntryRemoved:  "/home/me/.cache/opencode/packages/opencode-subagent-statusline@latest",
		NodeModulesPath:    "/home/me/.config/opencode/node_modules/opencode-subagent-statusline",
	}, nil)

	for _, want := range []string{
		"Sub-agent Statusline",
		"uninstalled",
		"Layer 1",
		"Layer 2",
		"Layer 3",
		"Layer 4",
		"Return to menu",
		"enter: return to menu",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("result screen missing %q; output:\n%s", want, out)
		}
	}
}

func TestRenderOpenCodePluginUninstallResultErrorShowsErrorMessage(t *testing.T) {
	out := RenderOpenCodePluginUninstallResult(opencodeplugin.UninstallResult{}, errors.New("boom"))

	for _, want := range []string{
		"Uninstall failed",
		"boom",
		"Return to menu",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("error result screen missing %q; output:\n%s", want, out)
		}
	}
}

func TestRenderOpenCodePluginUninstallResultGentleLogoShowsTSX(t *testing.T) {
	out := RenderOpenCodePluginUninstallResult(opencodeplugin.UninstallResult{
		PluginID: model.OpenCodePluginGentleLogo,
		TSXPath:  "/home/me/.config/opencode/tui-plugins/gentle-logo.tsx",
	}, nil)
	if !strings.Contains(out, "gentle-logo.tsx") {
		t.Fatalf("GentleLogo result screen missing TSX path; output:\n%s", out)
	}
}