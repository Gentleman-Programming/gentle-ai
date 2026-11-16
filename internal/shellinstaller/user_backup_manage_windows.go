//go:build windows

package shellinstaller

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// userWindowsReplacedSizeEntries bounds how many filesystem entries the size
// walk inspects before it reports an incomplete lower bound instead of an
// exact total. It is a variable so tests can bound a tiny tree deterministically.
var userWindowsReplacedSizeEntries = 250000

// UserReplacedBackups lists the replaced-backup candidates beside install,
// sorted by their canonical UTC stamp, and marks the lexically latest. Listing
// is read-only and never validates manifests, so an invalid tree is still
// listed; removal stays with the validated retention primitives.
func UserReplacedBackups(install UserShellInstall) ([]UserReplacedBackup, error) {
	if err := userWindowsReplacedManaged(install); err != nil {
		return nil, err
	}
	candidates, err := userWindowsReplacedCandidates(install.Target)
	if err != nil {
		return nil, err
	}
	prefix := filepath.Base(install.Target) + ".replaced-"
	backups := make([]UserReplacedBackup, 0, len(candidates))
	for index, candidate := range candidates {
		bytes, complete := userWindowsReplacedSize(candidate)
		backups = append(backups, UserReplacedBackup{
			Path:     candidate,
			Stamp:    strings.TrimPrefix(filepath.Base(candidate), prefix),
			Bytes:    bytes,
			Complete: complete,
			Latest:   index == len(candidates)-1,
		})
	}
	return backups, nil
}

// UserPruneReplacedBackups removes every replaced-backup candidate except the
// lexically latest, which it passes to the validated retention primitive as
// the keep. Discovery is shared with the listing, so the candidate Manage
// marks latest is exactly the one retained. Zero or one candidate is a no-op;
// uncertain candidates are preserved and reported in a joined error.
func UserPruneReplacedBackups(install UserShellInstall) error {
	if err := userWindowsReplacedManaged(install); err != nil {
		return err
	}
	candidates, err := userWindowsReplacedCandidates(install.Target)
	if err != nil {
		return err
	}
	if len(candidates) < 2 {
		return nil
	}
	return userWindowsPruneReplacedBackups(install.Target, candidates[len(candidates)-1])
}

// userWindowsReplacedManaged refuses installs whose replaced backups Manage
// cannot own: only a Separate install at a clean absolute target retires
// trees as <target>.replaced-<UTC> siblings.
func userWindowsReplacedManaged(install UserShellInstall) error {
	if install.Mode != "separate" {
		return fmt.Errorf("replaced backups are managed only for Separate installs, not %q", install.Mode)
	}
	if !filepath.IsAbs(install.Target) || filepath.Clean(install.Target) != install.Target {
		return fmt.Errorf("replaced backups need a clean absolute target, not %q", install.Target)
	}
	return nil
}

// userWindowsReplacedSize sums the regular-file bytes of one candidate tree.
// It inspects at most userWindowsReplacedSizeEntries entries and never
// descends a reparse point; a bounded or failed walk reports the partial sum
// as an incomplete lower bound rather than a fabricated exact total.
func userWindowsReplacedSize(root string) (int64, bool) {
	var total int64
	entries, complete := 0, true
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, cause error) error {
		if cause != nil {
			complete = false
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if path != root {
			entries++
			if entries > userWindowsReplacedSizeEntries {
				complete = false
				return filepath.SkipAll
			}
		}
		info, err := entry.Info()
		if err != nil {
			complete = false
			return nil
		}
		if data, ok := info.Sys().(*syscall.Win32FileAttributeData); ok && data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		complete = false
	}
	return total, complete
}
