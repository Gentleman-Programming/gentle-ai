package legacyassets

import (
	"fmt"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// SDDInventory is what releases installed for one runtime under one config
// root besides native agents and OpenCode-family settings: SDD skills, slash
// commands, workflows, Codex profiles, the Kimi module, the Claude Code
// preflight hook, and the SDD text left in prompt files. Install, sync, and
// uninstall snapshot (BackupPaths) and retire (Retire) exactly this
// inventory.
type SDDInventory struct {
	// Agent is empty for the shared ~/.agents/skills root.
	Agent model.AgentID
	Dirs  SDDAssetDirs
	// Settings is the Claude Code settings.json holding the preflight hook.
	Settings string
	// Blocks are prompt files whose SDD orchestrator block is removed.
	Blocks []SDDPromptFile
	// KimiHub is the legacy Kimi router that included the SDD module.
	KimiHub SDDPromptFile
}

// SDDPromptFile is a prompt file holding retired SDD text, edited only when
// every directory from Root down to it is a real directory, with the Active
// prompt routing guidance migrates instead (empty when none).
type SDDPromptFile struct{ Root, Path, Active string }

// RetiredSDDInventory resolves the inventory of adapter under root, the home
// or workspace directory releases resolved its config from. Workflows and
// scope-specific prompt files are the caller's to add. Pi has none: gentle-pi
// owns its home (#5219), so nothing there is ever inspected.
func RetiredSDDInventory(adapter agents.Adapter, root string) SDDInventory {
	inv := SDDInventory{Agent: adapter.Agent()}
	if adapter.Agent() == model.AgentPi {
		return inv
	}
	if adapter.SupportsSkills() {
		inv.Dirs.Skills = adapter.SkillsDir(root)
	}
	if adapter.SupportsSlashCommands() {
		inv.Dirs.Commands = adapter.CommandsDir(root)
	}
	if adapter.Agent() == model.AgentClaudeCode {
		inv.Settings = adapter.SettingsPath(root)
	}
	files := RetiredSDDRuntimeFiles(adapter.Agent(), root)
	inv.Dirs.CodexHome, inv.Dirs.KimiHome = files.Dirs.CodexHome, files.Dirs.KimiHome
	if files.Hub != "" {
		inv.KimiHub = SDDPromptFile{Root: files.Dirs.KimiHome, Path: files.Hub}
	}
	if files.Prompt != "" {
		inv.Blocks = append(inv.Blocks, SDDPromptFile{Root: files.Dirs.CodexHome, Path: files.Prompt, Active: files.Active})
	}
	return inv
}

// BackupPaths lists what Retire may change: the present inventory files, the
// Claude Code settings, and every prompt file it may rewrite. Nothing behind
// a directory that is not a real directory is listed.
func (inv SDDInventory) BackupPaths() []string {
	paths := PresentRetiredSDDAssetPaths(inv.Agent, inv.Dirs)
	if inv.Settings != "" {
		paths = append(paths, inv.Settings)
	}
	for _, prompt := range append([]SDDPromptFile{inv.KimiHub}, inv.Blocks...) {
		if prompt.Path != "" && RetirablePromptFile(prompt.Root, prompt.Path) {
			paths = append(paths, prompt.Path)
		}
	}
	return paths
}

// InventoryRetireResult lists the files a retirement removed and rewrote, and
// what the user must do with everything it kept.
type InventoryRetireResult struct {
	Removed, Rewritten []string
	ManualActions      []string
}

// Retire removes the inventory files whose bytes a release installed, the
// released preflight hook, and the retired SDD text of the prompt files. On
// error the result still lists what was already changed.
func (inv SDDInventory) Retire() (InventoryRetireResult, error) {
	var result InventoryRetireResult
	assets, err := RetireSDDAssets(inv.Agent, inv.Dirs)
	result.Removed = assets.Removed
	if err != nil {
		return result, fmt.Errorf("retire SDD files for %q: %w", inv.Agent, err)
	}
	result.ManualActions = assets.ManualActions()
	if inv.Settings != "" {
		hook, err := RetireClaudeSDDPreflightHook(inv.Settings)
		if err != nil {
			return result, fmt.Errorf("retire SDD preflight hook: %w", err)
		}
		if hook.Removed {
			result.Rewritten = append(result.Rewritten, inv.Settings)
		}
		result.ManualActions = append(result.ManualActions, hook.ManualActions()...)
	}
	texts := make([]TextRetireResult, 0, len(inv.Blocks)+1)
	for _, block := range inv.Blocks {
		res, err := RetireSDDOrchestratorBlock(block.Root, block.Path, block.Active)
		if err != nil {
			return result, fmt.Errorf("retire SDD orchestrator block: %w", err)
		}
		texts = append(texts, res)
	}
	if inv.KimiHub.Path != "" {
		res, err := RetireKimiSDDInclude(inv.KimiHub.Root, inv.KimiHub.Path)
		if err != nil {
			return result, fmt.Errorf("retire Kimi SDD include: %w", err)
		}
		texts = append(texts, res)
	}
	for _, res := range texts {
		if res.Removed {
			result.Rewritten = append(result.Rewritten, res.Path)
		}
		result.ManualActions = append(result.ManualActions, res.ManualActions()...)
	}
	return result, nil
}
