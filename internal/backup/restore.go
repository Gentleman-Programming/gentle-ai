package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
)

// UserHomeDirFn is the function used to resolve the user's home directory.
// Package-level var for testability — swapped in tests to use a temp directory.
var UserHomeDirFn = os.UserHomeDir

// isPathUnderRoot reports whether path is an absolute path that resides under
// root. This is used to prevent arbitrary file writes via tampered manifest
// OriginalPath fields: root must come from the caller (something it knows out
// of band, e.g. the home or workspace directory it actually installed into),
// never from the manifest itself — a tampered manifest would otherwise simply
// declare a wider root and walk straight through the guard.
//
// Symlink note: the path itself may not exist yet (typical during restore),
// so the check walks up to the closest existing ancestor and EvalSymlinks
// that ancestor. If an ancestor such as `/r/foo` is a symlink to `/etc`,
// writing `/r/foo/bar` would land in `/etc/bar` and escape the root; the
// ancestor-resolution step catches that case before the write. If no
// ancestor exists (the entire path is claim-only), fall back to the textual
// prefix check.
func isPathUnderRoot(path, root string) bool {
	rootClean := filepath.Clean(root)
	if rootClean == "" || rootClean == "." {
		return false
	}
	clean := filepath.Clean(path)
	if !strings.HasPrefix(clean, rootClean+string(filepath.Separator)) {
		return false
	}

	// Resolve the deepest existing ancestor of `clean` so we catch symlink
	// escapes buried in the parent directory chain — not only the leaf
	// path itself, which by definition is the file we are about to create.
	resolvedRoot := rootClean
	if rr, err := filepath.EvalSymlinks(rootClean); err == nil {
		resolvedRoot = rr
	}

	evalTarget := clean
	if _, err := os.Lstat(clean); err != nil {
		// Walk up the parent chain until we find an existing component or hit
		// the root itself.
		walk := clean
		for walk != rootClean && walk != "" && walk != "/" && walk != "." {
			parent := filepath.Dir(walk)
			if parent == walk {
				walk = rootClean
				break
			}
			walk = parent
			if _, statErr := os.Lstat(walk); statErr == nil {
				evalTarget = walk
				break
			}
		}
	}
	resolvedClean := clean
	if r, err := filepath.EvalSymlinks(evalTarget); err == nil {
		rel, relErr := filepath.Rel(evalTarget, clean)
		if relErr == nil {
			resolvedClean = filepath.Join(r, rel)
		}
	}

	return strings.HasPrefix(resolvedClean, resolvedRoot+string(filepath.Separator))
}

// isPathUnderRoots reports whether path resides under at least one of roots.
func isPathUnderRoots(path string, roots []string) bool {
	for _, root := range roots {
		if isPathUnderRoot(path, root) {
			return true
		}
	}
	return false
}

// invalidOriginalPathErr builds the refusal returned when a manifest entry's
// OriginalPath does not resolve under any allowed root. It names the roots
// actually validated against so a user who hits it can tell which boundary
// was crossed.
func invalidOriginalPathErr(originalPath string, roots []string) error {
	return fmt.Errorf("manifest entry has invalid OriginalPath %q: must be an absolute path under an allowed root (%s)", originalPath, strings.Join(roots, ", "))
}

// RestoreService restores a backup manifest, writing back or removing files
// at their OriginalPath.
//
// Roots restricts which directories a restore is allowed to write to or
// remove from. Every non-pre-existing manifest entry's OriginalPath must
// resolve under at least one Roots entry, or the restore refuses it.
//
// When Roots is empty, Restore falls back to the single directory returned by
// UserHomeDirFn — the historical, backward-compatible behavior. This is the
// correct default for standalone restores (`gentle-ai restore <id>` and the
// TUI "restore from list" screen): the backup being restored may be
// arbitrarily old, and the workspace root that was in effect when it was
// created is not something the current process can safely rediscover on its
// own, so it falls back to the one directory it explicitly owns.
//
// Callers that are still inside the same install/sync run that produced the
// manifest (e.g. pipeline rollback) know their own scope roots out of band
// and should set Roots explicitly — see rollbackRoots in internal/cli.
type RestoreService struct {
	Roots []string
}

// allowedRoots resolves the roots this restore may write under.
func (s RestoreService) allowedRoots() ([]string, error) {
	if len(s.Roots) > 0 {
		return s.Roots, nil
	}
	home, err := UserHomeDirFn()
	if err != nil {
		return nil, err
	}
	return []string{home}, nil
}

