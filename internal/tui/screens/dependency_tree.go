package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/planner"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
	"github.com/gentleman-programming/gentle-ai/internal/versions"
)

func DependencyTreeOptions() []string {
	return []string{"Continue", "Back"}
}

// AllComponents returns the full list of available components for the custom picker.
func AllComponents() []catalog.Component {
	return catalog.MVPComponents()
}

// RenderDependencyTree shows the install plan. For custom presets, it shows
// toggleable checkboxes; for other presets it shows a read-only ordered list.
func RenderDependencyTree(plan planner.ResolvedPlan, selection model.Selection, cursor int) string {
	if selection.Preset == model.PresetCustom {
		return renderCustomPicker(selection, cursor)
	}

	return renderPresetPlan(plan, selection, cursor)
}

func renderPresetPlan(plan planner.ResolvedPlan, selection model.Selection, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Install Plan"))
	b.WriteString("\n\n")

	if len(plan.OrderedComponents) == 0 {
		if !hasPiAgentInInstallPlan(plan, selection) {
			b.WriteString(styles.WarningStyle.Render("No components selected yet."))
			b.WriteString("\n")
		}
		b.WriteString("\n\n")
	} else {
		b.WriteString(styles.HeadingStyle.Render("Components to install"))
		b.WriteString("\n")

		autoSet := make(map[model.ComponentID]struct{}, len(plan.AddedDependencies))
		for _, auto := range plan.AddedDependencies {
			autoSet[auto] = struct{}{}
		}

		descMap := componentDescriptions()

		for idx, component := range plan.OrderedComponents {
			num := styles.SubtextStyle.Render(fmt.Sprintf("%d.", idx+1))
			name := styles.UnselectedStyle.Render(string(component))
			note := styles.SuccessStyle.Render("included")
			if _, isAuto := autoSet[component]; isAuto {
				note = styles.WarningStyle.Render("auto-dependency")
			}
			b.WriteString(fmt.Sprintf("  %s %s %s\n", num, name, note))
			if desc, ok := descMap[component]; ok {
				b.WriteString(styles.SubtextStyle.Render(fmt.Sprintf("     %s", desc)) + "\n")
			}
		}
		b.WriteString("\n")
	}

	if hasPiAgentInInstallPlan(plan, selection) {
		b.WriteString(renderPiInstallPlan())
		b.WriteString("\n")
	}

	b.WriteString(renderOptions(DependencyTreeOptions(), cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}

func hasPiAgentInInstallPlan(plan planner.ResolvedPlan, selection model.Selection) bool {
	agents := selection.Agents
	if len(agents) == 0 {
		agents = plan.Agents
	}

	for _, agent := range agents {
		if agent == model.AgentPi {
			return true
		}
	}
	return false
}

func piInstallCommands() []string {
	return []string{
		"pi install npm:gentle-pi",
		"pi install npm:gentle-engram",
		"pi install npm:pi-mcp-adapter",
		fmt.Sprintf("npm exec --yes --package gentle-engram@%s -- pi-engram init", versions.GentleEngram),
		"pi install npm:pi-subagents",
		"pi install npm:pi-intercom",
		"pi install npm:@juicesharp/rpiv-ask-user-question",
		"pi install npm:pi-web-access",
		"pi install npm:pi-lens",
		"pi install npm:@juicesharp/rpiv-todo",
		"pi install npm:pi-btw",
	}
}

func renderPiInstallPlan() string {
	var b strings.Builder
	b.WriteString(styles.SuccessStyle.Render("Pi agent support will be installed."))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("  • Engram component will be installed/provisioned."))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("  • Pi package stack will be installed:"))
	b.WriteString("\n")
	for _, command := range piInstallCommands() {
		b.WriteString(styles.SubtextStyle.Render(fmt.Sprintf("    - %s", command)))
		b.WriteString("\n")
	}
	return b.String()
}

func renderCustomPicker(selection model.Selection, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Select Components"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Toggle components with enter or space."))
	b.WriteString("\n\n")

	allComps := AllComponents()
	selectedSet := make(map[model.ComponentID]struct{}, len(selection.Components))
	for _, c := range selection.Components {
		selectedSet[c] = struct{}{}
	}

	for idx, comp := range allComps {
		_, checked := selectedSet[comp.ID]
		focused := idx == cursor
		b.WriteString(renderCheckbox(string(comp.ID), checked, focused))
		b.WriteString(styles.SubtextStyle.Render("    "+comp.Description) + "\n")
	}

	b.WriteString("\n")
	actionOffset := cursor - len(allComps)
	b.WriteString(renderOptions(DependencyTreeOptions(), actionOffset))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • space/enter: toggle • esc: back"))

	return b.String()
}

func componentDescriptions() map[model.ComponentID]string {
	comps := catalog.MVPComponents()
	m := make(map[model.ComponentID]string, len(comps))
	for _, c := range comps {
		m[c.ID] = c.Description
	}
	return m
}
