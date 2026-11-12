//go:build windows

package shellinstaller

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// userWindowsRemoveReplacedBackup removes one <destination>.replaced-<UTC>
// tree the Windows installer left behind when it retired an installation
// during a reconfigure.
//
// The backup is removed only when it is an exact clean absolute sibling of
// destination whose name is filepath.Base(destination)+".replaced-" followed
// by a canonical UTC timestamp, and only when its canonical manifest, canonical
// selection, retained lock, original active destination, current SID and
// physical backup identity all validate. The physical identity and cleanup
// inventory are rechecked immediately before removal, and the backup must be
// absent afterward. Invalid, unrelated, uncertain or noncanonical trees are
// preserved and reported as an error, so nothing is ever removed by an
// unvalidated name glob.
func userWindowsRemoveReplacedBackup(destination, backup string) error {
	if err := userWindowsReplacedSibling(destination, backup); err != nil {
		return err
	}
	info, err := os.Lstat(backup)
	if err != nil {
		return fmt.Errorf("Windows replaced backup %q cannot be inspected; preserved: %w", backup, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("Windows replaced backup %q is not a directory tree; preserved", backup)
	}
	manifest, err := userWindowsManifestReadAs(backup, destination)
	if err != nil {
		return fmt.Errorf("uncertain Windows replaced backup %q; preserved: %w", backup, err)
	}
	// Recheck the physical identity and complete owned, alias-free cleanup
	// inventory immediately before removal, exactly as the stage remove does.
	observed, err := userWindowsIdentity(backup, true)
	if err != nil || observed != manifest.Identity {
		return errors.Join(err, fmt.Errorf("uncertain Windows replaced backup identity; preserved %q", backup))
	}
	if err := userWindowsInventory(backup, userWindowsInventoryCleanup); err != nil {
		return errors.Join(err, fmt.Errorf("uncertain Windows replaced backup inventory; preserved %q", backup))
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("Windows replaced backup %q could not be removed; preserved: %w", backup, err)
	}
	if _, err := os.Lstat(backup); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(err, fmt.Errorf("Windows replaced backup %q survived removal", backup))
	}
	return nil
}

// userWindowsReplacedSibling refuses every path that is not an exact clean
// absolute sibling of destination named <base>.replaced-<canonical UTC>. The
// timestamp must round-trip through the canonical UTC layout so a
// close-looking name can never reach the installer's removal path.
func userWindowsReplacedSibling(destination, backup string) error {
	refused := func(reason string) error {
		return fmt.Errorf("Windows replaced backup %q %s; preserved", backup, reason)
	}
	if !filepath.IsAbs(destination) || filepath.Clean(destination) != destination {
		return refused("targets a noncanonical active destination")
	}
	if !filepath.IsAbs(backup) || filepath.Clean(backup) != backup {
		return refused("is not a clean absolute path")
	}
	if filepath.Dir(backup) != filepath.Dir(destination) {
		return refused("is not an exact sibling of the active destination")
	}
	base := filepath.Base(destination)
	name := filepath.Base(backup)
	if name == base || !strings.HasPrefix(name, base+".replaced-") {
		return refused("does not use the canonical replaced-backup name")
	}
	stamp := strings.TrimPrefix(name, base+".replaced-")
	parsed, err := time.Parse("20060102T150405Z", stamp)
	if err != nil || parsed.Format("20060102T150405Z") != stamp {
		return refused("carries a noncanonical UTC timestamp")
	}
	return nil
}

// userWindowsPruneReplacedBackups bounds the Windows replaced-backup trees a
// reconfigure leaves beside the active installation. Candidates are exact
// siblings of destination discovered through the same userWindowsReplacedSibling
// validator the single removal uses, so unrelated bases, wrong markers and
// noncanonical timestamps are ignored. The exact keep (the backup just
// created) is always retained, and the lexically latest candidate is also
// retained when clock skew dated it in the future. Every other candidate is
// removed through the validated single-backup primitive; an uncertain
// candidate is preserved whole and reported in a joined error, so cleanup is
// best-effort and never retracts a valid installation.
func userWindowsPruneReplacedBackups(destination, keep string) error {
	if err := userWindowsReplacedSibling(destination, keep); err != nil {
		return err
	}
	prefix := filepath.Base(destination) + ".replaced-"
	keepName := filepath.Base(keep)
	keepStamp := strings.TrimPrefix(keepName, prefix)

	entries, err := os.ReadDir(filepath.Dir(destination))
	if err != nil {
		return fmt.Errorf("Windows replaced backups beside %q cannot be listed; preserved: %w", destination, err)
	}
	candidates := make([]string, 0, len(entries))
	for _, entry := range entries {
		candidate := filepath.Join(filepath.Dir(destination), entry.Name())
		if err := userWindowsReplacedSibling(destination, candidate); err != nil {
			continue // unrelated base, wrong marker or noncanonical timestamp
		}
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 0 {
		return nil
	}
	// Timestamps are the fixed-width canonical UTC layout, so lexical order is
	// chronological order; the last candidate is the lexically latest.
	sort.Strings(candidates)
	retain := map[string]bool{keepName: true}
	if latest := filepath.Base(candidates[len(candidates)-1]); latest != keepName && latest > keepStamp {
		retain[latest] = true
	}
	var errs []error
	for _, candidate := range candidates {
		if retain[filepath.Base(candidate)] {
			continue
		}
		if err := userWindowsRemoveReplacedBackup(destination, candidate); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