// reconcileExistingSymlink enforces the symlink collision-detection contract
// from decode2's 2026-08-18 PR #2021 review. When a manifest entry declares a
// symlink-directory that already exists on disk, the on-disk node must BOTH
// still be a symlink AND point at the recorded LinkTarget. Any divergence is
// a fail-closed refusal: recreating would risk redirecting an
// outside-the-context entity (a real symlink pointed at a real codepath by
// an admin) toward whatever target the manifest happens to claim.
//
// When the symlink is missing on disk the function recreates it after
// validateSymlinkTarget accepts the target.
func reconcileExistingSymlink(entry ManifestEntry, roots []string) error {
	info, err := os.Lstat(entry.OriginalPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat pre-existing symlink %q: %w", entry.OriginalPath, err)
		}
		// Missing on disk: validate target and recreate.
		if err := validateSymlinkTarget(entry.LinkTarget, entry.OriginalPath, roots); err != nil {
			return fmt.Errorf("manifest entry %q has unsafe LinkTarget %q: %w", entry.OriginalPath, entry.LinkTarget, err)
		}
		if err := os.MkdirAll(filepath.Dir(entry.OriginalPath), 0o755); err != nil {
			return fmt.Errorf("create parent directory for symlink %q: %w", entry.OriginalPath, err)
		}
		if err := os.Symlink(entry.LinkTarget, entry.OriginalPath); err != nil {
			return fmt.Errorf("restore symlink %q -> %q: %w", entry.OriginalPath, entry.LinkTarget, err)
		}
		return nil
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("manifest entry %q declares symlink-directory but on-disk node is %s — refusing to clobber a non-symlink with a symlink", entry.OriginalPath, info.Mode().String())
	}
	existingTarget, readErr := os.Readlink(entry.OriginalPath)
	if readErr != nil {
		return fmt.Errorf("readlink pre-existing symlink %q: %w", entry.OriginalPath, readErr)
	}
	if existingTarget != entry.LinkTarget {
		return fmt.Errorf("manifest entry %q declares LinkTarget %q but on-disk symlink points at %q — refusing to redirect", entry.OriginalPath, entry.LinkTarget, existingTarget)
	}
	return nil
}

