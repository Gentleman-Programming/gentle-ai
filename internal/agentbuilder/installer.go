package agentbuilder

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// AdapterInfo pairs an AgentID with the path to its skills directory.
type AdapterInfo struct {
	AgentID   model.AgentID
	SkillsDir string
}

// Install writes the SKILL.md for agent into each adapter's skills directory.
// On any write failure all previously written files are rolled back (deleted).
// Returns one InstallResult per adapter.
func Install(agent *GeneratedAgent, adapters []AdapterInfo, _ string) ([]InstallResult, error) {
	if agent == nil {
		return nil, fmt.Errorf("install: agent must not be nil")
	}

	results := make([]InstallResult, 0, len(adapters))
	written := make([]string, 0, len(adapters)) // paths written so far, for rollback

	for _, adapter := range adapters {
		skillDir := filepath.Join(adapter.SkillsDir, agent.Name)
		skillFile := filepath.Join(skillDir, "SKILL.md")

		if err := os.MkdirAll(skillDir, 0755); err != nil {
			// Rollback previously written files and mark all results as failed.
			rollback(written)
			markAllFailed(results)
			results = append(results, InstallResult{
				AgentID: adapter.AgentID,
				Path:    skillFile,
				Success: false,
				Err:     fmt.Errorf("create directory %s: %w", skillDir, err),
			})
			return results, fmt.Errorf("install failed for %s: %w", adapter.AgentID, err)
		}

		if err := os.WriteFile(skillFile, []byte(agent.Content), 0644); err != nil {
			// Rollback previously written files and mark all results as failed.
			rollback(written)
			markAllFailed(results)
			results = append(results, InstallResult{
				AgentID: adapter.AgentID,
				Path:    skillFile,
				Success: false,
				Err:     fmt.Errorf("write %s: %w", skillFile, err),
			})
			return results, fmt.Errorf("install failed for %s: %w", adapter.AgentID, err)
		}

		written = append(written, skillFile)
		results = append(results, InstallResult{
			AgentID: adapter.AgentID,
			Path:    skillFile,
			Success: true,
		})
	}

	return results, nil
}

// rollback removes all files in paths, ignoring errors (best-effort cleanup).
func rollback(paths []string) {
	for _, p := range paths {
		_ = os.Remove(p)
	}
}

// markAllFailed sets Success=false on every result in the slice.
// Called after a rollback so previously-succeeded results reflect the true outcome.
func markAllFailed(results []InstallResult) {
	for i := range results {
		results[i].Success = false
	}
}
