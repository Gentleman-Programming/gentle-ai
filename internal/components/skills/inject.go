package skills

import (
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// IsSDDSkill identifies retired SDD skill IDs retained for compatibility.
// They are never installed, regardless of the requested capability.
func IsSDDSkill(id model.SkillID) bool {
	return strings.HasPrefix(string(id), "sdd-")
}

type InjectionResult struct {
	Changed bool
	Files   []string
	Skipped []model.SkillID
}

// InjectWithCapability writes retained skill files for the given capability.
func InjectWithCapability(homeDir string, adapter agents.Adapter, skillIDs []model.SkillID, capability string) (InjectionResult, error) {
	if !adapter.SupportsSkills() {
		return InjectionResult{Skipped: skillIDs}, nil
	}

	skillDir := adapter.SkillsDir(homeDir)
	if skillDir == "" {
		return InjectionResult{Skipped: skillIDs}, nil
	}
	result, err := InjectDirectoryWithCapability(skillDir, skillIDs, capability, homeDir)
	if err != nil {
		return InjectionResult{}, err
	}
	support, err := injectSupportFiles(homeDir, adapter, skillDir, skillIDs)
	if err != nil {
		return InjectionResult{}, err
	}
	result.Changed = result.Changed || support.Changed
	result.Files = append(result.Files, support.Files...)
	return result, nil
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
		if IsSDDSkill(id) {
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
func InjectDirectoryWithCapability(skillDir string, skillIDs []model.SkillID, capability string, homeDir string) (InjectionResult, error) {
	return InjectDirectoryWithCapabilityWithWriter(skillDir, skillIDs, capability, homeDir, filemerge.WriteFileAtomic)
}

// InjectDirectoryWithWriter refreshes the shared ~/.agents/skills
// compatibility root with a caller-selected writer, keeping its
// physical-directory contract. Besides the selected skills it writes the
// skills/_shared references bound to the generic runtime slot, because every
// runtime reads this root (v3.7.0 behavior, #4471). It then removes the
// obsolete LegacySharedMarkerPath through removeFile, so every transaction
// implementation converges on the same on-disk result. removeFile reports
// whether a file was removed and is responsible for only removing a regular
// file.
func InjectDirectoryWithWriter(skillDir string, skillIDs []model.SkillID, writeFile func(string, []byte, fs.FileMode) (filemerge.WriteResult, error), removeFile func(string) (bool, error)) (InjectionResult, error) {
	// For backward compatibility, use os.UserHomeDir as fallback
	homeDir, _ := os.UserHomeDir()
	result, err := InjectDirectoryWithCapabilityWithWriter(skillDir, skillIDs, "", homeDir, writeFile)
	if err != nil {
		return InjectionResult{}, err
	}
	shared, err := injectSharedReferences(skillDir, compatibilityRuntimeSlot, writeFile)
	if err != nil {
		return InjectionResult{}, err
	}
	result.Changed = result.Changed || shared.Changed
	result.Files = append(result.Files, shared.Files...)
	marker := LegacySharedMarkerPath(skillDir)
	removed, err := removeFile(marker)
	if err != nil {
		return InjectionResult{}, fmt.Errorf("remove legacy compatibility shared marker: %w", err)
	}
	if removed {
		result.Changed = true
		result.Files = append(result.Files, marker)
	}
	return result, nil
}

// backupExisting copies dest into the timestamped backup dir before the
// injection overwrites it. The directory is private (the backups contain the
// user's prompt content), the file keeps the source's mode, and the creation
// is exclusive: two runs within the same second never truncate each other's
// backups — a collision gets a numeric suffix. Best-effort: failures log and
// let the injection proceed.
func backupExisting(homeDir, skillDir, dest, timestamp string) {
	rel, err := filepath.Rel(skillDir, dest)
	if err != nil {
		rel = dest
	}
	backupPath := filepath.Join(homeDir, ".gentle-ai", "backups", timestamp, rel)
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o700); err != nil {
		log.Printf("skills: failed to create backup dir for %s: %v", dest, err)
		return
	}
	srcFile, err := os.Open(dest)
	if err != nil {
		log.Printf("skills: failed to open existing file %s for backup: %v", dest, err)
		return
	}
	defer srcFile.Close()
	srcInfo, err := srcFile.Stat()
	if err != nil {
		log.Printf("skills: failed to stat %s for backup: %v", dest, err)
		return
	}
	if !srcInfo.Mode().IsRegular() {
		log.Printf("skills: %s is not a regular file; skipping backup", dest)
		return
	}
	dstFile, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, srcInfo.Mode())
	for suffix := 1; err != nil && os.IsExist(err) && suffix <= 9; suffix++ {
		dstFile, err = os.OpenFile(fmt.Sprintf("%s.%d", backupPath, suffix), os.O_WRONLY|os.O_CREATE|os.O_EXCL, srcInfo.Mode())
	}
	if err != nil {
		log.Printf("skills: failed to create backup file %s: %v", backupPath, err)
		return
	}
	defer dstFile.Close()
	if _, copyErr := io.Copy(dstFile, srcFile); copyErr != nil {
		log.Printf("skills: failed to backup %s to %s: %v", dest, backupPath, copyErr)
	}
}

// InjectDirectoryWithCapabilityWithWriter writes skills with a caller-selected writer.
func InjectDirectoryWithCapabilityWithWriter(skillDir string, skillIDs []model.SkillID, capability string, homeDir string, writeFile func(string, []byte, fs.FileMode) (filemerge.WriteResult, error)) (InjectionResult, error) {
	entries, skipped, err := directoryAssets(skillDir, skillIDs, capability)
	if err != nil {
		return InjectionResult{}, err
	}
	result := InjectionResult{Files: make([]string, 0, len(entries)), Skipped: skipped}
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}
	timestamp := time.Now().Format("20060102150405")
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
		dest := entry.destination
		// Backup existing file if present
		if homeDir != "" {
			if _, err := os.Stat(dest); err == nil {
				backupExisting(homeDir, skillDir, dest, timestamp)
			}
		}
		writeResult, err := writeFile(dest, []byte(content), 0o644)
		if err != nil {
			return InjectionResult{}, fmt.Errorf("write %q: %w", dest, err)
		}
		result.Changed = result.Changed || writeResult.Changed
		result.Files = append(result.Files, dest)
	}
	return result, nil
}

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
