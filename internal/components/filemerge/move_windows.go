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

// displacedSuffix names the destination that was moved out of the way so a held
// file could be replaced. It is deterministic rather than random so a later run
// can clear a leftover instead of accumulating one per attempt.
const displacedSuffix = ".gentle-ai-displaced"

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
	displaced := dst + displacedSuffix
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

	// The new bytes are published. Whoever holds the old file still holds it, so
	// removing it is best effort — a leftover displaced file is not a failed
	// replacement and must not be reported as one.
	_ = os.Remove(displaced)
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
