//go:build windows

package filemerge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// displacedSuffixLocal is the package-private alias for filemerge.DisplacedSuffix.
// The suffix value lives in move.go so non-Windows callers (and tests) can
// reference it without dragging in <golang.org/x/sys/windows>; the build-tagged
// file here just reads from the public constant.
const displacedSuffixLocal = DisplacedSuffix

// moveFileReplace publishes src over dst on Windows.
//
// The chain, in order:
//
//  1. os.Rename. Go issues MoveFileEx(MOVEFILE_REPLACE_EXISTING), which is the
//     right call and wins whenever nothing holds dst.
//  2. MoveFileEx with MOVEFILE_REPLACE_EXISTING|MOVEFILE_WRITE_THROUGH. This is
//     a retry for the transient hold — an antivirus or the search indexer that
//     had dst open for the length of a scan — and it also flushes the rename to
//     disk before returning instead of leaving it in the cache.
//  3. displaceAndMove. dst is renamed aside and src is moved into its place.
//     Renaming a file that is open, or even currently executing, is legal on
//     Windows: the loader denies deletion of a running image, not renaming of
//     its directory entry. This is the rung that replaces a binary in use.
//
// MOVEFILE_DELAY_UNTIL_REBOOT is deliberately absent. It does not perform the
// move; it schedules it for the next boot, which lands after the read-back in
// replaceDurably has already run. The caller would then have to either report a
// failure while a mutation is silently pending, or claim a success it cannot
// verify. Both are the #2319 defect with extra steps, so a swap that cannot be
// proven now fails now.
//
// The displacement rung (3) leaves the prior binary at <dst><DisplacedSuffix>
// until the CALLER proves the swap. The displacement cleanup is the
// caller's privilege, never the move's (decode2 2026-08-18 PR #2715
// review): the move returning nil is not yet evidence the new bytes are
// on disk, so the displaced file is preserved through read-back. If
// read-back fails, the displaced file remains a recovery path for an
// operator who can decide by hand. If read-back succeeds, the caller
// deletes <dst><DisplacedSuffix> because dst is now verified.
//
// Every failure path leaves the installed file where it was, and names the cause
// so the caller can tell "held by another process" apart from "the directory
// refuses the write".
func moveFileReplace(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil || !destinationIsHeld(err) {
		return err
	}

	writeThroughErr := moveFileEx(src, dst, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
	if writeThroughErr == nil {
		return nil
	}
	if !destinationIsHeld(writeThroughErr) {
		return writeThroughErr
	}

	return displaceAndMove(src, dst, writeThroughErr)
}

// displaceAndMove replaces a destination that refuses to be overwritten while it
// is held, by moving it aside first.
//
// hold is the error that made this necessary; it travels into every message
// because "access is denied" on its own tells the user nothing about which file
// is in use. The displacement attempt also wraps its own error with %w so
// callers can match the underlying Windows errno (notably ERROR_ACCESS_DENIED)
// via errors.Is without string-scraping.
func displaceAndMove(src, dst string, hold error) error {
	const replace = windows.MOVEFILE_REPLACE_EXISTING | windows.MOVEFILE_WRITE_THROUGH
	displaced := dst + displacedSuffixLocal
	_ = os.Remove(displaced)

	if err := moveFileEx(dst, displaced, replace); err != nil {
		return fmt.Errorf("%q is held by another process (%v) and could not be moved aside to replace it: %w", dst, hold, err)
	}

	if err := moveFileEx(src, dst, replace); err != nil {
		// The installation must survive a refused swap. Put the displaced file
		// back under its own name before reporting.
		if restoreErr := moveFileEx(displaced, dst, replace); restoreErr != nil {
			return fmt.Errorf("%q could not be replaced (%w) and the original could not be restored from %q: %v. The installation is at %q", dst, err, displaced, restoreErr, displaced)
		}
		return fmt.Errorf("%q is held by another process (%v) and could not be replaced: %w", dst, hold, err)
	}

	// The new bytes are published. Whoever holds the old file still holds it,
	// so the prior binary MUST stay at `<dst><displacedSuffixLocal>` until the
	// caller proves the swap via read-back. Removing it here would be the
	// #2319 defect with extra steps — a swap that LOOKED successful from
	// the move call but whose read-back turned out to disagree with the
	// staged bytes would have left the user with no recovery path. The
	// caller (atomicReplace in internal/update/upgrade/download.go) deletes
	// `<dst><displacedSuffixLocal>` only AFTER fileDigest matches the staged
	// payload. If read-back fails, the displaced file persists, so an operator
	// can recover by hand.
	return nil
}

// destinationIsHeld reports whether err says the destination is in use rather
// than unwritable. These three are the Windows codes a hold produces:
// ERROR_ACCESS_DENIED (5), ERROR_SHARING_VIOLATION (32) and
// ERROR_LOCK_VIOLATION (33). Anything else is a real refusal and is returned
// unchanged instead of being retried against a wall.
func destinationIsHeld(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}

func moveFileEx(source, destination string, flags uint32) error {
	from, err := utf16Path(source)
	if err != nil {
		return err
	}
	to, err := utf16Path(destination)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, flags)
}

// utf16Path converts path to the wide-character form MoveFileEx takes, in the
// extended \\?\ namespace so a long install path is not silently rejected at
// MAX_PATH.
func utf16Path(path string) (*uint16, error) {
	extended, err := extendedPath(path)
	if err != nil {
		return nil, err
	}
	return windows.UTF16PtrFromString(extended)
}

func extendedPath(path string) (string, error) {
	if strings.HasPrefix(path, `\\?\`) {
		return filepath.Clean(path), nil
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	if strings.HasPrefix(absolute, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(absolute, `\\`), nil
	}
	return `\\?\` + absolute, nil
}
