package agentbuilder

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/codex"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/gemini"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/opencode"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

var saveRegistry = SaveRegistry

// ErrAgentNotFound indicates that the requested agent name does not exist in the custom agent registry.
var ErrAgentNotFound = errors.New("agent not found in registry")

// UninstallResult captures the backend uninstall outcome for one custom agent.
type UninstallResult struct {
	RemovedPaths  []string
	SkippedAgents []model.AgentID
}

type stagedKind int

const (
	stagedFile stagedKind = iota
	stagedSymlink
)

type stagedItem struct {
	agentID   model.AgentID
	parent    *os.Root
	sub       *os.Root
	kind      stagedKind
	name      string
	tmp       string
	finalPath string
}

func rollbackStaged(items []stagedItem) {
	for i := len(items) - 1; i >= 0; i-- {
		it := items[i]
		if it.kind == stagedSymlink {
			_ = it.parent.Rename(it.tmp, it.name)
		} else if it.kind == stagedFile {
			_ = it.sub.Rename(it.tmp, "SKILL.md")
		}
		if it.sub != nil {
			_ = it.sub.Close()
		}
		if it.parent != nil {
			_ = it.parent.Close()
		}
	}
}

func commitStaged(items []stagedItem) ([]string, int, error) {
	var removed []string
	for i, it := range items {
		if it.kind == stagedSymlink {
			if err := it.parent.Remove(it.tmp); err != nil {
				return removed, i, fmt.Errorf("remove %s: %w", it.finalPath, err)
			}
			removed = append(removed, it.finalPath)
		} else if it.kind == stagedFile {
			if err := it.sub.Remove(it.tmp); err != nil {
				return removed, i, fmt.Errorf("remove %s: %w", it.finalPath, err)
			}
			removed = append(removed, it.finalPath)
			_ = it.sub.Close()
			_ = it.parent.Remove(it.name)
		}
		if it.parent != nil {
			_ = it.parent.Close()
		}
	}
	return removed, len(items), nil
}

func stageSkillRemoval(skillsDir, targetName string) (*stagedItem, error) {
	parentRoot, err := os.OpenRoot(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat target: %w", err)
	}

	fi, err := parentRoot.Lstat(targetName)
	if err != nil {
		_ = parentRoot.Close()
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat target: %w", err)
	}

	skillDir := filepath.Join(skillsDir, targetName)

	if fi.Mode()&os.ModeSymlink != 0 {
		tmpName := targetName + ".uninstall-tmp"
		if err := parentRoot.Rename(targetName, tmpName); err != nil {
			_ = parentRoot.Close()
			return nil, fmt.Errorf("remove %s: %w", skillDir, err)
		}
		return &stagedItem{
			parent:    parentRoot,
			kind:      stagedSymlink,
			name:      targetName,
			tmp:       tmpName,
			finalPath: skillDir,
		}, nil
	}

	subRoot, err := parentRoot.OpenRoot(targetName)
	if err != nil {
		if lfi, lerr := parentRoot.Lstat(targetName); lerr == nil && lfi.Mode()&os.ModeSymlink != 0 {
			tmpName := targetName + ".uninstall-tmp"
			if rerr := parentRoot.Rename(targetName, tmpName); rerr != nil {
				_ = parentRoot.Close()
				return nil, fmt.Errorf("remove %s: %w", skillDir, rerr)
			}
			return &stagedItem{
				parent:    parentRoot,
				kind:      stagedSymlink,
				name:      targetName,
				tmp:       tmpName,
				finalPath: skillDir,
			}, nil
		}
		_ = parentRoot.Close()
		return nil, fmt.Errorf("stat %s: %w", skillDir, err)
	}

	subStat, err := subRoot.Stat(".")
	if err != nil {
		_ = subRoot.Close()
		_ = parentRoot.Close()
		return nil, fmt.Errorf("stat %s: %w", skillDir, err)
	}
	latestLstat, err := parentRoot.Lstat(targetName)
	if err != nil {
		_ = subRoot.Close()
		_ = parentRoot.Close()
		return nil, fmt.Errorf("stat %s: %w", skillDir, err)
	}
	if latestLstat.Mode()&os.ModeSymlink != 0 || !os.SameFile(subStat, latestLstat) {
		_ = subRoot.Close()
		tmpName := targetName + ".uninstall-tmp"
		if err := parentRoot.Rename(targetName, tmpName); err != nil {
			_ = parentRoot.Close()
			return nil, fmt.Errorf("remove %s: %w", skillDir, err)
		}
		return &stagedItem{
			parent:    parentRoot,
			kind:      stagedSymlink,
			name:      targetName,
			tmp:       tmpName,
			finalPath: skillDir,
		}, nil
	}

	skillFile := filepath.Join(skillDir, "SKILL.md")
	sfi, err := subRoot.Lstat("SKILL.md")
	if err != nil {
		_ = subRoot.Close()
		_ = parentRoot.Remove(targetName)
		_ = parentRoot.Close()
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat %s: %w", skillFile, err)
	}

	if sfi.IsDir() {
		_ = subRoot.Close()
		_ = parentRoot.Close()
		return nil, fmt.Errorf("remove %s: payload is a directory", skillFile)
	}

	tmpSkill := "SKILL.md.uninstall-tmp"
	if err := subRoot.Rename("SKILL.md", tmpSkill); err != nil {
		_ = subRoot.Close()
		_ = parentRoot.Close()
		return nil, fmt.Errorf("remove %s: %w", skillFile, err)
	}

	return &stagedItem{
		parent:    parentRoot,
		sub:       subRoot,
		kind:      stagedFile,
		name:      targetName,
		tmp:       tmpSkill,
		finalPath: skillFile,
	}, nil
}

