package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/sdd"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/skills"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// compatibilitySkillsRefreshStep refreshes the registry-scanned shared skills
// path once after adapter-specific component injection has completed. The path
// remains opt-in: installs and syncs never create it when it is absent.
type compatibilitySkillsRefreshStep struct {
	id           string
	homeDir      string
	components   []model.ComponentID
	selection    model.Selection
	changedFiles *[]string
}

var lstatCompatibilityDestination = os.Lstat

type compatibilityDirectoryWriter interface {
	Write(string, []byte, fs.FileMode) (filemerge.WriteResult, error)
	Close() error
}

func (s compatibilitySkillsRefreshStep) ID() string {
	if s.id != "" {
		return s.id
	}
	return "compatibility-skills-refresh"
}

func needsCompatibilitySkillsRefresh(components []model.ComponentID) bool {
	return slices.Contains(components, model.ComponentSkills) || slices.Contains(components, model.ComponentSDD)
}

func compatibilitySkillsDir(homeDir string) (string, bool, error) {
	agentsDir := filepath.Join(homeDir, ".agents")
	skillDir := filepath.Join(agentsDir, "skills")
	parent, err := lstatCompatibilityDestination(agentsDir)
	if os.IsNotExist(err) || err == nil && !parent.IsDir() {
		return skillDir, false, nil
	}
	if err != nil {
		return skillDir, false, fmt.Errorf("stat compatibility skills parent directory %q: %w", agentsDir, err)
	}
	info, err := lstatCompatibilityDestination(skillDir)
	if os.IsNotExist(err) || err == nil && !info.IsDir() {
		return skillDir, false, nil
	}
	if err != nil {
		return skillDir, false, fmt.Errorf("stat compatibility skills directory %q: %w", skillDir, err)
	}
	return skillDir, true, nil
}

func compatibilitySkillsRefreshable(homeDir string, selection model.Selection) (bool, error) {
	_, ok, err := compatibilitySkillsDir(homeDir)
	if err != nil || !ok {
		return false, err
	}
	return slices.Contains(selection.Components, model.ComponentSDD) ||
		slices.Contains(selection.Components, model.ComponentSkills) && len(selectedSkillIDs(selection)) > 0, nil
}

func compatibilitySkillFiles(skillDir string, components []model.ComponentID, selection model.Selection) ([]string, error) {
	paths := map[string]struct{}{}
	if slices.Contains(components, model.ComponentSkills) {
		prospective, err := skills.DirectoryPaths(skillDir, selectedSkillIDs(selection), "")
		if err != nil {
			return nil, fmt.Errorf("enumerate compatibility skills: %w", err)
		}
		for _, path := range prospective {
			paths[path] = struct{}{}
		}
	}
	if slices.Contains(components, model.ComponentSDD) {
		prospective, err := sdd.SkillDirectoryPaths(skillDir, "")
		if err != nil {
			return nil, fmt.Errorf("enumerate compatibility SDD skills: %w", err)
		}
		for _, path := range prospective {
			paths[path] = struct{}{}
		}
	}
	files := make([]string, 0, len(paths))
	for path := range paths {
		files = append(files, path)
	}
	sort.Strings(files)
	if err := validateCompatibilityDestinations(skillDir, files); err != nil {
		return nil, err
	}
	return files, nil
}

func validateCompatibilityDestinations(root string, destinations []string) error {
	for _, destination := range destinations {
		relative, err := filepath.Rel(root, destination)
		if err != nil || relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("compatibility destination %q escapes %q; remove the escaping destination, then rerun gentle-ai install or gentle-ai sync", destination, root)
		}
		current := root
		for _, part := range strings.Split(filepath.Dir(relative), string(filepath.Separator)) {
			if part == "." || part == "" {
				continue
			}
			current = filepath.Join(current, part)
			info, err := lstatCompatibilityDestination(current)
			if os.IsNotExist(err) {
				break
			}
			if err != nil {
				return fmt.Errorf("stat compatibility destination ancestor %q: %w", current, err)
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("compatibility destination ancestor %q must be a physical directory; replace it with a physical directory, then rerun gentle-ai install or gentle-ai sync", current)
			}
		}
		info, err := lstatCompatibilityDestination(destination)
		if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
			return fmt.Errorf("compatibility destination %q must be a regular file; replace it with a regular file or remove it, then rerun gentle-ai install or gentle-ai sync", destination)
		}
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("stat compatibility destination %q: %w", destination, err)
		}
	}
	return nil
}

func (s compatibilitySkillsRefreshStep) Run() error {
	skillDir, ok, err := compatibilitySkillsDir(s.homeDir)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	writer, err := newCompatibilityDirectoryWriter(s.homeDir, skillDir)
	if err != nil {
		return err
	}
	defer writer.Close()

	_, err = compatibilitySkillFiles(skillDir, s.components, s.selection)
	if err != nil {
		return err
	}

	var changed []string
	if slices.Contains(s.components, model.ComponentSkills) {
		skillIDs := selectedSkillIDs(s.selection)
		if len(skillIDs) > 0 {
			result, injectErr := skills.InjectDirectoryWithWriter(skillDir, skillIDs, writer.Write)
			if injectErr != nil {
				return fmt.Errorf("refresh compatibility skills: %w", injectErr)
			}
			if result.Changed {
				changed = append(changed, result.Files...)
			}
		}
	}

	if slices.Contains(s.components, model.ComponentSDD) {
		result, injectErr := sdd.InjectSkillDirectoryWithWriter(skillDir, "", writer.Write)
		if injectErr != nil {
			return fmt.Errorf("refresh compatibility SDD skills: %w", injectErr)
		}
		if result.Changed {
			changed = append(changed, result.Files...)
		}
	}

	if s.changedFiles != nil {
		*s.changedFiles = append(*s.changedFiles, changed...)
	}
	return nil
}
