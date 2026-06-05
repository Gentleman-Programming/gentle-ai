package backup

import (
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ManifestFilename = "manifest.json"

// ArchiveFilename is the name of the compressed archive inside a backup directory.
const ArchiveFilename = "snapshot.tar.gz"

// emptyFilesChecksum is the sentinel checksum used when no files exist.
// This allows consecutive zero-file backups to be correctly deduplicated.
var emptyFilesChecksum = fmt.Sprintf("%x", sha256.Sum256(nil))

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
		ID:         filepath.Base(snapshotDir),
		CreatedAt:  s.now().UTC(),
		RootDir:    snapshotDir,
		Entries:    make([]ManifestEntry, 0, len(paths)),
		Compressed: true,
	}

	// Collect archive entries and build manifest entries in one pass.
	var archiveEntries []ArchiveEntry
	var existingPaths []string

	for _, path := range paths {
		entry, archiveEntry, err := s.buildEntry(path)
		if err != nil {
			return Manifest{}, err
		}
		manifest.Entries = append(manifest.Entries, entry)
		if entry.Existed {
			manifest.FileCount++
			archiveEntries = append(archiveEntries, archiveEntry)
			existingPaths = append(existingPaths, archiveEntry.SourcePath)
		}
	}

	// Create the tar.gz archive with all existing files.
	// Skip archive creation when there are no files to back up.
	if len(archiveEntries) == 0 {
		manifest.Compressed = false
	} else {
		archivePath := filepath.Join(snapshotDir, ArchiveFilename)
		if err := CreateArchive(archivePath, archiveEntries); err != nil {
			return Manifest{}, fmt.Errorf("create archive %q: %w", archivePath, err)
		}
	}

	// Compute checksum from the source files for deduplication.
	// When there are no files, use the SHA-256 of the empty string as a stable
	// sentinel so consecutive zero-file backups are correctly detected as duplicates.
	var checksum string
	if len(existingPaths) == 0 {
		checksum = emptyFilesChecksum
	} else {
		var csErr error
		checksum, csErr = ComputeChecksum(existingPaths)
		if csErr != nil {
			// Non-fatal: skip checksum rather than failing the entire backup.
			log.Printf("backup: compute checksum: %v", csErr)
			checksum = ""
		}
	}
	manifest.Checksum = checksum

	// Write manifest.json outside the archive.
	if err := WriteManifest(filepath.Join(snapshotDir, ManifestFilename), manifest); err != nil {
		return Manifest{}, err
	}

	return manifest, nil
}

// buildEntry inspects a single source path and returns the ManifestEntry and
// (when the file exists) the ArchiveEntry to include in the archive.
func (s Snapshotter) buildEntry(sourcePath string) (ManifestEntry, ArchiveEntry, error) {
	cleanSource := filepath.Clean(sourcePath)
	entry := ManifestEntry{OriginalPath: cleanSource}

	info, err := os.Stat(cleanSource)
	if err != nil {
		if os.IsNotExist(err) {
			return entry, ArchiveEntry{}, nil
		}
		return ManifestEntry{}, ArchiveEntry{}, fmt.Errorf("stat source path %q: %w", cleanSource, err)
	}

	if !info.Mode().IsRegular() {
		// Skip directories and special runtime files such as sockets, FIFOs, and
		// devices. Backup archives only contain regular files that can be restored
		// safely as files.
		return entry, ArchiveEntry{}, nil
	}

	// Build the relative path inside the archive, mirroring the old files/ layout.
	relative := strings.TrimPrefix(cleanSource, filepath.VolumeName(cleanSource))
	relative = strings.TrimPrefix(relative, string(filepath.Separator))
	if relative == "" {
		relative = "root"
	}

	relPath := filepath.ToSlash(filepath.Join("files", relative))

	archiveEntry := ArchiveEntry{
		RelPath:    relPath,
		SourcePath: cleanSource,
		Mode:       info.Mode(),
	}

	entry.SnapshotPath = relPath
	entry.Existed = true
	entry.Mode = uint32(info.Mode())

	return entry, archiveEntry, nil
}