// Uninstall removes the exact SKILL.md files owned by that entry, then
// removes and saves the registry entry.
func Uninstall(registryPath, agentName, homeDir string) (UninstallResult, error) {
	if registryPath == "" || agentName == "" || homeDir == "" {
		return UninstallResult{}, fmt.Errorf("uninstall: registryPath, agentName, and homeDir are required")
	}

	registry, err := LoadRegistry(registryPath)
	if err != nil {
		return UninstallResult{}, fmt.Errorf("uninstall: load registry: %w", err)
	}

	entry := registry.FindByName(agentName)
	if entry == nil {
		return UninstallResult{}, fmt.Errorf("uninstall: %w: %q", ErrAgentNotFound, agentName)
	}

	targetName := entry.Name
	if err := validateTargetName(targetName); err != nil {
		return UninstallResult{}, fmt.Errorf("uninstall: invalid registry entry name %q: %w", targetName, err)
	}
	installedAgents := append([]model.AgentID(nil), entry.InstalledAgents...)

	result := UninstallResult{}
	skillsDirs := supportedSkillsDirs(homeDir)
	var staged []stagedItem

	for _, agentID := range installedAgents {
		skillsDir, ok := skillsDirs[agentID]
		if !ok {
			result.SkippedAgents = append(result.SkippedAgents, agentID)
			continue
		}

		if _, err := uninstallSkillDir(skillsDir, targetName); err != nil {
			rollbackStaged(staged)
			return result, fmt.Errorf("uninstall: invalid registry entry name %q for agent %s: %w", targetName, agentID, err)
		}

		item, err := stageSkillRemoval(skillsDir, targetName)
		if err != nil {
			rollbackStaged(staged)
			return result, fmt.Errorf("uninstall: %w", err)
		}
		if item != nil {
			item.agentID = agentID
			staged = append(staged, *item)
		}
	}

	if !registry.RemoveByName(agentName) {
		rollbackStaged(staged)
		return result, fmt.Errorf("uninstall: agent %q disappeared from registry", agentName)
	}
	if err := saveRegistry(registryPath, registry); err != nil {
		rollbackStaged(staged)
		return result, fmt.Errorf("uninstall: save registry: %w", err)
	}

	removed, committed, commitErr := commitStaged(staged)
	result.RemovedPaths = removed
	if commitErr != nil {
		rollbackStaged(staged[committed:])
		var remaining []model.AgentID
		for _, it := range staged[committed:] {
			remaining = append(remaining, it.agentID)
		}
		remaining = append(remaining, result.SkippedAgents...)
		entry.InstalledAgents = remaining
		registry.Add(*entry)
		if saveErr := saveRegistry(registryPath, registry); saveErr != nil {
			return result, fmt.Errorf("uninstall: %w (restore registry: %v)", commitErr, saveErr)
		}
		return result, fmt.Errorf("uninstall: %w", commitErr)
	}
	return result, nil
}

func validateTargetName(entryName string) error {
	if entryName == "" {
		return fmt.Errorf("name must not be empty")
	}
	if filepath.IsAbs(entryName) {
		return fmt.Errorf("absolute paths are not allowed")
	}
	if entryName != filepath.Base(entryName) || entryName == "." || entryName == ".." || strings.ContainsAny(entryName, `/\\`) {
		return fmt.Errorf("path separators are not allowed")
	}
	return nil
}

func uninstallSkillDir(skillsDir, entryName string) (string, error) {
	if err := validateTargetName(entryName); err != nil {
		return "", err
	}

	skillDir := filepath.Join(skillsDir, entryName)
	rel, err := filepath.Rel(skillsDir, skillDir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.Dir(rel) != "." {
		return "", fmt.Errorf("resolved path escapes skills directory")
	}

	return skillDir, nil
}

func supportedSkillsDirs(homeDir string) map[model.AgentID]string {
	return map[model.AgentID]string{
		model.AgentClaudeCode: claude.NewAdapter().SkillsDir(homeDir),
		model.AgentOpenCode:   opencode.NewAdapter().SkillsDir(homeDir),
		model.AgentGeminiCLI:  gemini.NewAdapter().SkillsDir(homeDir),
		model.AgentCodex:      codex.NewAdapter().SkillsDir(homeDir),
	}
}

func removeIfEmpty(path string) {
	if entries, err := os.ReadDir(path); err == nil && len(entries) == 0 {
		_ = os.Remove(path)
	}
}
