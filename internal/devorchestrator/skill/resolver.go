package skill

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	// ErrSkillNotFound indicates a requested skill has no matching SKILL.md
	// anywhere under the workspace skills directory. This is an expected,
	// clean not-found result, never treated as an unexpected error.
	ErrSkillNotFound = errors.New("skill not found")

	// ErrSkillLookup indicates an unexpected filesystem error occurred while
	// looking up a skill (anything other than "path does not exist").
	ErrSkillLookup = errors.New("skill lookup failed")
)

// Resolver identifies which standard agent skills map to a given set of requested skill names.
type Resolver struct {
	WorkspaceRoot string

	// readDir and stat are seams over the filesystem so tests can inject
	// deterministic, OS-independent lookup errors without relying on
	// platform-specific permission behavior.
	readDir func(name string) ([]os.DirEntry, error)
	stat    func(name string) (os.FileInfo, error)
}

// New creates a new Skill Resolver.
func New(workspaceRoot string) *Resolver {
	return &Resolver{
		WorkspaceRoot: workspaceRoot,
		readDir:       os.ReadDir,
		stat:          os.Stat,
	}
}

// Resolve looks up the requested skill names against the local workspace "skills" directory.
// In a full implementation, this could also check ~/.gemini/config/skills.
// It returns a list of paths to the resolved SKILL.md files.
func (r *Resolver) Resolve(requestedSkills []string) ([]string, error) {
	var resolvedPaths []string
	var missingSkills []string

	workspaceSkillsDir := filepath.Join(r.WorkspaceRoot, "skills")

	for _, skillName := range requestedSkills {
		// Clean the skill name
		cleanName := strings.TrimSpace(skillName)
		if cleanName == "" {
			continue
		}

		foundPath, err := r.findSkillInDir(workspaceSkillsDir, cleanName)
		if err != nil {
			return resolvedPaths, err
		}
		if foundPath != "" {
			resolvedPaths = append(resolvedPaths, foundPath)
		} else {
			missingSkills = append(missingSkills, cleanName)
		}
	}

	if len(missingSkills) > 0 {
		return resolvedPaths, fmt.Errorf("%w: could not resolve the following skills: %v", ErrSkillNotFound, missingSkills)
	}

	return resolvedPaths, nil
}

// findSkillInDir resolves a skill by direct path join and stat instead of
// recursively walking the entire skills tree. It probes the bare
// "skills/<name>/SKILL.md" layout first, then each top-level category
// directory ("skills/<category>/<name>/SKILL.md"). A genuinely missing
// skill returns ("", nil). Any filesystem error other than "path does not
// exist" is wrapped in ErrSkillLookup and returned immediately, instead of
// being silently discarded.
func (r *Resolver) findSkillInDir(baseDir string, skillName string) (string, error) {
	bareSkillMd := filepath.Join(baseDir, skillName, "SKILL.md")
	found, err := r.statSkillMd(bareSkillMd)
	if err != nil {
		return "", err
	}
	if found {
		return bareSkillMd, nil
	}

	entries, err := r.readDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("%w: %w", ErrSkillLookup, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillMd := filepath.Join(baseDir, entry.Name(), skillName, "SKILL.md")
		found, err := r.statSkillMd(skillMd)
		if err != nil {
			return "", err
		}
		if found {
			return skillMd, nil
		}
	}

	return "", nil
}

// statSkillMd stats a candidate SKILL.md path. It returns (true, nil) when
// found, (false, nil) when the path cleanly does not exist, and (false, err)
// with err wrapped in ErrSkillLookup for any other filesystem failure.
func (r *Resolver) statSkillMd(path string) (bool, error) {
	if _, err := r.stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("%w: %w", ErrSkillLookup, err)
	}
	return true, nil
}
