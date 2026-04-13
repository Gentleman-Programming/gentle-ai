package backup

import (
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultRetentionCount is the default number of unpinned backups to keep.
// Pinned backups do not count toward this limit and are never pruned.
const DefaultRetentionCount = 5

// ComputeChecksum computes a composite SHA-256 checksum over the given file paths.
//
// Algorithm:
//  1. Filter to paths that exist and are regular files; skip others silently.
//  2. Sort filtered paths lexicographically (deterministic ordering).
//  3. For each path, compute SHA-256 of its contents.
//  4. Concatenate all "path:hexhash\n" pairs into a single string.
//  5. SHA-256 the concatenated result and return the hex-encoded digest.
//
// If paths is empty or all paths are missing/non-regular, ComputeChecksum
// returns ("", nil) — callers treat an empty checksum as "no dedup possible".
//
// Note: checksums include absolute file paths, so the same files at different
// paths (e.g., different home directories) will produce different checksums.
// This is intentional: dedup is per-machine, not cross-machine.
func ComputeChecksum(paths []string) (string, error) {
	type entry struct {
		path string
		hash string
	}

	var entries []entry
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", fmt.Errorf("stat %q: %w", p, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}

		data, err := os.ReadFile(p)
		if err != nil {
			return "", fmt.Errorf("read %q: %w", p, err)
		}

		sum := sha256.Sum256(data)
		entries = append(entries, entry{path: p, hash: fmt.Sprintf("%x", sum)})
	}

	if len(entries) == 0 {
		return "", nil
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].path < entries[j].path
	})

	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString(e.path)
		sb.WriteByte(':')
		sb.WriteString(e.hash)
		sb.WriteByte('\n')
	}

	composite := sha256.Sum256([]byte(sb.String()))
	return fmt.Sprintf("%x", composite), nil
}

// IsDuplicate reports whether newChecksum matches the checksum of the most
// recent backup found in backupDir.
//
// Returns false (never skips) when:
//   - newChecksum is empty
//   - no prior backups exist in backupDir
//   - the most recent backup has an empty Checksum (old manifest without dedup)
//
// Directories inside backupDir that do not contain a manifest.json are
// silently skipped.
func IsDuplicate(backupDir string, newChecksum string) (bool, error) {
	if newChecksum == "" {
		return false, nil
	}

	manifests, err := listManifests(backupDir)
	if err != nil {
		return false, err
	}

	if len(manifests) == 0 {
		return false, nil
	}

	// Find the most recent backup by CreatedAt.
	latest := manifests[0]
	for _, m := range manifests[1:] {
		if m.CreatedAt.After(latest.CreatedAt) {
			latest = m
		}
	}

	if latest.Checksum == "" {
		return false, nil
	}

	return latest.Checksum == newChecksum, nil
}

// Prune deletes the oldest unpinned backups in backupDir, keeping at most
// retentionCount unpinned backups. Pinned backups are never deleted.
//
// If retentionCount <= 0 the function is a no-op (unlimited retention).
//
// Directories inside backupDir that do not contain a readable manifest.json
// are silently skipped and do not count toward the limit.
//
// Deletion errors for individual backup directories are logged but do not
// abort the loop — remaining backups are still evaluated.
//
// Returns the list of deleted backup IDs.
func Prune(backupDir string, retentionCount int) ([]string, error) {
	if retentionCount <= 0 {
		return nil, nil
	}

	manifests, err := listManifests(backupDir)
	if err != nil {
		return nil, err
	}

	// Sort newest-first.
	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].CreatedAt.After(manifests[j].CreatedAt)
	})

	// Partition into pinned and unpinned.
	var unpinned []Manifest
	for _, m := range manifests {
		if !m.Pinned {
			unpinned = append(unpinned, m)
		}
	}

	if len(unpinned) <= retentionCount {
		return nil, nil
	}

	// Delete unpinned[retentionCount:] — the oldest excess backups.
	toDelete := unpinned[retentionCount:]
	var deleted []string
	for _, m := range toDelete {
		if err := DeleteBackup(m); err != nil {
			log.Printf("backup: prune: failed to delete %q: %v", m.RootDir, err)
			continue
		}
		deleted = append(deleted, m.ID)
	}

	return deleted, nil
}

// listManifests reads all backup directories inside backupDir and returns the
// parsed manifests. Subdirectories without a readable manifest.json are skipped.
func listManifests(backupDir string) ([]Manifest, error) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list backup dir %q: %w", backupDir, err)
	}

	var manifests []Manifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		manifestPath := filepath.Join(backupDir, e.Name(), ManifestFilename)
		m, err := ReadManifest(manifestPath)
		if err != nil {
			// Skip directories that don't have a valid manifest.
			continue
		}
		manifests = append(manifests, m)
	}

	return manifests, nil
}
