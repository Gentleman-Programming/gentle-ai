package agentbuilder

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

var (
	installerReadFile   = os.ReadFile
	installerStat       = os.Stat
	installerWriteFile  = os.WriteFile
	installerRemoveFile = os.Remove
	installerChmod      = os.Chmod
)

// AdapterInfo pairs an AgentID with the path to its skills directory.
type AdapterInfo struct {
	AgentID   model.AgentID
	SkillsDir string
}

// Install writes the SKILL.md for agent into each adapter's skills directory.
// finalize runs after the files are written and before the installation commits.
// On any failure every destination is restored to its state before install.
// Returns one InstallResult per adapter.
func Install(agent *GeneratedAgent, adapters []AdapterInfo, finalize func([]InstallResult) error) ([]InstallResult, error) {
	if agent == nil {
		return nil, fmt.Errorf("install: agent must not be nil")
	}

	results := make([]InstallResult, 0, len(adapters))
	before := make([]beforeImage, 0, len(adapters))

	for _, adapter := range adapters {
		skillDir := filepath.Join(adapter.SkillsDir, agent.Name)
		skillFile := filepath.Join(skillDir, "SKILL.md")

		if err := os.MkdirAll(skillDir, 0755); err != nil {
			return installFailure(results, before, adapter, skillFile, "create directory", err)
		}

		snapshot, err := captureBeforeImage(skillFile)
		if err != nil {
			return installFailure(results, before, adapter, skillFile, "snapshot", err)
		}
		before = append(before, snapshot)

		if err := installerWriteFile(skillFile, []byte(agent.Content), 0644); err != nil {
			return installFailure(results, before, adapter, skillFile, "write", err)
		}

		results = append(results, InstallResult{
			AgentID: adapter.AgentID,
			Path:    skillFile,
			Success: true,
		})
	}

	if finalize != nil {
		if err := finalize(results); err != nil {
			return finalizeFailure(results, before, err)
		}
	}

	return results, nil
}

type beforeImage struct {
	path    string
	content []byte
	mode    os.FileMode
	exists  bool
}

func captureBeforeImage(path string) (beforeImage, error) {
	info, err := installerStat(path)
	if errors.Is(err, os.ErrNotExist) {
		return beforeImage{path: path}, nil
	}
	if err != nil {
		return beforeImage{}, fmt.Errorf("stat %s: %w", path, err)
	}
	content, err := installerReadFile(path)
	if err != nil {
		return beforeImage{}, fmt.Errorf("read %s: %w", path, err)
	}
	return beforeImage{path: path, content: content, mode: info.Mode(), exists: true}, nil
}

func installFailure(results []InstallResult, before []beforeImage, adapter AdapterInfo, skillFile, operation string, cause error) ([]InstallResult, error) {
	rollbackErr := rollback(before)
	markAllFailed(results)
	results = append(results, InstallResult{
		AgentID: adapter.AgentID,
		Path:    skillFile,
		Success: false,
		Err:     fmt.Errorf("%s %s: %w", operation, skillFile, cause),
	})
	installErr := fmt.Errorf("install failed for %s: %w", adapter.AgentID, cause)
	if rollbackErr != nil {
		return results, errors.Join(installErr, fmt.Errorf("rollback: %w", rollbackErr))
	}
	return results, installErr
}

func finalizeFailure(results []InstallResult, before []beforeImage, cause error) ([]InstallResult, error) {
	rollbackErr := rollback(before)
	markAllFailed(results)
	installErr := fmt.Errorf("complete installation: %w", cause)
	if rollbackErr != nil {
		return results, errors.Join(installErr, fmt.Errorf("rollback: %w", rollbackErr))
	}
	return results, installErr
}

// rollback restores overwritten files and removes files created by this install.
func rollback(before []beforeImage) error {
	var rollbackErrors []error
	for i := len(before) - 1; i >= 0; i-- {
		snapshot := before[i]
		if !snapshot.exists {
			if err := installerRemoveFile(snapshot.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("remove %s: %w", snapshot.path, err))
			}
			continue
		}
		if err := installerWriteFile(snapshot.path, snapshot.content, snapshot.mode); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore %s: %w", snapshot.path, err))
			continue
		}
		if err := installerChmod(snapshot.path, snapshot.mode); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore mode %s: %w", snapshot.path, err))
		}
	}
	return errors.Join(rollbackErrors...)
}

// markAllFailed sets Success=false on every result in the slice.
// Called after a rollback so previously-succeeded results reflect the true outcome.
func markAllFailed(results []InstallResult) {
	for i := range results {
		results[i].Success = false
	}
}
