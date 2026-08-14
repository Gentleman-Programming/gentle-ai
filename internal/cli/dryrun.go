package cli

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/planner"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

func RenderDryRun(result InstallResult) string {
	b := &strings.Builder{}

	_, _ = fmt.Fprintln(b, "AI Gentle Stack dry-run")
	_, _ = fmt.Fprintln(b, "=====================")
	_, _ = fmt.Fprintf(b, "Agents: %s\n", joinAgentIDs(result.Resolved.Agents))
	_, _ = fmt.Fprintf(b, "Unsupported agents: %s\n", joinAgentIDs(result.Resolved.UnsupportedAgents))
	_, _ = fmt.Fprintf(b, "Persona: %s\n", result.Selection.Persona)
	_, _ = fmt.Fprintf(b, "Preset: %s\n", result.Selection.Preset)
	if result.Selection.SDDMode != "" {
		_, _ = fmt.Fprintf(b, "SDD mode: %s\n", result.Selection.SDDMode)
	}
	_, _ = fmt.Fprintf(b, "Components order: %s\n", joinComponentIDs(result.Resolved.OrderedComponents))
	_, _ = fmt.Fprintf(b, "Auto-added dependencies: %s\n", joinComponentIDs(result.Resolved.AddedDependencies))
	_, _ = fmt.Fprintf(b, "Platform decision: %s\n", formatPlatformDecision(result.Review.PlatformDecision))
	_, _ = fmt.Fprintf(b, "Prepare steps: %d\n", len(result.Plan.Prepare))
	_, _ = fmt.Fprintf(b, "Apply steps: %d\n", len(result.Plan.Apply))
	if containsAgent(result.Resolved.Agents, model.AgentOpenCode) {
		_, _ = fmt.Fprintf(b, "OpenCode background intent: %s (policy effective: %s)\n", result.Background.Intent, result.Background.Effective)
	}
	if containsAgent(result.Resolved.Agents, model.AgentOpenCode) && result.Background.Activation.Action != "" {
		if result.Background.Effective == model.OpenCodeBackgroundOn {
			_, _ = fmt.Fprintf(b, "OpenCode background runtime ready: %t\n", result.BackgroundPolicyEnabled)
		}
		_, _ = fmt.Fprintln(b, renderOpenCodeBackgroundActivation(result.Background))
	}

	if len(result.Dependencies.Dependencies) > 0 {
		_, _ = fmt.Fprintln(b, "")
		_, _ = fmt.Fprintln(b, system.RenderDependencyReport(result.Dependencies))
	}

	return strings.TrimRight(b.String(), "\n")
}

func joinAgentIDs(values []model.AgentID) string {
	if len(values) == 0 {
		return "none"
	}

	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, string(value))
	}
	return strings.Join(parts, ",")
}

func joinComponentIDs(values []model.ComponentID) string {
	if len(values) == 0 {
		return "none"
	}

	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, string(value))
	}
	return strings.Join(parts, ",")
}

func formatPlatformDecision(decision planner.PlatformDecision) string {
	osName := decision.OS
	if strings.TrimSpace(osName) == "" {
		osName = "unknown"
	}

	distro := decision.LinuxDistro
	if strings.TrimSpace(distro) == "" {
		distro = "n/a"
	}

	manager := decision.PackageManager
	if strings.TrimSpace(manager) == "" {
		manager = "n/a"
	}

	status := "unsupported"
	if decision.Supported {
		status = "supported"
	}

	return fmt.Sprintf("os=%s distro=%s package-manager=%s status=%s", osName, distro, manager, status)
}
