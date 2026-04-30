package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/catalog"
	componentuninstall "github.com/gentleman-programming/gentle-ai/internal/components/uninstall"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

type UninstallModeOption struct {
	Mode        model.UninstallMode
	Label       string
	Description string
}

type UninstallEngramScopeOption struct {
	Scope       model.EngramUninstallScope
	Label       string
	Description string
}

func UninstallModeOptions() []UninstallModeOption {
	return []UninstallModeOption{
		{
			Mode:        model.UninstallModePartial,
			Label:       "Partial Uninstall",
			Description: "Select specific agents and components to remove",
		},
		{
			Mode:        model.UninstallModeFull,
			Label:       "Full Uninstall",
			Description: "Remove all gentle-ai managed configuration from all agents",
		},
		{
			Mode:        model.UninstallModeFullRemove,
			Label:       "Full Uninstall & Remove Binary",
			Description: "Remove all configuration AND delete the gentle-ai binary itself",
		},
		{
			Mode:        model.UninstallModeCleanInstall,
			Label:       "Full Uninstall + Clean Install",
			Description: "Remove all configuration, then re-sync all managed assets from scratch",
		},
	}
}

func RenderUninstallMode(cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Uninstall Mode Selection"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Choose how you want to uninstall gentle-ai:"))
	b.WriteString("\n\n")

	options := UninstallModeOptions()
	for idx, opt := range options {
		focused := idx == cursor
		if focused {
			b.WriteString(styles.SelectedStyle.Render("▸ " + opt.Label))
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  " + opt.Label))
		}
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("  " + opt.Description))
		b.WriteString("\n")
		if opt.Mode == model.UninstallModeFullRemove {
			b.WriteString(styles.ErrorStyle.Render("  ⚠ WARNING: This cannot be undone without reinstalling"))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(renderOptions([]string{"Back"}, cursor-len(options)))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}

func UninstallAgentOptions() []catalog.Agent {
	return catalog.AllAgents()
}

func UninstallComponentOptions() []catalog.Component {
	return catalog.MVPComponents()
}

func RenderUninstall(selected []model.AgentID, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Uninstall Managed Configs"))
	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("Use j/k to move, space to toggle, enter to continue."))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Select the agents whose gentle-ai managed configuration should be removed."))
	b.WriteString("\n\n")

	selectedSet := make(map[model.AgentID]struct{}, len(selected))
	for _, agent := range selected {
		selectedSet[agent] = struct{}{}
	}

	for idx, agent := range UninstallAgentOptions() {
		_, checked := selectedSet[agent.ID]
		focused := idx == cursor
		b.WriteString(renderCheckbox(agent.Name, checked, focused))
	}

	b.WriteString("\n")
	agentCount := len(UninstallAgentOptions())
	relCursor := cursor - agentCount
	if len(selected) == 0 {
		// Render Continue as dimmed with an inline hint when nothing is selected.
		if relCursor == 0 {
			b.WriteString(styles.SelectedStyle.Render(styles.Cursor+styles.SubtextStyle.Render("Continue")+" "+styles.HelpStyle.Render("(select at least one agent)")) + "\n")
		} else {
			b.WriteString(styles.SubtextStyle.Render("  Continue") + "\n")
		}
		b.WriteString(renderOptions([]string{"Back"}, relCursor-1))
	} else {
		b.WriteString(renderOptions([]string{"Continue", "Back"}, relCursor))
	}
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("space: toggle • enter: confirm • esc: back"))

	return b.String()
}

func RenderUninstallComponents(selected []model.ComponentID, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Uninstall Managed Components"))
	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("Use j/k to move, space to toggle, enter to continue."))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Select which gentle-ai managed components should be removed from the selected agents."))
	b.WriteString("\n\n")

	selectedSet := make(map[model.ComponentID]struct{}, len(selected))
	for _, component := range selected {
		selectedSet[component] = struct{}{}
	}

	for idx, component := range UninstallComponentOptions() {
		_, checked := selectedSet[component.ID]
		focused := idx == cursor
		b.WriteString(renderCheckbox(component.Name, checked, focused))
		b.WriteString(styles.SubtextStyle.Render("    "+component.Description) + "\n")
	}

	b.WriteString("\n")
	compCount := len(UninstallComponentOptions())
	relCursor := cursor - compCount
	if len(selected) == 0 {
		// Render Continue as dimmed with an inline hint when nothing is selected.
		if relCursor == 0 {
			b.WriteString(styles.SelectedStyle.Render(styles.Cursor+styles.SubtextStyle.Render("Continue")+" "+styles.HelpStyle.Render("(select at least one component)")) + "\n")
		} else {
			b.WriteString(styles.SubtextStyle.Render("  Continue") + "\n")
		}
		b.WriteString(renderOptions([]string{"Back"}, relCursor-1))
	} else {
		b.WriteString(renderOptions([]string{"Continue", "Back"}, relCursor))
	}
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("space: toggle • enter: continue • esc: back"))

	return b.String()
}

