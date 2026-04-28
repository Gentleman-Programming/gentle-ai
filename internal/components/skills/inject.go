package skills

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/undo"
)

// isSDDSkill reports whether a skill ID belongs to the SDD orchestrator suite.
// SDD skills are installed by the SDD component; the skills component skips
// them to prevent duplicate writes when both components are selected.
func isSDDSkill(id model.SkillID) bool {
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
	return InjectWithRecorder(homeDir, adapter, skillIDs, nil)
}

// InjectWithRecorder is identical to Inject but accepts an optional undo
// Recorder. When rec is non-nil, each file write is recorded so the step
// can be rolled back granularly without restoring the entire backup snapshot.
func InjectWithRecorder(homeDir string, adapter agents.Adapter, skillIDs []model.SkillID, rec *undo.Recorder) (InjectionResult, error) {
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
		if isSDDSkill(id) {
			continue
		}

		assetPath := "skills/" + string(id) + "/SKILL.md"
		content, readErr := assets.Read(assetPath)
		if readErr != nil {
			log.Printf("skills: skipping %q — embedded asset not found: %v", id, readErr)
			skipped = append(skipped, id)
			continue
		}
		if len(content) == 0 {
			return InjectionResult{}, fmt.Errorf("skill %q: embedded asset exists but is empty — build may be corrupt", id)
		}

		path := filepath.Join(skillDir, string(id), "SKILL.md")
		writeResult, writeErr := filemerge.WriteFileAtomicWithRecorder(path, []byte(content), 0o644, rec)
		if writeErr != nil {
			return InjectionResult{}, fmt.Errorf("skill %q: write failed: %w", id, writeErr)
		}

		changed = changed || writeResult.Changed
		paths = append(paths, path)
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
