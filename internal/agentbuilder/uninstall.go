package agentbuilder

import (
	"errors"
	"fmt"
	"io/fs"
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

// Uninstall removes the specified custom agents from the registry and all
// configured adapter skills directories. It validates agent names, removes the
// agent's entire payload directory safely without escaping adapter
// directories, updates the registry, and restores removed content if registry
// persistence fails. Restore failures are reported, never swallowed.
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

	var restorers []func() error
	defer func() {
		err = runRestorers(err, restorers)
	}()

	// Remove the payload directory from all adapters for each requested agent.
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

// payloadSnapshot captures every file and symlink under an agent directory so
// a failed uninstall can restore the exact payload.
type payloadSnapshot struct {
	files    map[string]payloadFile // paths relative to the skills root
	symlinks map[string]string      // paths relative to the skills root -> targets
}

type payloadFile struct {
	data []byte
	perm os.FileMode
}

func snapshotPayload(root *os.Root, name string) (*payloadSnapshot, error) {
	snap := &payloadSnapshot{files: map[string]payloadFile{}, symlinks: map[string]string{}}
	walkErr := fs.WalkDir(root.FS(), name, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == name {
			return nil
		}
		fi, err := root.Lstat(p)
		if err != nil {
			return err
		}
		switch {
		case fi.Mode()&os.ModeSymlink != 0:
			target, err := root.Readlink(p)
			if err != nil {
				return err
			}
			snap.symlinks[p] = target
		case fi.Mode().IsRegular():
			data, err := root.ReadFile(p)
			if err != nil {
				return err
			}
			snap.files[p] = payloadFile{data: data, perm: fi.Mode().Perm()}
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return snap, nil
}

func (s *payloadSnapshot) restore(root *os.Root) error {
	var errs []error
	for p, target := range s.symlinks {
		if err := root.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := root.Symlink(target, p); err != nil {
			errs = append(errs, err)
		}
	}
	for p, f := range s.files {
		if err := root.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := root.WriteFile(p, f.data, f.perm); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// runRestorers executes restorers in reverse order and joins every failure
// onto err, so a partially restored payload is never reported as a clean
// rollback. A nil err is returned untouched: restorers only run on failure.
func runRestorers(err error, restorers []func() error) error {
	if err == nil {
		return nil
	}
	restoreErrs := make([]error, 0, len(restorers))
	for i := len(restorers) - 1; i >= 0; i-- {
		if rErr := restorers[i](); rErr != nil {
			restoreErrs = append(restoreErrs, rErr)
		}
	}
	if len(restoreErrs) > 0 {
		return errors.Join(append([]error{err}, restoreErrs...)...)
	}
	return err
}

func uninstallFromAdapter(skillsDir, name string) (func() error, error) {
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
		target, err := root.Readlink(name)
		if err != nil {
			return nil, err
		}
		if err := root.Remove(name); err != nil {
			return nil, err
		}
		return func() error {
			r, err := os.OpenRoot(skillsDir)
			if err != nil {
				return err
			}
			defer r.Close()
			return r.Symlink(target, name)
		}, nil
	}

	if !fi.IsDir() {
		return nil, fmt.Errorf("expected directory or symlink for agent %s in %s", name, skillsDir)
	}

	// Snapshot the entire payload so a failed uninstall can restore it.
	snap, err := snapshotPayload(root, name)
	if err != nil {
		return nil, err
	}

	// Remove the whole agent directory: partial removals would leave
	// untracked leftovers with no registry entry to retry from.
	if err := root.RemoveAll(name); err != nil {
		return nil, err
	}

	return func() error {
		r, err := os.OpenRoot(skillsDir)
		if err != nil {
			return err
		}
		defer r.Close()
		return snap.restore(r)
	}, nil
}
