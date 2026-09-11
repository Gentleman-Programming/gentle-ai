package openrecord

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// stagedFile is one file `openrecord skills --emit` wrote into the staging
// directory, read back from the real filesystem — the manifest-driven
// counterpart to directoryAssets, which only walks the compiled-in
// assets.FS embed and cannot enumerate a runtime staging dir.
type stagedFile struct {
	relativePath string
	content      []byte
}

func stagedFiles(stagingDir string) ([]stagedFile, error) {
	var files []stagedFile
	err := filepath.WalkDir(stagingDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(stagingDir, path)
		if err != nil {
			return err
		}
		if filepath.ToSlash(relative) == manifestName {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, stagedFile{relativePath: relative, content: content})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("enumerate staged openrecord skills: %w", err)
	}
	return files, nil
}

// fanOut writes the staged skills and manifest into every selected agent's
// skills directory, pruning each target with its own previous manifest first.
// An agent whose SkillsDir resolves to "" (Pi) is skipped without error. It
// returns how many agents actually received the files.
func fanOut(stagingDir, homeDir string, selectedAgents []model.AgentID) (int, error) {
	staged, err := stagedFiles(stagingDir)
	if err != nil {
		return 0, err
	}
	manifestBytes, err := os.ReadFile(filepath.Join(stagingDir, manifestName))
	if err != nil {
		return 0, fmt.Errorf("openrecord fan-out: read staged manifest: %w", err)
	}
	stagedManifest := loadManifest(stagingDir)

	reg, err := agents.NewDefaultRegistry()
	if err != nil {
		return 0, fmt.Errorf("openrecord fan-out: %w", err)
	}

	fanned := 0
	for _, id := range selectedAgents {
		adapter, ok := reg.Get(id)
		if !ok {
			continue
		}
		skillsDir := adapter.SkillsDir(homeDir)
		if skillsDir == "" {
			continue
		}
		if err := pruneStaleEntries(skillsDir, stagedManifest); err != nil {
			return fanned, fmt.Errorf("openrecord fan-out: prune %s: %w", skillsDir, err)
		}
		for _, file := range staged {
			destination := filepath.Join(skillsDir, filepath.FromSlash(file.relativePath))
			if _, err := filemerge.WriteFileAtomic(destination, file.content, 0o644); err != nil {
				return fanned, fmt.Errorf("openrecord fan-out: write %s: %w", destination, err)
			}
		}
		manifestPath := filepath.Join(skillsDir, manifestName)
		if _, err := filemerge.WriteFileAtomic(manifestPath, manifestBytes, 0o644); err != nil {
			return fanned, fmt.Errorf("openrecord fan-out: write manifest at %s: %w", skillsDir, err)
		}
		fanned++
	}
	return fanned, nil
}
