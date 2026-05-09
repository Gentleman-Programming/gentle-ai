package skills

import (
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// IsSDDSkill reports whether a skill ID belongs to the SDD orchestrator suite.
// SDD skills are installed by the SDD component; the skills component skips
// them to prevent duplicate writes when both components are selected.
func IsSDDSkill(id model.SkillID) bool {
	return strings.HasPrefix(string(id), "sdd-")
}

type InjectionResult struct {
	Changed bool
	Files   []string
	Skipped []model.SkillID
}

// Inject writes the embedded SKILL.md files for each requested skill
// to the correct directory for the given agent adapter.
//
// The skills directory is determined by adapter.SkillsDir(), removing
// the need for any agent-specific switch statements.
//
// SDD skills (those whose IDs begin with "sdd-") are intentionally skipped
// here because the SDD component installs them as part of its own injection.
// This prevents a write conflict when both components are selected together.
//
// Individual skill failures (e.g., missing embedded asset) are logged
// and skipped rather than aborting the entire operation.
func Inject(homeDir string, adapter agents.Adapter, skillIDs []model.SkillID) (InjectionResult, error) {
	if !adapter.SupportsSkills() {
		return InjectionResult{Skipped: skillIDs}, nil
	}

	skillDir := adapter.SkillsDir(homeDir)
	if skillDir == "" {
		return InjectionResult{Skipped: skillIDs}, nil
	}

	paths := make([]string, 0, len(skillIDs))
	skipped := make([]model.SkillID, 0)
	changed := false

	for _, id := range skillIDs {
		// SDD skills are written by the SDD component — skip to avoid conflicts.
		if IsSDDSkill(id) {
			continue
		}

		embedDir := "skills/" + string(id)
		entries, readErr := fs.ReadDir(assets.FS, embedDir)
		if readErr != nil {
			log.Printf("skills: skipping %q — embedded asset not found: %v", id, readErr)
			skipped = append(skipped, id)
			continue
		}
		if len(entries) == 0 {
			return InjectionResult{}, fmt.Errorf("skill %q: embedded directory exists but is empty — build may be corrupt", id)
		}

		destDir := filepath.Join(skillDir, string(id))
		walkErr := fs.WalkDir(assets.FS, embedDir, func(assetPath string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}

			content, readErr := assets.Read(assetPath)
			if readErr != nil {
				return fmt.Errorf("read %q: %w", assetPath, readErr)
			}
			if len(content) == 0 {
				return fmt.Errorf("embedded asset %q is empty", assetPath)
			}

			relPath, relErr := filepath.Rel(filepath.FromSlash(embedDir), filepath.FromSlash(assetPath))
			if relErr != nil {
				return fmt.Errorf("resolve relative path for %q: %w", assetPath, relErr)
			}
			path := filepath.Join(destDir, relPath)
			writeResult, writeErr := filemerge.WriteFileAtomic(path, []byte(content), 0o644)
			if writeErr != nil {
				return fmt.Errorf("write %q: %w", path, writeErr)
			}

			changed = changed || writeResult.Changed
			paths = append(paths, path)
			return nil
		})
		if walkErr != nil {
			return InjectionResult{}, fmt.Errorf("skill %q: copy embedded directory: %w", id, walkErr)
		}
	}

	return InjectionResult{Changed: changed, Files: paths, Skipped: skipped}, nil
}

// SkillPathForAgent returns the filesystem path where a skill file would be written.
func SkillPathForAgent(homeDir string, adapter agents.Adapter, id model.SkillID) string {
	skillDir := adapter.SkillsDir(homeDir)
	if skillDir == "" {
		return ""
	}
	return filepath.Join(skillDir, string(id), "SKILL.md")
}
