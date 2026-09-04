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
		// Only count files that have a SnapshotPath (regular files with archive content).
		// Directories and symlinks-to-directories have Existed=true but no archive entry.
		if entry.Existed && entry.SnapshotPath != "" {
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

	// Lstat: does NOT follow symlinks. We need the symlink's own mode
	// bits to detect it as a symlink before classifying the target.
	info, err := os.Lstat(cleanSource)
	if err != nil {
		if os.IsNotExist(err) {
			// Path does not exist on disk: it is a prospective install target,
			// not a user-owned artifact (sockets, FIFOs, devices, and dangling
			// symlinks are handled separately below). Mark it as a regular
			// file install target so the rollback removes anything the install
			// subsequently creates at this path. Dangling symlinks fall through
			// to the symlink branch and keep Kind="" (preserve), and special
			// files keep Kind="" via the IsRegular guard.
			entry.Kind = PathKindRegularFile
			return entry, ArchiveEntry{}, nil
		}
		return ManifestEntry{}, ArchiveEntry{}, fmt.Errorf("lstat source path %q: %w", cleanSource, err)
	}

	mode := info.Mode()

	switch {
	case mode.IsDir():
		// Empty directory. Record as Kind=directory, Existed=true,
		// not archived. Restore ensures the directory exists.
		entry.Kind = PathKindDirectory
		entry.Existed = true
		entry.Mode = uint32(mode)
		return entry, ArchiveEntry{}, nil

	case mode&os.ModeSymlink != 0:
		// Symlink. Read the target via Readlink. Classify the resolved
		// target via Stat (which DOES follow symlinks). Only treat as
		// Kind=symlink_directory when the target is a directory.
		// Leaf symlinks (target is a regular file) and dangling
		// symlinks fall through to the regular-file branch via
		// Stat(target) — same behavior as before, per the issue's
		// "leaf-symlink behavior remains outside this change".
		target, err := os.Readlink(cleanSource)
		if err != nil {
			return ManifestEntry{}, ArchiveEntry{}, fmt.Errorf("read symlink %q: %w", cleanSource, err)
		}
		targetInfo, statErr := os.Stat(cleanSource)
		if statErr == nil && targetInfo.IsDir() {
			entry.Kind = PathKindSymlinkDirectory
			entry.LinkTarget = target
			entry.Existed = true
			entry.Mode = uint32(mode.Perm())
			return entry, ArchiveEntry{}, nil
		}
		// Fall through to regular-file classification below.
		// Replace `info` so the archive-entry builder uses the
		// resolved target's mode rather than the symlink's Lstat mode.
		if statErr == nil {
			info = targetInfo
		} else {
			// Dangling symlink: record as unknown (Existed=false).
			// We don't archive it; restore leaves it as the user wrote it.
			return entry, ArchiveEntry{}, nil
		}
	}

	if !info.Mode().IsRegular() {
		// Sockets, FIFOs, devices, and any other non-regular type
		// continue to be skipped (Existed=false). Per the issue,
		// these are explicitly out of scope.
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
	entry.Kind = PathKindRegularFile

	return entry, archiveEntry, nil
}
