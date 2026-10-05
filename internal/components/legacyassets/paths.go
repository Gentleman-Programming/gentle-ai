// Package legacyassets enumerates retired managed paths for snapshots and rollback.
// It does not install or render SDD assets.
package legacyassets

import (
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// commandNames is the fixed inventory of eleven historical slash commands.
var commandNames = [...]string{
	"sdd-init", "sdd-new", "sdd-continue", "sdd-status", "sdd-explore",
	"sdd-research", "sdd-ff", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
}

var promptPhases = [...]string{
	"sdd-init", "sdd-explore", "sdd-research", "sdd-propose", "sdd-spec",
	"sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
}

// agentNames is the fixed inventory of eleven historical SDD sub-agents. v3
// wrote the same phase names as native agents, and Claude Code kept them in
// ~/.claude/agents/ after the upgrade to v4.
var agentNames = [...]string{
	"sdd-init", "sdd-explore", "sdd-research", "sdd-propose", "sdd-spec",
	"sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
}

// SlashCommandPaths inventories historical commands; Claude has both namespaced
// and earlier unprefixed files. Enumeration does not create or delete files.
func SlashCommandPaths(agent model.AgentID, commandsDir string) []string {
	paths := make([]string, 0, 2*len(commandNames))
	for _, name := range commandNames {
		if agent == model.AgentClaudeCode {
			paths = append(paths, filepath.Join(commandsDir, "gentle-"+name+".md"))
		}
		paths = append(paths, filepath.Join(commandsDir, name+".md"))
	}
	return paths
}

// SharedPromptDir is the historical OpenCode prompt location (including XDG).
func SharedPromptDir(homeDir string) string {
	return filepath.Join(opencode.ConfigPath(homeDir), "prompts", "sdd")
}

// SharedPromptPhases returns an independent copy of historical prompt names.
func SharedPromptPhases() []string {
	return append([]string(nil), promptPhases[:]...)
}

// SubAgentPaths inventories the historical SDD agents a v3 install left in a
// native agents directory. Only Claude Code used that directory; runtimes with
// no legacy agents there return no paths. Enumeration does not create or delete
// files.
func SubAgentPaths(agent model.AgentID, agentsDir string) []string {
	if agent != model.AgentClaudeCode {
		return nil
	}
	paths := make([]string, 0, len(agentNames))
	for _, name := range agentNames {
		paths = append(paths, filepath.Join(agentsDir, name+".md"))
	}
	return paths
}

// IsLegacyClaudeCommandPath matches exactly the retired unprefixed Claude files.
func IsLegacyClaudeCommandPath(path string) bool {
	path = filepath.Clean(path)
	dir := filepath.Dir(path)
	if filepath.Base(dir) != "commands" || filepath.Base(filepath.Dir(dir)) != ".claude" {
		return false
	}
	name := strings.TrimSuffix(filepath.Base(path), ".md")
	if filepath.Base(path) != name+".md" {
		return false
	}
	for _, command := range commandNames {
		if name == command {
			return true
		}
	}
	return false
}