func uninstallEngramScopeOptions(projectScopeAvailable bool) []UninstallEngramScopeOption {
	options := make([]UninstallEngramScopeOption, 0, 2)
	if projectScopeAvailable {
		options = append(options, UninstallEngramScopeOption{
			Scope:       model.EngramUninstallScopeProject,
			Label:       "Project-only cleanup",
			Description: "Delete only .engram/ in the current project",
		})
	}
	options = append(options, UninstallEngramScopeOption{
		Scope:       model.EngramUninstallScopeGlobal,
		Label:       "Global cleanup",
		Description: "Remove global Engram MCP/system prompt integration",
	})
	return options
}

func RenderUninstallProfiles(available []string, selected []string, engramProjectScopeAvailable bool, selectedEngramScope model.EngramUninstallScope, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Uninstall Scope Selection"))
	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("Use j/k to move, space to toggle/select, enter to continue."))
	b.WriteString("\n\n")

	if len(available) > 0 {
		b.WriteString(styles.SubtextStyle.Render("Choose which OpenCode SDD profiles should be removed from opencode.json."))
		b.WriteString("\n\n")
	}

	selectedSet := make(map[string]struct{}, len(selected))
	for _, profile := range selected {
		selectedSet[profile] = struct{}{}
	}

	for idx, profileName := range available {
		_, checked := selectedSet[profileName]
		focused := idx == cursor
		b.WriteString(renderCheckbox(profileName, checked, focused))
	}

	engramScopeOptions := uninstallEngramScopeOptions(engramProjectScopeAvailable)
	engramScopeDisplayed := 0
	if len(engramScopeOptions) > 1 {
		engramScopeDisplayed = len(engramScopeOptions)
		if len(available) > 0 {
			b.WriteString("\n")
		}
		b.WriteString(styles.SubtextStyle.Render("Select Engram cleanup scope:"))
		b.WriteString("\n")
		for idx, option := range engramScopeOptions {
			focused := len(available)+idx == cursor
			checked := selectedEngramScope == option.Scope
			b.WriteString(renderCheckbox(option.Label, checked, focused))
			b.WriteString(styles.SubtextStyle.Render("    " + option.Description))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	relCursor := cursor - (len(available) + engramScopeDisplayed)
	b.WriteString(renderOptions([]string{"Continue", "Back"}, relCursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("space: toggle/select • enter: continue • esc: back"))

	return b.String()
}

func RenderUninstallConfirm(mode model.UninstallMode, selected []model.AgentID, components []model.ComponentID, profilesToRemove []string, engramScope model.EngramUninstallScope, engramProjectScopeAvailable bool, cursor int, operationRunning bool, spinnerFrame int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Confirm Uninstall"))
	b.WriteString("\n\n")

	if operationRunning {
		b.WriteString(styles.WarningStyle.Render(SpinnerChar(spinnerFrame) + "  Removing managed configuration..."))
		b.WriteString("\n\n")
		b.WriteString(styles.HelpStyle.Render("Please wait..."))
		return b.String()
	}

	// Render mode-specific information
	switch mode {
	case model.UninstallModePartial:
		if len(selected) == 0 {
			b.WriteString(styles.WarningStyle.Render("No agents selected."))
			b.WriteString("\n\n")
			b.WriteString(styles.HelpStyle.Render("enter: back • esc: back"))
			return b.String()
		}
		b.WriteString(styles.SubtextStyle.Render("Mode: Partial Uninstall"))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("Agents:"))
		b.WriteString("\n")
		for _, label := range uninstallAgentLabels(selected) {
			b.WriteString(styles.UnselectedStyle.Render("  • " + label))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("Components:"))
		b.WriteString("\n")
		for _, label := range uninstallComponentLabels(components) {
			b.WriteString(styles.UnselectedStyle.Render("  • " + label))
			b.WriteString("\n")
		}
	case model.UninstallModeFull:
		b.WriteString(styles.SubtextStyle.Render("Mode: Full Uninstall"))
		b.WriteString("\n\n")
		b.WriteString(styles.UnselectedStyle.Render("This will remove all gentle-ai managed configuration from all supported agents."))
		b.WriteString("\n")
	case model.UninstallModeFullRemove:
		b.WriteString(styles.ErrorStyle.Render("Mode: Full Uninstall & Remove Binary"))
		b.WriteString("\n\n")
		b.WriteString(styles.UnselectedStyle.Render("This will remove all gentle-ai managed configuration from all agents"))
		b.WriteString("\n")
		b.WriteString(styles.ErrorStyle.Render("AND delete the gentle-ai binary itself."))
		b.WriteString("\n\n")
		b.WriteString(styles.ErrorStyle.Render("⚠ WARNING: This action cannot be undone without reinstalling!"))
		b.WriteString("\n")
	case model.UninstallModeCleanInstall:
		b.WriteString(styles.SuccessStyle.Render("Mode: Full Uninstall + Clean Install"))
		b.WriteString("\n\n")
		b.WriteString(styles.UnselectedStyle.Render("This will remove all gentle-ai managed configuration from all agents"))
		b.WriteString("\n")
		b.WriteString(styles.SuccessStyle.Render("and immediately re-sync all managed assets from scratch."))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("Use this to fix broken configurations or reset to a clean state."))
		b.WriteString("\n")
	}

	if len(profilesToRemove) > 0 {
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("Profiles to remove:"))
		b.WriteString("\n")
		for _, profile := range profilesToRemove {
			b.WriteString(styles.UnselectedStyle.Render("  • " + profile))
			b.WriteString("\n")
		}
	}

	if hasSelectedComponent(components, model.ComponentEngram) {
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("Engram cleanup scope:"))
		b.WriteString("\n")
		scopeLabel := "Global"
		detail := "  • Removes global Engram MCP/system prompt configuration"
		if engramScope == model.EngramUninstallScopeProject && engramProjectScopeAvailable {
			scopeLabel = "Project-only"
			detail = "  • Deletes .engram/ in the current project only"
		}
		b.WriteString(styles.UnselectedStyle.Render("  • " + scopeLabel))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render(detail))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Workspace-scoped assets warning
	hasWorkspaceAssets := false
	for _, comp := range components {
		if comp == model.ComponentSDD || comp == model.ComponentSkills {
			hasWorkspaceAssets = true
			break
		}
	}
	if (mode == model.UninstallModeFull || mode == model.UninstallModeFullRemove) || hasWorkspaceAssets {
		b.WriteString(styles.WarningStyle.Render("⚠ Workspace Assets Warning:"))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("  Removing SDD or Skills will delete workspace-scoped files like:"))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("  • .windsurf/workflows/ (SDD workflows)"))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("  • .engram/ (persistent memory context)"))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("  • Skills directories"))
		b.WriteString("\n\n")
		b.WriteString(styles.ErrorStyle.Render("  If you commit these deletions, ALL collaborators will lose this context!"))
		b.WriteString("\n\n")
	}

	b.WriteString(styles.WarningStyle.Render("A backup snapshot will be created before any file is modified."))
	b.WriteString("\n\n")
	b.WriteString(renderOptions([]string{"Uninstall", "Cancel"}, cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}

func RenderUninstallResult(result componentuninstall.Result, err error, mode model.UninstallMode, selectedProfiles []string, engramScope model.EngramUninstallScope, engramProjectScopeAvailable bool, syncFilesChanged int, syncErr error) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Uninstall Result"))
	b.WriteString("\n\n")

	if err != nil {
		b.WriteString(styles.ErrorStyle.Render("✗ Uninstall failed"))
		b.WriteString("\n\n")
		b.WriteString(styles.HeadingStyle.Render("Error:"))
		b.WriteString("\n")
		b.WriteString(styles.ErrorStyle.Render("  " + err.Error()))
		b.WriteString("\n\n")
		if result.Manifest.ID != "" {
			b.WriteString(styles.SubtextStyle.Render("Backup created before failure: "))
			b.WriteString(styles.SelectedStyle.Render(result.Manifest.ID))
			b.WriteString("\n")
			b.WriteString(styles.SubtextStyle.Render(result.Manifest.DisplayLabel()))
		}
	} else {
		b.WriteString(styles.SuccessStyle.Render("✓ Uninstall complete"))
		b.WriteString("\n\n")
		if result.Manifest.ID != "" {
			b.WriteString(styles.SubtextStyle.Render("Backup: "))
			b.WriteString(styles.SelectedStyle.Render(result.Manifest.ID))
			b.WriteString("\n")
			b.WriteString(styles.SubtextStyle.Render(result.Manifest.DisplayLabel()))
			b.WriteString("\n\n")
		}
		b.WriteString(styles.UnselectedStyle.Render(fmt.Sprintf("Rewritten files: %d", len(result.ChangedFiles))))
		b.WriteString("\n")
		b.WriteString(styles.UnselectedStyle.Render(fmt.Sprintf("Deleted files: %d", len(result.RemovedFiles))))
		b.WriteString("\n")
		b.WriteString(styles.UnselectedStyle.Render(fmt.Sprintf("Deleted directories: %d", len(result.RemovedDirectories))))
		if len(result.AgentsRemovedFromState) > 0 {
			b.WriteString("\n")
			b.WriteString(styles.UnselectedStyle.Render("Updated state.json: " + strings.Join(uninstallAgentLabels(result.AgentsRemovedFromState), ", ")))
		}
		if len(result.ManualActions) > 0 {
			b.WriteString("\n\n")
			b.WriteString(styles.WarningStyle.Render("Manual cleanup required:"))
			for _, item := range result.ManualActions {
				b.WriteString("\n")
				b.WriteString(styles.UnselectedStyle.Render("  • " + item))
			}
		}

		if len(selectedProfiles) > 0 {
			b.WriteString("\n\n")
			b.WriteString(styles.UnselectedStyle.Render("Profiles removed: " + strings.Join(selectedProfiles, ", ")))
		}

		if hasEngramArtifacts(result) {
			b.WriteString("\n\n")
			if engramScope == model.EngramUninstallScopeProject && engramProjectScopeAvailable {
				b.WriteString(styles.UnselectedStyle.Render("Engram scope: Project-only (.engram/ removed from current workspace)"))
			} else {
				b.WriteString(styles.UnselectedStyle.Render("Engram scope: Global (MCP/system prompt integration removed)"))
			}
		}

		// Clean install: show sync results after uninstall stats.
		if mode == model.UninstallModeCleanInstall {
			b.WriteString("\n\n")
			if syncErr != nil {
				b.WriteString(styles.ErrorStyle.Render("✗ Clean install sync failed"))
				b.WriteString("\n")
				b.WriteString(styles.ErrorStyle.Render("  " + syncErr.Error()))
				b.WriteString("\n\n")
				b.WriteString(styles.WarningStyle.Render("You can run 'gentle-ai sync' manually to retry."))
			} else {
				b.WriteString(styles.SuccessStyle.Render("✓ Clean install sync complete"))
				b.WriteString("\n")
				b.WriteString(styles.UnselectedStyle.Render(fmt.Sprintf("Synced files: %d", syncFilesChanged)))
			}
		}
	}

	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("enter: return • esc: back • q: quit"))
	return b.String()
}

