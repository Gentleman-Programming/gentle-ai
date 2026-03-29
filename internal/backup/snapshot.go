package backup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ManifestFilename = "manifest.json"

type Snapshotter struct {
	now func() time.Time
}

func NewSnapshotter() Snapshotter {
	return Snapshotter{now: time.Now}
}

func (s Snapshotter) Create(snapshotDir string, paths []string) (Manifest, error) {
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		return Manifest{}, fmt.Errorf("create snapshot directory %q: %w", snapshotDir, err)
	}

	manifest := Manifest{
		ID:        filepath.Base(snapshotDir),
		CreatedAt: s.now().UTC(),
		RootDir:   snapshotDir,
		Entries:   make([]ManifestEntry, 0, len(paths)),
	}

	for _, path := range paths {
		entry, err := s.snapshotPath(snapshotDir, path)
		if err != nil {
			return Manifest{}, err
		}
		manifest.Entries = append(manifest.Entries, entry)
		if entry.Existed {
			manifest.FileCount++
		}
	}

	if err := WriteManifest(filepath.Join(snapshotDir, ManifestFilename), manifest); err != nil {
		return Manifest{}, err
	}

	return manifest, nil
}

func (s Snapshotter) snapshotPath(snapshotDir string, sourcePath string) (ManifestEntry, error) {
	cleanSource := filepath.Clean(sourcePath)
	entry := ManifestEntry{OriginalPath: cleanSource}

	info, err := os.Stat(cleanSource)
	if err != nil {
		if os.IsNotExist(err) {
			return entry, nil
		}
		return ManifestEntry{}, fmt.Errorf("stat source path %q: %w", cleanSource, err)
	}

	if info.IsDir() {
		return ManifestEntry{}, fmt.Errorf("backup source path %q is a directory", cleanSource)
	}

	// Strip volume name (e.g. "C:") on Windows so the remaining path
	// can be used safely as a relative directory inside the snapshot.
	relative := strings.TrimPrefix(cleanSource, filepath.VolumeName(cleanSource))
	relative = strings.TrimPrefix(relative, string(filepath.Separator))
	if relative == "" {
		relative = "root"
	}

	destination := filepath.Join(snapshotDir, "files", relative)
	if err := copyFile(cleanSource, destination, info.Mode()); err != nil {
		return ManifestEntry{}, err
	}

	entry.SnapshotPath = destination
	entry.Existed = true
	entry.Mode = uint32(info.Mode())
	return entry, nil
}

func copyFile(source string, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", source, err)
	}
	defer input.Close()

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create backup directory for %q: %w", destination, err)
	}

	output, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return fmt.Errorf("create snapshot file %q: %w", destination, err)
	}

	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return fmt.Errorf("copy %q to %q: %w", source, destination, err)
	}

	if err := output.Close(); err != nil {
		return fmt.Errorf("close snapshot file %q: %w", destination, err)
	}

	return nil
}
