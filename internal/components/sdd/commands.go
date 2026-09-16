package sdd

import (
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

type OpenCodeCommand struct {
	Name        string
	Description string
	Body        string
}

func OpenCodeCommands() []OpenCodeCommand {
	return []OpenCodeCommand{
		{Name: "sdd-init", Description: "Initialize SDD context", Body: "/sdd-init"},
		{Name: "sdd-new", Description: "Start a new SDD change", Body: "/sdd-new ${change-name}"},
		{Name: "sdd-continue", Description: "Continue next pending artifact", Body: "/sdd-continue ${change-name}"},
		{Name: "sdd-status", Description: "Show SDD change status", Body: "/sdd-status ${change-name}"},
		{Name: "sdd-explore", Description: "Explore an idea before committing", Body: "/sdd-explore ${topic}"},
		{Name: "sdd-research", Description: "Research external evidence", Body: "/sdd-research ${change-name}"},
		{Name: "sdd-ff", Description: "Generate all planning artifacts", Body: "/sdd-ff ${change-name}"},
		{Name: "sdd-apply", Description: "Implement tasks", Body: "/sdd-apply ${change-name}"},
		{Name: "sdd-verify", Description: "Verify implementation", Body: "/sdd-verify ${change-name}"},
		{Name: "sdd-archive", Description: "Archive completed change", Body: "/sdd-archive ${change-name}"},
		{Name: "sdd-onboard", Description: "Guided SDD walkthrough", Body: "/sdd-onboard"},
	}
}

// SlashCommandFileName returns the managed command file for one SDD command on
// the given runtime.
func SlashCommandFileName(agent model.AgentID, name string) string {
	return name + ".md"
}

// SlashCommandPaths lists the managed SDD command files under commandsDir for
// one runtime. For Claude Code it also lists the legacy gentle-* names an older
// install wrote, so rollback snapshots capture them and verification expects
// their absence once install or sync retires them.
func SlashCommandPaths(agent model.AgentID, commandsDir string) []string {
	paths := make([]string, 0, 2*len(OpenCodeCommands()))
	for _, command := range OpenCodeCommands() {
		fileName := SlashCommandFileName(agent, command.Name)
		paths = append(paths, filepath.Join(commandsDir, fileName))
		if legacy := LegacyClaudeCommandPath(agent, commandsDir, fileName); legacy != "" {
			paths = append(paths, legacy)
		}
	}
	return paths
}

// LegacyClaudeCommandPath returns the legacy prefixed path (gentle-sdd-*.md) that
// previous installs wrote for the Claude Code command fileName (sdd-*.md), or ""
// when the runtime has no retired predecessor.
func LegacyClaudeCommandPath(agent model.AgentID, commandsDir, fileName string) string {
	if agent != model.AgentClaudeCode || strings.HasPrefix(fileName, "gentle-") {
		return ""
	}
	return filepath.Join(commandsDir, "gentle-"+fileName)
}

// IsLegacyClaudeCommandPath reports whether path names a retired prefixed
// Claude Code SDD command file (gentle-sdd-*.md), which install and sync remove.
func IsLegacyClaudeCommandPath(path string) bool {
	path = filepath.Clean(path)
	commandsDir := filepath.Dir(path)
	if filepath.Base(commandsDir) != "commands" || filepath.Base(filepath.Dir(commandsDir)) != ".claude" {
		return false
	}
	for _, command := range OpenCodeCommands() {
		if filepath.Base(path) == "gentle-"+command.Name+".md" {
			return true
		}
	}
	return false
}
