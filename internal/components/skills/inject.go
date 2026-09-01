package skills

import (
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
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

// InjectWithCapability writes skill files like Inject, but for SDD skills
// it extracts only the section matching the given capability.
func InjectWithCapability(homeDir string, adapter agents.Adapter, skillIDs []model.SkillID, capability string) (InjectionResult, error) {
	if !adapter.SupportsSkills() {
		return InjectionResult{Skipped: skillIDs}, nil
	}

	skillDir := adapter.SkillsDir(homeDir)
	if skillDir == "" {
		return InjectionResult{Skipped: skillIDs}, nil
	}
	return InjectDirectoryWithCapability(skillDir, skillIDs, capability)
}

type directoryAsset struct {
	source      string
	destination string
}

// DirectoryPaths returns every file that directory injection may write.
func DirectoryPaths(skillDir string, skillIDs []model.SkillID, capability string) ([]string, error) {
	entries, _, err := directoryAssets(skillDir, skillIDs, capability)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.destination)
	}
	return paths, nil
}

func directoryAssets(skillDir string, skillIDs []model.SkillID, capability string) ([]directoryAsset, []model.SkillID, error) {
	var result []directoryAsset
	var skipped []model.SkillID
	for _, id := range skillIDs {
		if IsSDDSkill(id) && capability == "" {
			continue
		}
		embedDir := "skills/" + string(id)
		entries, err := fs.ReadDir(assets.FS, embedDir)
		if err != nil {
			log.Printf("skills: skipping %q — embedded asset not found: %v", id, err)
			skipped = append(skipped, id)
			continue
		}
		if len(entries) == 0 {
			return nil, nil, fmt.Errorf("skill %q: embedded directory exists but is empty — build may be corrupt", id)
		}
		err = fs.WalkDir(assets.FS, embedDir, func(source string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(filepath.FromSlash(embedDir), filepath.FromSlash(source))
			if err != nil {
				return fmt.Errorf("resolve relative path for %q: %w", source, err)
			}
			result = append(result, directoryAsset{source: source, destination: filepath.Join(skillDir, string(id), relative)})
			return nil
		})
		if err != nil {
			return nil, nil, fmt.Errorf("skill %q: enumerate embedded directory: %w", id, err)
		}
	}
	return result, skipped, nil
}

// InjectDirectoryWithCapability writes skills directly to an already-selected
// skills directory. Operation-level compatibility refreshes use this to avoid
// routing the shared directory through every selected agent adapter.
func InjectDirectoryWithCapability(skillDir string, skillIDs []model.SkillID, capability string) (InjectionResult, error) {
	return InjectDirectoryWithCapabilityWithWriter(skillDir, skillIDs, capability, filemerge.WriteFileAtomic)
}

// InjectDirectoryWithWriter writes ordinary skills with a caller-selected writer.
// Compatibility refreshes use this to keep their physical-directory contract.
func InjectDirectoryWithWriter(skillDir string, skillIDs []model.SkillID, writeFile func(string, []byte, fs.FileMode) (filemerge.WriteResult, error)) (InjectionResult, error) {
	return InjectDirectoryWithCapabilityWithWriter(skillDir, skillIDs, "", writeFile)
}

// InjectDirectoryWithCapabilityWithWriter writes skills with a caller-selected writer.
func InjectDirectoryWithCapabilityWithWriter(skillDir string, skillIDs []model.SkillID, capability string, writeFile func(string, []byte, fs.FileMode) (filemerge.WriteResult, error)) (InjectionResult, error) {
	entries, skipped, err := directoryAssets(skillDir, skillIDs, capability)
	if err != nil {
		return InjectionResult{}, err
	}
	result := InjectionResult{Files: make([]string, 0, len(entries)), Skipped: skipped}
	for _, entry := range entries {
		content, err := assets.Read(entry.source)
		if err != nil {
			return InjectionResult{}, fmt.Errorf("read %q: %w", entry.source, err)
		}
		if len(content) == 0 {
			return InjectionResult{}, fmt.Errorf("embedded asset %q is empty", entry.source)
		}
		if capability != "" {
			content = extractModelSection(content, capability)
		}
		writeResult, err := writeFile(entry.destination, []byte(content), 0o644)
		if err != nil {
			return InjectionResult{}, fmt.Errorf("write %q: %w", entry.destination, err)
		}
		result.Changed = result.Changed || writeResult.Changed
		result.Files = append(result.Files, entry.destination)
	}
	return result, nil
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
	return InjectWithCapability(homeDir, adapter, skillIDs, "")
}

// SkillPathForAgent returns the filesystem path where a skill file would be written.
func SkillPathForAgent(homeDir string, adapter agents.Adapter, id model.SkillID) string {
	skillDir := adapter.SkillsDir(homeDir)
	if skillDir == "" {
		return ""
	}
	return filepath.Join(skillDir, string(id), "SKILL.md")
}

// extractModelSection extracts the section matching the given capability
// ("capable" or "small") from content containing <!-- section:model-capable -->
// and <!-- section:model-small --> markers. If no matching section is found,
// the full content is returned.
func extractModelSection(content, capability string) string {
	return filemerge.ExtractHTMLCommentSection(content, "model-"+capability)
}
