package sdd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
)

const hiddenClaudeInternalAgentFrontmatter = "user-invocable: false"

func hideManagedClaudeInternalAgentsForVSCode(homeDir string) (InjectionResult, error) {
	agentsDir := filepath.Join(homeDir, ".claude", "agents")
	if info, err := os.Stat(agentsDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return InjectionResult{}, nil
		}
		return InjectionResult{}, fmt.Errorf("inspect Claude agents dir: %w", err)
	} else if !info.IsDir() {
		return InjectionResult{}, nil
	}

	result := InjectionResult{}
	for _, fileName := range managedClaudeInternalAgentFiles() {
		path := filepath.Join(agentsDir, fileName)
		content, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return InjectionResult{}, fmt.Errorf("read managed Claude agent %s: %w", fileName, err)
		}

		text := string(content)
		updated := hideClaudeAgentFromVSCodePicker(text)
		if updated == text {
			continue
		}

		// Back up original content before write
		backupPath := path + ".backup"
		if err := os.WriteFile(backupPath, content, 0o644); err != nil {
			return InjectionResult{}, fmt.Errorf("backup managed Claude agent %s: %w", fileName, err)
		}

		writeResult, err := filemerge.WriteFileAtomic(path, []byte(updated), 0o644)
		if err != nil {
			return InjectionResult{}, fmt.Errorf("write managed Claude agent %s: %w", fileName, err)
		}
		if writeResult.Changed {
			result.Changed = true
			result.Files = append(result.Files, path)
		}
	}
	return result, nil
}

// RestoreManagedClaudeInternalAgentsForVSCode restores the backed-up Claude internal agent files
// from their .backup copies and removes the backup files.
func RestoreManagedClaudeInternalAgentsForVSCode(homeDir string) error {
	agentsDir := filepath.Join(homeDir, ".claude", "agents")
	for _, fileName := range managedClaudeInternalAgentFiles() {
		path := filepath.Join(agentsDir, fileName)
		backupPath := path + ".backup"
		if _, err := os.Stat(backupPath); err == nil {
			content, err := os.ReadFile(backupPath)
			if err != nil {
				return fmt.Errorf("read backup %s: %w", backupPath, err)
			}
			if _, err := filemerge.WriteFileAtomic(path, content, 0o644); err != nil {
				return fmt.Errorf("restore agent %s from backup: %w", path, err)
			}
			_ = os.Remove(backupPath)
		}
	}
	return nil
}

func managedClaudeInternalAgentFiles() []string {
	return []string{
		"sdd-init.md",
		"sdd-explore.md",
		"sdd-propose.md",
		"sdd-spec.md",
		"sdd-design.md",
		"sdd-tasks.md",
		"sdd-apply.md",
		"sdd-verify.md",
		"sdd-archive.md",
		"sdd-onboard.md",
		"jd-judge-a.md",
		"jd-judge-b.md",
		"jd-fix-agent.md",
	}
}

func hideClaudeAgentFromVSCodePicker(content string) string {
	lineBreak := "\n"
	if strings.Contains(content, "\r\n") {
		lineBreak = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	closing := frontmatterClosingLine(lines)
	if closing == -1 {
		return content
	}

	updated := make([]string, 0, len(lines)+1)
	updated = append(updated, lines[0])
	hidden := false
	for i := 1; i < closing; i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "user-invocable:") {
			if !hidden {
				updated = append(updated, hiddenClaudeInternalAgentFrontmatter)
				hidden = true
			}
			continue
		}
		updated = append(updated, lines[i])
	}
	if !hidden {
		updated = append(updated, hiddenClaudeInternalAgentFrontmatter)
	}
	updated = append(updated, lines[closing:]...)
	return strings.Join(updated, lineBreak)
}