func hasEngramArtifacts(result componentuninstall.Result) bool {
	for _, path := range result.ChangedFiles {
		if strings.Contains(path, "engram") {
			return true
		}
	}
	for _, path := range result.RemovedFiles {
		if strings.Contains(path, "engram") {
			return true
		}
	}
	for _, path := range result.RemovedDirectories {
		if strings.Contains(path, ".engram") {
			return true
		}
	}
	return false
}

func uninstallAgentLabels(agentIDs []model.AgentID) []string {
	labels := make([]string, 0, len(agentIDs))
	for _, selected := range agentIDs {
		labels = append(labels, uninstallAgentLabel(selected))
	}
	return labels
}

func uninstallAgentLabel(agentID model.AgentID) string {
	for _, agent := range UninstallAgentOptions() {
		if agent.ID == agentID {
			return agent.Name
		}
	}
	return string(agentID)
}

func uninstallComponentLabels(componentIDs []model.ComponentID) []string {
	labels := make([]string, 0, len(componentIDs))
	for _, selected := range componentIDs {
		labels = append(labels, uninstallComponentLabel(selected))
	}
	return labels
}

func uninstallComponentLabel(componentID model.ComponentID) string {
	for _, component := range UninstallComponentOptions() {
		if component.ID == componentID {
			return component.Name
		}
	}
	return string(componentID)
}

func hasSelectedComponent(components []model.ComponentID, target model.ComponentID) bool {
	for _, component := range components {
		if component == target {
			return true
		}
	}
	return false
}
