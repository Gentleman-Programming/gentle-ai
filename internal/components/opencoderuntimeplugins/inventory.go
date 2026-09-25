// Package opencoderuntimeplugins owns the names and eligibility of managed
// OpenCode-compatible runtime plugins. Filesystem lifecycle remains with callers.
package opencoderuntimeplugins

import "github.com/gentleman-programming/gentle-ai/v3/internal/model"

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

// OpenCodePluginLifecycleNames includes the retired review plugin for cleanup.
func OpenCodePluginLifecycleNames(agent model.AgentID) []string {
	names := ManagedPluginNames(agent)
	if AgentReceivesManagedOpenCodePlugins(agent) {
		names = append(names, LegacyOpenCodeReviewPluginName)
	}
	return names
}

// AgentReceivesManagedOpenCodePlugins identifies compatible runtimes.
func AgentReceivesManagedOpenCodePlugins(agent model.AgentID) bool {
	return agent == model.AgentOpenCode || agent == model.AgentKilocode
}
