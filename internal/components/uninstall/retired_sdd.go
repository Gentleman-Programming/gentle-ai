package uninstall

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	opencodeagent "github.com/gentleman-programming/gentle-ai/v4/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/legacyassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// retiredSDDOperation retires what releases installed for SDD for a runtime
// uninstall removes completely, with the inventory and ownership proof
// install and sync use (#5157): the legacyassets inventory, the native SDD
// agents, and the OpenCode family's settings entries and shared prompts.
// Bytes no release wrote are kept and reported, and a directory that is not a
// real directory is never entered. It returns every path it may change, for
// the snapshot. Pi has nothing to retire: gentle-pi owns its home (#5219).
func (s *Service) retiredSDDOperation(adapter agents.Adapter) (operation, []string, bool) {
	agent := adapter.Agent()
	if agent == model.AgentPi {
		return operation{}, nil, false
	}
	inventory := legacyassets.RetiredSDDInventory(adapter, s.homeDir)
	// Uninstall delivers no routing guidance, so nothing migrates the SDD
	// block of the active prompt: it is retired there too.
	for i := range inventory.Blocks {
		inventory.Blocks[i].Active = ""
	}
	if adapter.SupportsSystemPrompt() {
		prompt := adapter.SystemPromptFile(s.homeDir)
		inventory.Blocks = append(inventory.Blocks, legacyassets.SDDPromptFile{Root: filepath.Dir(prompt), Path: prompt})
	}
	targets := inventory.BackupPaths()
	var agentsDir string
	if len(legacyassets.RetiredSDDAgentFiles(agent)) > 0 {
		agentsDir = adapter.SubAgentsDir(s.homeDir)
		if realDirectory(agentsDir) {
			targets = append(targets, presentPaths(legacyassets.RetiredSDDAgentPaths(agent, agentsDir))...)
		}
	}
	var settings, agentDirs []string
	var prompts string
	switch agent {
	case model.AgentOpenCode:
		config := opencodeagent.ConfigPath(s.homeDir)
		agentDirs = []string{filepath.Join(config, "agent"), filepath.Join(config, "agents")}
		prompts = legacyassets.SharedPromptDir(s.homeDir)
		fallthrough
	case model.AgentKilocode:
		settings = settingsTargets(s.homeDir, adapter)
	}
	targets = append(targets, settings...)
	if prompts != "" && realDirectory(prompts) {
		for _, phase := range legacyassets.SharedPromptPhases() {
			targets = append(targets, presentPaths([]string{filepath.Join(prompts, phase+".md")})...)
		}
	}

	var removed, rewritten, actions []string
	op := operation{typeID: opRetireSDD, path: adapter.GlobalConfigDir(s.homeDir), agents: []model.AgentID{agent}}
	op.apply = func(string) (bool, bool, error) {
		res, err := inventory.Retire()
		removed, rewritten, actions = append(removed, res.Removed...), append(rewritten, res.Rewritten...), append(actions, res.ManualActions...)
		if err != nil {
			return false, false, err
		}
		if agentsDir != "" {
			if err := retireSDDAgents(agent, agentsDir, &removed, &actions); err != nil {
				return false, false, err
			}
		}
		for _, path := range settings {
			res, err := legacyassets.RetireOpenCodeSDDSettings(path, agentDirs...)
			if res.Changed {
				rewritten = append(rewritten, path)
			}
			if err != nil {
				return false, false, fmt.Errorf("retire SDD agents in %q settings: %w", agent, err)
			}
			actions = append(actions, res.ManualActions()...)
		}
		if prompts != "" {
			res, err := legacyassets.RetireOpenCodeSDDPrompts(prompts)
			removed = append(removed, res.Removed...)
			if err != nil {
				return false, false, fmt.Errorf("retire SDD prompts for %q: %w", agent, err)
			}
			actions = append(actions, res.ManualActions()...)
		}
		return len(removed)+len(rewritten) > 0, false, nil
	}
	op.report = func(result *Result) {
		result.RemovedFiles = append(result.RemovedFiles, removed...)
		for _, path := range rewritten {
			if !slices.Contains(result.ChangedFiles, path) {
				result.ChangedFiles = append(result.ChangedFiles, path)
			}
		}
		result.ManualActions = append(result.ManualActions, actions...)
	}
	return op, targets, true
}

// retireSDDAgents retires the native SDD agents in dir. A dir that is not a
// real directory (a dotfiles symlink, for example) is never entered: it is
// reported when it holds a retired SDD agent file.
func retireSDDAgents(agent model.AgentID, dir string, removed, actions *[]string) error {
	info, err := os.Lstat(dir)
	switch {
	case os.IsNotExist(err):
		return nil
	case err != nil:
		return fmt.Errorf("inspect agents directory %s: %w", dir, err)
	case !info.IsDir():
		for _, path := range legacyassets.RetiredSDDAgentPaths(agent, dir) {
			if _, err := os.Stat(path); err == nil {
				*actions = append(*actions, fmt.Sprintf("%s is not a real directory (for example a symlink), so Gentle AI did not inspect or remove the retired SDD agents behind it. SDD was retired in v4.0.0 and those agents are no longer maintained; if you no longer need them, move or delete them yourself.", dir))
				break
			}
		}
		return nil
	}
	res, err := legacyassets.RetireSDDAgents(agent, dir)
	*removed = append(*removed, res.Removed...)
	if err != nil {
		return fmt.Errorf("retire SDD agents for %q: %w", agent, err)
	}
	*actions = append(*actions, res.ManualActions()...)
	return nil
}

// realDirectory reports whether path is a directory and not a link to one.
// A missing path holds nothing to snapshot.
func realDirectory(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
}

func presentPaths(paths []string) []string {
	var present []string
	for _, path := range paths {
		if _, err := os.Lstat(path); err == nil {
			present = append(present, path)
		}
	}
	return present
}
