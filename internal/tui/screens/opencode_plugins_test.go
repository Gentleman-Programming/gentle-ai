package screens

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func TestRenderOpenCodePluginsShowsInstallAndRepoOptions(t *testing.T) {
	out := RenderOpenCodePlugins([]model.OpenCodeCommunityPluginID{model.OpenCodePluginSubAgentStatusline}, 0)

	for _, want := range []string{
		"Optional OpenCode Community Plugins",
		"Sub-agent Statusline",
		"View repo",
		"Continue",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("RenderOpenCodePlugins missing %q; output:\n%s", want, out)
		}
	}
	if strings.Contains(out, "SDD Engram Manager") || strings.Contains(out, "sdd-engram-plugin") {
		t.Fatalf("RenderOpenCodePlugins offers retired plugin; output:\n%s", out)
	}
	if !strings.Contains(out, "[x]") {
		t.Fatalf("RenderOpenCodePlugins should show selected checkbox; output:\n%s", out)
	}
}
