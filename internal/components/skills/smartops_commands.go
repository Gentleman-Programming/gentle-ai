package skills

import (
	"fmt"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// smartopsCommandAssetDir is the embedded directory holding the standalone
// smartops-ui OpenCode slash-command file. It is a separate domain from
// qaCommandAssetDir (skills.qa_commands.go): smartops-ui has no ledger/change
// concept at all, and the user explicitly required its own asset directory
// and injector rather than merging into the QA command registry.
//
// Like qaCommandAssetDir, it is deliberately NOT under "opencode/commands/"
// (see qaCommandAssetDir's comment for why), to keep the
// "only write what's selected" discipline InjectSmartopsUICommands
// implements below.
const smartopsCommandAssetDir = "opencode/smartops-commands"

// smartopsStandaloneCommands maps each standalone smartops-ui skill to its
// embedded OpenCode slash-command asset file name.
var smartopsStandaloneCommands = map[model.SkillID]string{
	model.SkillSmartopsUI: "smartops-ui.md",
}

// smartopsCommandOrder is smartopsStandaloneCommands's iteration order, kept
// separate because Go map iteration is unordered and both
// InjectSmartopsUICommands and SmartopsUICommandPaths need a deterministic
// result.
var smartopsCommandOrder = []model.SkillID{
	model.SkillSmartopsUI,
}

// InjectSmartopsUICommands writes OpenCode slash-command files for the
// standalone smartops-ui skill(s) present in skillIDs. It mirrors
// InjectQACommands's "only write what's selected" discipline: an unselected
// skill or an ineligible adapter (Claude Code, or any adapter without a
// commands directory) writes nothing.
func InjectSmartopsUICommands(homeDir string, adapter agents.Adapter, skillIDs []model.SkillID) (InjectionResult, error) {
	// Claude Code is excluded for the same reason InjectQACommands excludes
	// it: it already resolves a same-named skill directory as
	// /<skill-name> natively.
	if adapter.Agent() == model.AgentClaudeCode || !adapter.SupportsSlashCommands() {
		return InjectionResult{}, nil
	}
	commandsDir := adapter.CommandsDir(homeDir)
	if commandsDir == "" {
		return InjectionResult{}, nil
	}

	selected := make(map[model.SkillID]bool, len(skillIDs))
	for _, id := range skillIDs {
		selected[id] = true
	}

	result := InjectionResult{}
	for _, id := range smartopsCommandOrder {
		if !selected[id] {
			continue
		}
		fileName := smartopsStandaloneCommands[id]
		content, err := assets.Read(smartopsCommandAssetDir + "/" + fileName)
		if err != nil {
			return InjectionResult{}, fmt.Errorf("read embedded %q: %w", smartopsCommandAssetDir+"/"+fileName, err)
		}
		path := filepath.Join(commandsDir, fileName)
		writeResult, err := filemerge.WriteFileAtomic(path, []byte(content), 0o644)
		if err != nil {
			return InjectionResult{}, fmt.Errorf("write %q: %w", path, err)
		}
		result.Changed = result.Changed || writeResult.Changed
		result.Files = append(result.Files, path)
	}
	return result, nil
}

// SmartopsUICommandPaths lists the OpenCode smartops-ui command files that
// InjectSmartopsUICommands may write for the given selection, so
// install/uninstall backup and rollback machinery can track them without a
// live filesystem listing. It applies the exact same gating
// InjectSmartopsUICommands does, so the two never disagree about which paths
// exist.
func SmartopsUICommandPaths(adapter agents.Adapter, homeDir string, skillIDs []model.SkillID) []string {
	if adapter.Agent() == model.AgentClaudeCode || !adapter.SupportsSlashCommands() {
		return nil
	}
	commandsDir := adapter.CommandsDir(homeDir)
	if commandsDir == "" {
		return nil
	}

	selected := make(map[model.SkillID]bool, len(skillIDs))
	for _, id := range skillIDs {
		selected[id] = true
	}

	var paths []string
	for _, id := range smartopsCommandOrder {
		if !selected[id] {
			continue
		}
		paths = append(paths, filepath.Join(commandsDir, smartopsStandaloneCommands[id]))
	}
	return paths
}
