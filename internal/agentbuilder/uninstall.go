package agentbuilder

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateAgentName ensures an agent name is non-empty, contains no path separators
// or directory traversal sequences, and is safe for filesystem operations.
func ValidateAgentName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("agent name cannot be empty")
	}
	if filepath.Clean(name) != name || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid agent name: %q", name)
	}
	return nil
}

// Uninstall removes the specified custom agents from the registry and all configured adapter skills directories.
// It validates agent names, removes agent skill files safely without escaping adapter directories,
// updates the registry, and restores removed files if registry persistence fails.
func Uninstall(registryPath string, agentNames []string, adapters []AdapterInfo) (err error) {
	if len(agentNames) == 0 {
		return nil
	}
	for _, name := range agentNames {
		if err := ValidateAgentName(name); err != nil {
			return err
		}
	}

	reg, err := LoadRegistry(registryPath)
	if err != nil {
		return fmt.Errorf("load registry: %w", err)
	}

	var restorers []func()
	defer func() {
		if err != nil {
			for i := len(restorers) - 1; i >= 0; i-- {
				restorers[i]()
			}
		}
	}()

	// Remove files from all adapters for each requested agent.
	for _, name := range agentNames {
		for _, adapter := range adapters {
			rFn, removeErr := uninstallFromAdapter(adapter.SkillsDir, name)
			if removeErr != nil {
				err = removeErr
				return err
			}
			if rFn != nil {
				restorers = append(restorers, rFn)
			}
		}
	}

	// Remove agent entries from registry.
	for _, name := range agentNames {
		reg.RemoveByName(name)
	}

	if saveErr := SaveRegistry(registryPath, reg); saveErr != nil {
		err = fmt.Errorf("save registry: %w", saveErr)
		return err
	}

	return nil
}

func uninstallFromAdapter(skillsDir, name string) (func(), error) {
	if skillsDir == "" {
		return nil, nil
	}
	root, err := os.OpenRoot(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer root.Close()

	fi, err := root.Lstat(name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// If the entry itself is a symlink, remove it directly (never follow outside).
	if fi.Mode()&os.ModeSymlink != 0 {
		target, _ := root.Readlink(name)
		if err := root.Remove(name); err != nil {
			return nil, err
		}
		return func() {
			r, err := os.OpenRoot(skillsDir)
			if err == nil {
				defer r.Close()
				_ = r.Symlink(target, name)
			}
		}, nil
	}

	if !fi.IsDir() {
		return nil, fmt.Errorf("expected directory or symlink for agent %s in %s", name, skillsDir)
	}

	skillPath := filepath.Join(name, "SKILL.md")
	sfi, err := root.Lstat(skillPath)
	if err != nil {
		if os.IsNotExist(err) {
			_ = root.Remove(name)
			return nil, nil
		}
		return nil, err
	}

	// Reject directory payload (e.g. SKILL.md is a directory).
	if sfi.IsDir() {
		return nil, fmt.Errorf("payload %s is a directory, expected file", filepath.Join(skillsDir, skillPath))
	}

	content, err := root.ReadFile(skillPath)
	if err != nil {
		return nil, err
	}

	if err := root.Remove(skillPath); err != nil {
		return nil, err
	}
	_ = root.Remove(name) // Best-effort cleanup of empty directory.

	return func() {
		r, err := os.OpenRoot(skillsDir)
		if err == nil {
			defer r.Close()
			_ = r.MkdirAll(name, 0755)
			_ = r.WriteFile(skillPath, content, 0644)
		}
	}, nil
}
