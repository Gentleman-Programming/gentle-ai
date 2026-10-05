// Package legacyassets enumerates retired managed paths for snapshots and
// rollback, and records the v3 prose that identifies a retired SDD sub-agent.
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

// legacySubAgents is the fixed inventory of eleven historical SDD sub-agents.
// v3 wrote the same phase names as native agents, and Claude Code kept them in
// ~/.claude/agents/ after the upgrade to v4. Each name carries the sentence
// only Gentle AI's v3 template wrote, so the installer can tell a retired
// managed agent apart from a user file that merely shares its name.
var legacySubAgents = [...]LegacySubAgent{
	{Name: "sdd-init", Marker: "You are the SDD **init** executor."},
	{Name: "sdd-explore", Marker: "You are the SDD **explore** executor."},
	{Name: "sdd-research", Marker: "You are an output-only evidence collector, not the orchestrator."},
	{Name: "sdd-propose", Marker: "You are the SDD **propose** executor."},
	{Name: "sdd-spec", Marker: "You are the SDD **spec** executor."},
	{Name: "sdd-design", Marker: "You are the SDD **design** executor."},
	{Name: "sdd-tasks", Marker: "You are the SDD **tasks** executor."},
	{Name: "sdd-apply", Marker: "You are the SDD **apply** executor."},
	{Name: "sdd-verify", Marker: "You are the SDD **verify** executor."},
	{Name: "sdd-archive", Marker: "You are the SDD **archive** executor."},
	{Name: "sdd-onboard", Marker: "You are the SDD **onboard** executor."},
}

// LegacySubAgent is one retired v3 SDD agent: the file name it was written
// under, and the sentence that identifies Gentle AI's v3 template.
type LegacySubAgent struct {
	Name   string
	Marker string
}

// SubAgents returns an independent copy of the retired SDD sub-agent
// inventory. The marker is embedded v3 prose deliberately: it is the only
// ownership evidence available for files a release predating the current
// ownership ledger wrote.
func SubAgents() []LegacySubAgent {
	return append([]LegacySubAgent(nil), legacySubAgents[:]...)
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
