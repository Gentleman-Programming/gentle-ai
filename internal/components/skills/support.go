package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// Support assets are the non-skill files that installed skills depend on:
// the skills/_shared references (skill-resolver, review-ledger-contract, ...)
// and the OpenCode-compatible slash commands that front skill-creator and
// skill-registry. Up to v3.7.0 the SDD component installed them; its
// retirement dropped them by accident while the installed skills still point
// at them (#4471). The skills component now owns them.

const (
	// runtimeAgentIDPlaceholder is the identity slot the shared review ledger
	// contract carries; it is bound to the runtime that receives the bytes.
	runtimeAgentIDPlaceholder = "{{GENTLE_AI_RUNTIME_AGENT_ID}}"
	sharedDirName             = "_shared"
	skillCommandsAssetDir     = "opencode/commands"
	// compatibilityRuntimeSlot binds the shared ~/.agents/skills root, which
	// every runtime reads, to a generic slot instead of one runtime identity.
	compatibilityRuntimeSlot = "<runtime>"
)

// internalSharedSources are embedded under skills/_shared as source material
// for rendered prompts only. No installed text references them and v3.7.0
// never installed them.
var internalSharedSources = map[string]bool{"odd-orchestrator-sections.md": true}

// SharedReferencePaths returns every skills/_shared file the skills
// component writes into skillDir.
func SharedReferencePaths(skillDir string) ([]string, error) {
	references, err := sharedReferences(skillDir)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(references))
	for _, reference := range references {
		paths = append(paths, reference.destination)
	}
	return paths, nil
}

func sharedReferences(skillDir string) ([]directoryAsset, error) {
	names, err := assets.SharedSkillFileNames()
	if err != nil {
		return nil, fmt.Errorf("resolve shared skill references: %w", err)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("resolve shared skill references: embedded %s listing is empty", assets.SharedSkillDir)
	}
	references := make([]directoryAsset, 0, len(names))
	for _, name := range names {
		if internalSharedSources[name] {
			continue
		}
		references = append(references, directoryAsset{
			source:      assets.SharedSkillDir + "/" + name,
			destination: filepath.Join(skillDir, sharedDirName, filepath.FromSlash(name)),
		})
	}
	return references, nil
}

// LegacySharedMarkerPath is the obsolete generated file that made _shared look
// like an invokable skill. Install removes it; snapshots must include it.
func LegacySharedMarkerPath(skillDir string) string {
	return filepath.Join(skillDir, sharedDirName, "SKILL.md")
}

// SkillCommandPaths returns the slash-command files the skills component
// writes for the selected skills. Claude Code has no retained skill commands.
func SkillCommandPaths(homeDir string, adapter agents.Adapter, skillIDs []model.SkillID) ([]string, error) {
	commands, err := skillCommands(homeDir, adapter, skillIDs)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(commands))
	for _, command := range commands {
		paths = append(paths, command.destination)
	}
	return paths, nil
}

// SupportFilePaths returns every support file InjectWithCapability writes for
// the adapter: shared references plus selected skill commands.
func SupportFilePaths(homeDir string, adapter agents.Adapter, skillIDs []model.SkillID) ([]string, error) {
	if !adapter.SupportsSkills() {
		return nil, nil
	}
	skillDir := adapter.SkillsDir(homeDir)
	if skillDir == "" {
		return nil, nil
	}
	paths, err := SharedReferencePaths(skillDir)
	if err != nil {
		return nil, err
	}
	commands, err := SkillCommandPaths(homeDir, adapter, skillIDs)
	if err != nil {
		return nil, err
	}
	return append(paths, commands...), nil
}

// AllSkillCommandPaths returns every skill command gentle-ai may own for the
// adapter, independent of the current selection. Uninstall and upgrade
// snapshots use it so a deselected skill's command is still accounted for.
func AllSkillCommandPaths(homeDir string, adapter agents.Adapter) ([]string, error) {
	return SkillCommandPaths(homeDir, adapter, AllSkillIDs())
}

