package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolver identifies which standard agent skills map to a given set of requested skill names.
type Resolver struct {
	WorkspaceRoot string
}

// New creates a new Skill Resolver.
func New(workspaceRoot string) *Resolver {
	return &Resolver{
		WorkspaceRoot: workspaceRoot,
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

		foundPath := r.findSkillInDir(workspaceSkillsDir, cleanName)
		if foundPath != "" {
			resolvedPaths = append(resolvedPaths, foundPath)
		} else {
			missingSkills = append(missingSkills, cleanName)
		}
	}

	if len(missingSkills) > 0 {
		return resolvedPaths, fmt.Errorf("could not resolve the following skills: %v", missingSkills)
	}

	return resolvedPaths, nil
}

func (r *Resolver) findSkillInDir(baseDir string, skillName string) string {
	// A basic walk to find a directory exactly matching the skill name that contains SKILL.md
	var foundPath string
	_ = filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // ignore errors
		}
		if info.IsDir() && info.Name() == skillName {
			skillMd := filepath.Join(path, "SKILL.md")
			if _, statErr := os.Stat(skillMd); statErr == nil {
				foundPath = skillMd
				return filepath.SkipDir // found it, stop walking deeper here
			}
		}
		return nil
	})
	return foundPath
}
