package agentbuilder

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/mutationjournal"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// AdapterInfo pairs an AgentID with the path to its skills directory.
type AdapterInfo struct {
	AgentID   model.AgentID
	SkillsDir string
}

type installJournal interface {
	Capture(string) error
	WriteWithMode(string, []byte, os.FileMode) (mutationjournal.OwnedFile, error)
	Restore() error
}

type installOperations struct {
	mkdirAll   func(string, os.FileMode) error
	newJournal func(...string) installJournal
}

// Install writes the SKILL.md for agent into each adapter's skills directory.
// On failure, every changed SKILL.md is restored to its before-image.
func Install(agent *GeneratedAgent, adapters []AdapterInfo, _ string) ([]InstallResult, error) {
	return install(agent, adapters, installOperations{
		mkdirAll:   os.MkdirAll,
		newJournal: newInstallJournal,
	})
}

func install(agent *GeneratedAgent, adapters []AdapterInfo, ops installOperations) ([]InstallResult, error) {
	if agent == nil {
		return nil, fmt.Errorf("install: agent must not be nil")
	}

	results := make([]InstallResult, 0, len(adapters))
	roots := make([]string, 0, len(adapters))
	for _, adapter := range adapters {
		roots = append(roots, adapter.SkillsDir)
	}
	journal := ops.newJournal(roots...)

	for _, adapter := range adapters {
		skillDir := filepath.Join(adapter.SkillsDir, agent.Name)
		skillFile := filepath.Join(skillDir, "SKILL.md")

		if err := journal.Capture(skillFile); err != nil {
			return rollback(journal, results, adapter, skillFile, fmt.Errorf("capture before-image %s: %w", skillFile, err))
		}

		if err := ops.mkdirAll(skillDir, 0755); err != nil {
			return rollback(journal, results, adapter, skillFile, fmt.Errorf("create directory %s: %w", skillDir, err))
		}

		if _, err := journal.WriteWithMode(skillFile, []byte(agent.Content), 0644); err != nil {
			return rollback(journal, results, adapter, skillFile, fmt.Errorf("write %s: %w", skillFile, err))
		}

		results = append(results, InstallResult{
			AgentID: adapter.AgentID,
			Path:    skillFile,
			Success: true,
		})
	}

	return results, nil
}

func newInstallJournal(roots ...string) installJournal {
	return mutationjournal.New(roots...)
}

func rollback(journal installJournal, results []InstallResult, adapter AdapterInfo, skillFile string, cause error) ([]InstallResult, error) {
	restoreErr := journal.Restore()
	markAllFailed(results)
	results = append(results, InstallResult{
		AgentID: adapter.AgentID,
		Path:    skillFile,
		Success: false,
		Err:     cause,
	})
	return results, errors.Join(cause, restoreErr)
}

// markAllFailed sets Success=false on every result in the slice.
// Called after a rollback so previously-succeeded results reflect the true outcome.
func markAllFailed(results []InstallResult) {
	for i := range results {
		results[i].Success = false
	}
}