func skillCommands(homeDir string, adapter agents.Adapter, skillIDs []model.SkillID) ([]directoryAsset, error) {
	if !adapter.SupportsSlashCommands() || adapter.Agent() == model.AgentClaudeCode {
		return nil, nil
	}
	commandsDir := adapter.CommandsDir(homeDir)
	if commandsDir == "" {
		return nil, nil
	}
	entries, err := fs.ReadDir(assets.FS, skillCommandsAssetDir)
	if err != nil {
		return nil, fmt.Errorf("read embedded %s: %w", skillCommandsAssetDir, err)
	}
	selected := make(map[model.SkillID]bool, len(skillIDs))
	for _, id := range skillIDs {
		selected[id] = true
	}
	var commands []directoryAsset
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".md") || !selected[model.SkillID(strings.TrimSuffix(name, ".md"))] {
			continue
		}
		commands = append(commands, directoryAsset{
			source:      skillCommandsAssetDir + "/" + name,
			destination: filepath.Join(commandsDir, name),
		})
	}
	return commands, nil
}

// injectSupportFiles writes the shared references and skill commands for one
// runtime, binding the runtime identity the shared review contract states.
func injectSupportFiles(homeDir string, adapter agents.Adapter, skillDir string, skillIDs []model.SkillID) (InjectionResult, error) {
	result, err := injectSharedReferences(skillDir, string(adapter.Agent()), filemerge.WriteFileAtomic)
	if err != nil {
		return InjectionResult{}, err
	}
	removed, err := removeLegacySharedMarker(LegacySharedMarkerPath(skillDir))
	if err != nil {
		return InjectionResult{}, err
	}
	if removed {
		result.Changed = true
		result.Files = append(result.Files, LegacySharedMarkerPath(skillDir))
	}

	commands, err := skillCommands(homeDir, adapter, skillIDs)
	if err != nil {
		return InjectionResult{}, err
	}
	for _, command := range commands {
		content, err := assets.Read(command.source)
		if err != nil {
			return InjectionResult{}, fmt.Errorf("read %q: %w", command.source, err)
		}
		if err := result.write(filemerge.WriteFileAtomic, command.destination, content); err != nil {
			return InjectionResult{}, err
		}
	}
	return result, nil
}

// injectSharedReferences writes every installable skills/_shared reference
// through writeFile, binding the review contract's runtime placeholder.
func injectSharedReferences(skillDir, runtime string, writeFile func(string, []byte, fs.FileMode) (filemerge.WriteResult, error)) (InjectionResult, error) {
	result := InjectionResult{}
	references, err := sharedReferences(skillDir)
	if err != nil {
		return InjectionResult{}, err
	}
	for _, reference := range references {
		content, err := assets.Read(reference.source)
		if err != nil {
			return InjectionResult{}, fmt.Errorf("read %q: %w", reference.source, err)
		}
		if len(content) == 0 {
			return InjectionResult{}, fmt.Errorf("embedded asset %q is empty", reference.source)
		}
		content = strings.ReplaceAll(content, runtimeAgentIDPlaceholder, runtime)
		if err := result.write(writeFile, reference.destination, content); err != nil {
			return InjectionResult{}, err
		}
	}
	return result, nil
}

func (r *InjectionResult) write(writeFile func(string, []byte, fs.FileMode) (filemerge.WriteResult, error), path, content string) error {
	written, err := writeFile(path, []byte(content), 0o644)
	if err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	r.Changed = r.Changed || written.Changed
	r.Files = append(r.Files, path)
	return nil
}

// removeLegacySharedMarker removes only the obsolete regular marker file. It
// never touches README.md or other shared references, and refuses non-regular
// paths rather than following or removing them.
func removeLegacySharedMarker(markerPath string) (bool, error) {
	info, err := os.Lstat(markerPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat legacy shared skill marker %s: %w", markerPath, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("legacy shared skill marker %s is not a regular file", markerPath) // refusal:by-design world-action: replace or remove the non-regular legacy marker before refreshing shared skill references
	}
	if err := os.Remove(markerPath); err != nil {
		return false, fmt.Errorf("remove legacy shared skill marker %s: %w", markerPath, err)
	}
	return true, nil
}
