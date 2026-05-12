package screens

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/planner"
)

func TestRenderDependencyTreePiOnlyEngramPlanShowsComponentAndPiInstallCopy(t *testing.T) {
	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentPi},
		Preset:     model.PresetFullGentleman,
		Components: []model.ComponentID{model.ComponentEngram},
	}
	plan := planner.ResolvedPlan{
		Agents:            []model.AgentID{model.AgentPi},
		OrderedComponents: []model.ComponentID{model.ComponentEngram},
	}

	out := RenderDependencyTree(plan, selection, 0)

	if strings.Contains(out, "No components selected yet.") {
		t.Fatalf("RenderDependencyTree() showed generic empty copy for Pi-only Engram plan; output:\n%s", out)
	}
	for _, want := range []string{
		"Components to install",
		"engram",
		"Pi agent support will be installed.",
		"pi install npm:gentle-pi",
		"pi install npm:gentle-engram",
		"pi install npm:pi-mcp-adapter",
		"npm exec --yes --package gentle-engram -- pi-engram init",
		"pi install npm:pi-subagents",
		"pi install npm:pi-intercom",
		"pi install npm:@juicesharp/rpiv-ask-user-question",
		"pi install npm:pi-web-access",
		"pi install npm:pi-lens",
		"pi install npm:@juicesharp/rpiv-todo",
		"pi install npm:pi-btw",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("RenderDependencyTree() missing %q for Pi-only plan; output:\n%s", want, out)
		}
	}
}

func TestRenderDependencyTreeGenericEmptyPlanKeepsExistingCopy(t *testing.T) {
	selection := model.Selection{Preset: model.PresetFullGentleman}

	out := RenderDependencyTree(planner.ResolvedPlan{}, selection, 0)

	if !strings.Contains(out, "No components selected yet.") {
		t.Fatalf("RenderDependencyTree() missing generic empty copy; output:\n%s", out)
	}
	if strings.Contains(out, "Pi agent support will be installed.") {
		t.Fatalf("RenderDependencyTree() showed Pi copy for generic empty plan; output:\n%s", out)
	}
}

func TestRenderDependencyTreeMixedPiEmptyPlanShowsPiInstallCopy(t *testing.T) {
	selection := model.Selection{
		Agents: []model.AgentID{model.AgentPi, model.AgentOpenCode},
		Preset: model.PresetFullGentleman,
	}
	plan := planner.ResolvedPlan{Agents: selection.Agents}

	out := RenderDependencyTree(plan, selection, 0)

	if strings.Contains(out, "No components selected yet.") {
		t.Fatalf("RenderDependencyTree() showed generic empty copy for mixed Pi plan; output:\n%s", out)
	}
	for _, want := range []string{
		"Pi agent support will be installed.",
		"pi install npm:gentle-pi",
		"pi install npm:gentle-engram",
		"pi install npm:pi-mcp-adapter",
		"npm exec --yes --package gentle-engram -- pi-engram init",
		"pi install npm:pi-subagents",
		"pi install npm:pi-intercom",
		"pi install npm:@juicesharp/rpiv-ask-user-question",
		"pi install npm:pi-web-access",
		"pi install npm:pi-lens",
		"pi install npm:@juicesharp/rpiv-todo",
		"pi install npm:pi-btw",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("RenderDependencyTree() missing %q for mixed Pi plan; output:\n%s", want, out)
		}
	}
}
