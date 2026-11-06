package cli

import (
	"fmt"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/geminimodules"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
)

// geminiLegacyRootSteps runs before every prompt writer when Gemini CLI is
// selected: it moves the Gentle AI content older releases wrote into
// GEMINI.md into the staging file the writers now target (#1989).
func geminiLegacyRootSteps(id, homeDir, workspaceDir string, scope InstallScope, agentIDs []model.AgentID, changedFiles *[]string) []pipeline.Step {
	return geminiModulesStep(id, homeDir, workspaceDir, scope, agentIDs, changedFiles, geminimodules.MigrateLegacyRoot, "move Gentle AI guidance out of GEMINI.md")
}

// geminiModulesSteps runs after every prompt writer when Gemini CLI is
// selected: it publishes the staged guidance as modules that GEMINI.md
// imports, so Antigravity, which loads GEMINI.md without expanding imports,
// no longer receives a duplicated, truncated copy.
func geminiModulesSteps(id, homeDir, workspaceDir string, scope InstallScope, agentIDs []model.AgentID, changedFiles *[]string) []pipeline.Step {
	return geminiModulesStep(id, homeDir, workspaceDir, scope, agentIDs, changedFiles, geminimodules.Publish, "publish Gemini CLI guidance modules")
}

func geminiModulesStep(id, homeDir, workspaceDir string, scope InstallScope, agentIDs []model.AgentID, changedFiles *[]string, run func(string) (geminimodules.Result, error), action string) []pipeline.Step {
	if !containsAgent(agentIDs, model.AgentGeminiCLI) {
		return nil
	}
	baseDir := ResolveAgentConfigDir(scope, homeDir, workspaceDir)
	return []pipeline.Step{&publishedGuidanceStep{
		id:           id,
		dir:          geminimodules.ModulesDir(baseDir),
		paths:        func() []string { return geminimodules.ManagedPaths(baseDir) },
		changedFiles: changedFiles,
		publish: func() ([]string, error) {
			result, err := run(baseDir)
			if err != nil {
				return result.Files, fmt.Errorf("%s: %w", action, err)
			}
			return result.Files, nil
		},
	}}
}

// geminiModulesBackupPaths lists GEMINI.md, the staging file and the modules
// the Gemini CLI steps may write or remove.
func geminiModulesBackupPaths(homeDir, workspaceDir string, scope InstallScope, agentIDs []model.AgentID) []string {
	if !containsAgent(agentIDs, model.AgentGeminiCLI) {
		return nil
	}
	return geminimodules.ManagedPaths(ResolveAgentConfigDir(scope, homeDir, workspaceDir))
}
