package skills

import (
	"fmt"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v3/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// qaCommandAssetDir is the embedded directory holding the standalone QA tool
// OpenCode slash-command files.
//
// It is deliberately NOT under "opencode/commands/" (the directory
// sdd.Inject's "2. Write slash commands" step enumerates via fs.ReadDir and
// writes wholesale for every adapter that supports slash commands): SDD is
// installed independently of which skills a user selected, so any file
// placed there would be written unconditionally, defeating the
// "only write what's selected" discipline InjectQACommands implements below
// (the same discipline skills.Inject already applies to SKILL.md files) and
// breaking the exact-count assertion in
// TestOpenCodeEmbeddedAssetLayout (internal/assets/assets_test.go).
const qaCommandAssetDir = "opencode/qa-commands"

// qaStandaloneCommands maps each standalone (non-ledger-bound) QA tool skill
// to its embedded OpenCode slash-command asset file name.
//
// The 5 ledger-bound QA stages (qa-explore, qa-spec, qa-apply, qa-verify,
// qa-docs) are deliberately absent: they depend on QAStateMachine/{change}
// orchestration that qa-supervisor already routes through
// `gentle-ai qa-status`, and a same-named /qa-explore (etc.) command would
// let a user skip that routing entirely.
var qaStandaloneCommands = map[model.SkillID]string{
	model.SkillQASupervisor:     "qa-supervisor.md",
	model.SkillQALocatorHunting: "qa-locator-hunting.md",
	model.SkillQADocReference:   "qa-doc-reference.md",
	model.SkillQADocAccess:      "qa-doc-access.md",
	model.SkillQAEvidence:       "qa-evidence.md",
}

// qaCommandOrder is qaStandaloneCommands's iteration order, kept separate
// because Go map iteration is unordered and both InjectQACommands and
// QACommandPaths need a deterministic result.
var qaCommandOrder = []model.SkillID{
	model.SkillQASupervisor,
	model.SkillQALocatorHunting,
	model.SkillQADocReference,
	model.SkillQADocAccess,
	model.SkillQAEvidence,
}

// InjectQACommands writes OpenCode slash-command files for the standalone QA
// tool skills present in skillIDs, one file per selected tool. It mirrors
// skills.Inject's "only write what's selected" discipline: an unselected
// standalone tool, a ledger-bound stage skill, or an ineligible adapter
// (Claude Code, or any adapter without a commands directory — Codex and
// Antigravity both return "" from CommandsDir today) writes nothing.
func InjectQACommands(homeDir string, adapter agents.Adapter, skillIDs []model.SkillID) (InjectionResult, error) {
	// Claude Code is excluded even though it supports slash commands and has
	// a non-empty CommandsDir: it already resolves a same-named skill
	// directory as /<skill-name> natively — exactly why SDD prefixes its own
	// commands gentle-sdd-* (see sdd.claudeCommandPrefix), to dodge that same
	// native resolution. QA tools want the native resolution, not a
	// workaround for it, so Claude Code gets no command file at all here.
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
	for _, id := range qaCommandOrder {
		if !selected[id] {
			continue
		}
		fileName := qaStandaloneCommands[id]
		content, err := assets.Read(qaCommandAssetDir + "/" + fileName)
		if err != nil {
			return InjectionResult{}, fmt.Errorf("read embedded %q: %w", qaCommandAssetDir+"/"+fileName, err)
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

// QACommandPaths lists the OpenCode QA standalone command files that
// InjectQACommands may write for the given selection, so install/uninstall
// backup and rollback machinery can track them without a live filesystem
// listing — the skills-component analogue of sdd.SlashCommandPaths. It
// applies the exact same gating InjectQACommands does, so the two never
// disagree about which paths exist.
func QACommandPaths(adapter agents.Adapter, homeDir string, skillIDs []model.SkillID) []string {
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
	for _, id := range qaCommandOrder {
		if !selected[id] {
			continue
		}
		paths = append(paths, filepath.Join(commandsDir, qaStandaloneCommands[id]))
	}
	return paths
}
