package agentbuilder

import (
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

// UninstallResult captures the backend uninstall outcome for one custom agent.
type UninstallResult struct {
	RemovedPaths  []string
	SkippedAgents []model.AgentID
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
		return UninstallResult{}, fmt.Errorf("uninstall: agent %q not found in registry", agentName)
	}

	targetName := entry.Name
	if err := validateTargetName(targetName); err != nil {
		return UninstallResult{}, fmt.Errorf("uninstall: invalid registry entry name %q: %w", targetName, err)
	}
	installedAgents := append([]model.AgentID(nil), entry.InstalledAgents...)

	result := UninstallResult{}
	skillsDirs := supportedSkillsDirs(homeDir)

	for _, agentID := range installedAgents {
		skillsDir, ok := skillsDirs[agentID]
		if !ok {
			result.SkippedAgents = append(result.SkippedAgents, agentID)
			continue
		}

		skillDir, err := uninstallSkillDir(skillsDir, targetName)
		if err != nil {
			return result, fmt.Errorf("uninstall: invalid registry entry name %q for agent %s: %w", targetName, agentID, err)
		}

		info, err := os.Lstat(skillDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return result, fmt.Errorf("uninstall: stat %s: %w", skillDir, err)
		}

		toRemove := filepath.Join(skillDir, "SKILL.md")
		if info.Mode()&os.ModeSymlink != 0 {
			toRemove = skillDir
		}

		if err := os.Remove(toRemove); err != nil {
			if !os.IsNotExist(err) {
				return result, fmt.Errorf("uninstall: remove %s: %w", toRemove, err)
			}
		} else {
			result.RemovedPaths = append(result.RemovedPaths, toRemove)
		}

		if toRemove != skillDir {
			removeIfEmpty(skillDir)
		}
	}

	if !registry.RemoveByName(agentName) {
		return result, fmt.Errorf("uninstall: agent %q disappeared from registry", agentName)
	}
	if err := saveRegistry(registryPath, registry); err != nil {
		return result, fmt.Errorf("uninstall: save registry: %w", err)
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