// validateSymlinkTarget rejects LinkTarget values that could be exploited to
// follow symlinks outside the allowed roots. The policy: must be non-empty,
// relative (no leading separator), and resolve under one of roots when joined
// against filepath.Dir(originalPath). The resolve step normalizes both
// forward-slash `..` and Windows-style `..\..` traversal forms via
// filepath.Clean, replacing the forward-slash-only substring check used
// before #2451 that let `..\..` slip through.
func validateSymlinkTarget(target, originalPath string, roots []string) error {
	if target == "" {
		return fmt.Errorf("empty target")
	}
	if filepath.IsAbs(target) || strings.HasPrefix(target, "/") || strings.HasPrefix(target, `\`) {
		return fmt.Errorf("absolute targets are not allowed")
	}
	// Resolve against the link's parent and re-check containment against roots.
	// filepath.Clean normalizes both / and \ separators on Windows, so this
	// catches traversal sequences the forward-slash-only check used to miss.
	parent := filepath.Dir(originalPath)
	resolved := filepath.Clean(filepath.Join(parent, target))
	if !isPathUnderRoots(resolved, roots) {
		return fmt.Errorf("target %q resolved to %q is outside allowed roots", target, resolved)
	}
	return nil
}

func (s RestoreService) Restore(manifest Manifest) error {
	if manifest.Compressed {
		return s.restoreCompressed(manifest)
	}
	return s.restorePlain(manifest)
}

// restoreCompressed handles backups where Compressed==true.
// It extracts the tar.gz archive into a temp directory, then restores each
// entry by resolving the relative SnapshotPath inside that temp directory.
//
// PathKind semantics for Existed entries:
//   - PathKindDirectory: ensure the directory still exists; never delete.
//   - PathKindSymlinkDirectory: validate LinkTarget (relative, contained
//     under roots) and recreate the symlink if missing on disk.
//   - PathKindRegularFile / PathKindUnknown (legacy): restore from the
//     archive via restoreEntry.
//
// For !Existed entries, only PathKindRegularFile is removed. Kind=""
// (legacy manifests) and directory/symlink kinds are preserved because the
// install never proved it created them, so deleting them could nuke a path
// the user owns. This is a behavior change vs. pre-#2021: legacy manifests
// whose Kind=="" and Existed==false will no longer be deleted on rollback —
// callers that need the old behavior must rewrite their manifests with
// Kind="regular".
func (s RestoreService) restoreCompressed(manifest Manifest) error {
	roots, err := s.allowedRoots()
	if err != nil {
		return fmt.Errorf("resolve restore roots: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "gentle-ai-restore-*")
	if err != nil {
		return fmt.Errorf("create temp restore dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	archivePath := filepath.Join(manifest.RootDir, ArchiveFilename)
	if _, err := ExtractArchive(archivePath, tempDir); err != nil {
		return fmt.Errorf("extract archive %q: %w", archivePath, err)
	}

	for _, entry := range manifest.Entries {
		// All paths must be absolute and under an allowed root — both
		// Existed and !Existed branches share the same containment check.
		if !filepath.IsAbs(entry.OriginalPath) || !isPathUnderRoots(entry.OriginalPath, roots) {
			return invalidOriginalPathErr(entry.OriginalPath, roots)
		}

		switch {
		case entry.Kind == PathKindDirectory && entry.Existed:
			// Pre-existing directory: ensure it still exists; never delete.
			if err := os.MkdirAll(entry.OriginalPath, os.FileMode(entry.Mode)); err != nil {
				return fmt.Errorf("ensure pre-existing directory %q: %w", entry.OriginalPath, err)
			}
			continue

		case entry.Kind == PathKindSymlinkDirectory && entry.Existed:
			// Pre-existing symlink to a directory: verify type and recorded
			// target against the manifest BEFORE doing anything (decode2
			// 2026-08-18 PR #2021 review). The previous code accepted any
			// existing node at the path; a tampered manifest could claim a
			// regular file is a symlink, or vice versa, and the restore
			// would silently accept the disk's actual node without cross-
			// checking. We must either (a) validate the on-disk symlink
			// type and target, or (b) recreate from the manifest. We pick
			// (a) because recreating risks redirecting an outside-the-root
			// entity if the manifest's LinkTarget diverges from the
			// on-disk one — refusing closed is the safer side.
			info, err := os.Lstat(entry.OriginalPath)
			if err != nil {
				if !os.IsNotExist(err) {
					return fmt.Errorf("stat pre-existing symlink %q: %w", entry.OriginalPath, err)
				}
				// Symlink is missing on disk: validate and recreate.
				if err := validateSymlinkTarget(entry.LinkTarget, entry.OriginalPath, roots); err != nil {
					return fmt.Errorf("manifest entry %q has unsafe LinkTarget %q: %w", entry.OriginalPath, entry.LinkTarget, err)
				}
				if err := os.MkdirAll(filepath.Dir(entry.OriginalPath), 0o755); err != nil {
					return fmt.Errorf("create parent directory for symlink %q: %w", entry.OriginalPath, err)
				}
				if err := os.Symlink(entry.LinkTarget, entry.OriginalPath); err != nil {
					return fmt.Errorf("restore symlink %q -> %q: %w", entry.OriginalPath, entry.LinkTarget, err)
				}
				continue
			}
			if info.Mode()&os.ModeSymlink == 0 {
				return fmt.Errorf("manifest entry %q declares symlink-directory but on-disk node is %s — refusing to clobber a non-symlink with a symlink", entry.OriginalPath, info.Mode().String())
			}
			existingTarget, readErr := os.Readlink(entry.OriginalPath)
			if readErr != nil {
				return fmt.Errorf("readlink pre-existing symlink %q: %w", entry.OriginalPath, readErr)
			}
			if existingTarget != entry.LinkTarget {
				return fmt.Errorf("manifest entry %q declares LinkTarget %q but on-disk symlink points at %q — refusing to redirect", entry.OriginalPath, entry.LinkTarget, existingTarget)
			}
			continue

		case entry.Existed:
			// Existed=true + Kind unknown or regular: treat as regular file
			// (legacy compatibility). Existing restore path applies.
			// SnapshotPath must be relative inside the archive (e.g. "files/.config/foo.json").
			// An absolute path would cause filepath.Join to ignore tempDir, reading from
			// the live filesystem instead of the extraction directory.
			if filepath.IsAbs(entry.SnapshotPath) {
				return fmt.Errorf("manifest entry %q has absolute SnapshotPath %q, expected relative", entry.OriginalPath, entry.SnapshotPath)
			}
			resolvedEntry := ManifestEntry{
				OriginalPath: entry.OriginalPath,
				SnapshotPath: filepath.Join(tempDir, filepath.FromSlash(entry.SnapshotPath)),
				Existed:      true,
				Mode:         entry.Mode,
			}
			if err := restoreEntry(resolvedEntry, true, roots); err != nil {
				return err
			}
			continue
		}

		// !Existed branch: what to do depends on Kind.
		switch entry.Kind {
		case PathKindRegularFile:
			// PathKindRegularFile explicitly opts into deletion semantics:
			// the install may have created this file and the manifest
			// records it as absent, so it is safe to delete.
			if err := os.Remove(entry.OriginalPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove path %q: %w", entry.OriginalPath, err)
			}
		default:
			// Kind="" (legacy unknown), Kind=directory, Kind=symlink_directory:
			// we never proved the install created this path. Preserve it.
		}
	}

	return nil
}

// restorePlain handles old-style backups where Compressed==false.
// SnapshotPath is an absolute path to a plain file on disk.
//
// PathKind semantics mirror restoreCompressed: directory/symlink-directory
// entries are preserved (never deleted); only PathKindRegularFile and the
// legacy PathKindUnknown / "regular" case are removed when Existed==false.
// See the note in restoreCompressed about the behavior change for legacy
// manifests whose Kind=="" and Existed==false.
func (s RestoreService) restorePlain(manifest Manifest) error {
	roots, err := s.allowedRoots()
	if err != nil {
		return fmt.Errorf("resolve restore roots: %w", err)
	}

	for _, entry := range manifest.Entries {
		// All paths must be absolute and under an allowed root — both
		// Existed and !Existed branches share the same containment check.
		if !filepath.IsAbs(entry.OriginalPath) || !isPathUnderRoots(entry.OriginalPath, roots) {
			return invalidOriginalPathErr(entry.OriginalPath, roots)
		}

		switch {
		case entry.Kind == PathKindDirectory && entry.Existed:
			// Pre-existing directory: ensure it still exists; never delete.
			if err := os.MkdirAll(entry.OriginalPath, os.FileMode(entry.Mode)); err != nil {
				return fmt.Errorf("ensure pre-existing directory %q: %w", entry.OriginalPath, err)
			}
			continue

		case entry.Kind == PathKindSymlinkDirectory && entry.Existed:
			if err := reconcileExistingSymlink(entry, roots); err != nil {
				return err
			}
			continue

		case entry.Existed:
			if err := restoreEntry(entry, false, roots); err != nil {
				return err
			}
			continue
		}

		// !Existed branch: what to do depends on Kind.
		switch entry.Kind {
		case PathKindRegularFile:
			// PathKindRegularFile explicitly opts into deletion semantics.
			if err := os.Remove(entry.OriginalPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove path %q: %w", entry.OriginalPath, err)
			}
		default:
			// Kind="" (legacy unknown), Kind=directory, Kind=symlink_directory:
			// we never proved the install created this path. Preserve it.
		}
	}

	return nil
}

// restoreEntry writes the snapshot file at entry.SnapshotPath back to entry.OriginalPath.
// trustedSnapshot must be true when SnapshotPath has already been resolved to a safe
// temp directory (compressed restores), skipping the isRootDirUnderBackupRoot check.
// It must be false for plain restores where SnapshotPath comes directly from the manifest
// and must be validated against the backup root to prevent arbitrary file reads.
func restoreEntry(entry ManifestEntry, trustedSnapshot bool, roots []string) error {
	if !filepath.IsAbs(entry.OriginalPath) || !isPathUnderRoots(entry.OriginalPath, roots) {
		return invalidOriginalPathErr(entry.OriginalPath, roots)
	}

	// Validate SnapshotPath is under the backup root to prevent reading arbitrary
	// files from the filesystem via a tampered manifest (e.g. SnapshotPath: "/etc/shadow").
	// Skip this check for trusted snapshots (compressed restores) where SnapshotPath
	// has already been resolved to a safe temp directory by restoreCompressed.
	if !trustedSnapshot {
		ok, err := isRootDirUnderBackupRoot(entry.SnapshotPath)
		if err != nil || !ok {
			return fmt.Errorf("manifest entry has invalid SnapshotPath %q: must be under the backup root directory", entry.SnapshotPath)
		}
	}

	content, err := os.ReadFile(entry.SnapshotPath)
	if err != nil {
		return fmt.Errorf("read snapshot file %q: %w", entry.SnapshotPath, err)
	}

	if err := os.MkdirAll(filepath.Dir(entry.OriginalPath), 0o755); err != nil {
		return fmt.Errorf("create restore directory for %q: %w", entry.OriginalPath, err)
	}

	if _, err := filemerge.WriteFileAtomic(entry.OriginalPath, content, os.FileMode(entry.Mode)); err != nil {
		return fmt.Errorf("restore path %q: %w", entry.OriginalPath, err)
	}

	return nil
}
