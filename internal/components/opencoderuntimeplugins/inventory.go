// Package opencoderuntimeplugins owns the names and eligibility of managed
// OpenCode-compatible runtime plugins. Filesystem lifecycle remains with callers.
package opencoderuntimeplugins

import "github.com/gentleman-programming/gentle-ai/v4/internal/model"

const LegacyOpenCodeReviewPluginName = "review-result-artifacts.ts"

// ManagedOpenCodePluginNames returns the versioned OpenCode runtime inventory.
func ManagedOpenCodePluginNames() []string { return ManagedPluginNames(model.AgentOpenCode) }

// ManagedPluginNames returns the plugins installed for an eligible agent.
func ManagedPluginNames(agent model.AgentID) []string {
	switch agent {
	case model.AgentOpenCode:
		return []string{"model-variants.ts", "opencode-review-transport.ts", "skill-registry.ts"}
	case model.AgentKilocode:
		return []string{"model-variants.ts", "skill-registry.ts"}
	default:
		return nil
	}
}

// OpenCodePluginLifecycleNames is every plugin name Install can write or
// remove for the agent: the managed inventory plus its retired plugins.
// Backup, sync, and uninstall derive their plugin paths from it.
func OpenCodePluginLifecycleNames(agent model.AgentID) []string {
	return append(ManagedPluginNames(agent), retiredPluginNames(agent)...)
}

// retiredPluginNames are earlier managed plugins Install removes, per agent
// that received them: background-agents.ts (OpenCode v1.7.0 to v1.37.2,
// Kilocode v1.19.0 to v1.37.2), review-result-artifacts.ts (both, v2.1.7 to
// v2.4.0-rc.7), and sdd-task-result-artifacts.ts (OpenCode only, v2.4.0-rc.8
// to v3.7.0).
// RetiredPluginNames returns the retired managed plugins for agent; a
// completed install or sync leaves none of them on disk.
func RetiredPluginNames(agent model.AgentID) []string { return retiredPluginNames(agent) }

func retiredPluginNames(agent model.AgentID) []string {
	switch agent {
	case model.AgentOpenCode:
		return []string{"background-agents.ts", LegacyOpenCodeReviewPluginName, "sdd-task-result-artifacts.ts"}
	case model.AgentKilocode:
		return []string{"background-agents.ts", LegacyOpenCodeReviewPluginName}
	default:
		return nil
	}
}

// AgentReceivesManagedOpenCodePlugins identifies compatible runtimes.
func AgentReceivesManagedOpenCodePlugins(agent model.AgentID) bool {
	return agent == model.AgentOpenCode || agent == model.AgentKilocode
}
