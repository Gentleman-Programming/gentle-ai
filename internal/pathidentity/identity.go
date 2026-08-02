// Package pathidentity decides whether two filesystem paths name the same
// directory, and whether one path lies inside another.
//
// # The policy
//
// Identity is the operating system's answer, not the string's. Two paths name
// the same directory when a lookup of both reports the same device and inode
// (os.SameFile). Nothing here compares path strings to decide identity.
//
// This is the only rule that is correct on every volume gentle-ai runs on,
// because every property that makes two different spellings name one directory
// is decided by the filesystem during lookup and is not recoverable from the
// strings:
//
//   - symlinked ancestors, where /var and /private/var are one directory on
//     macOS and $TMPDIR lives under both spellings;
//   - case-insensitive lookup, where repo and REPO are one directory on a
//     default APFS or NTFS volume;
//   - Unicode equivalence, where the NFC and NFD spellings of an accented
//     name are one directory on APFS.
//
// filepath.EvalSymlinks resolves only the first of those three. A string
// comparison after EvalSymlinks therefore rejects the other two, which is the
// defect this package exists to remove. Canonicalizing one operand and not the
// other rejects all three, which is the defect it exists to prevent.
//
// # What this deliberately does not support
//
// Paths that do not exist. A lookup is the whole mechanism, so a path that has
// never been created has no identity: SameDirectory reports false for it, even
// against itself. Contains works around this in the one way that is sound — it
// asks the question of the deepest ancestor that does exist — so a not-yet-
// created descendant of a real root is still contained. Callers that need to
// compare two paths that both do not exist are outside this policy and must
// say so where they do it.
//
// Mount aliasing is the kernel's answer and is not second-guessed. One
// directory visible through two mounts of the same device is one identity
// here; two bind mounts that expose identical content from different devices
// are two identities. Hardlinked regular files are outside the question:
// SameDirectory answers about directories only.
//
// Identity is a point-in-time answer. A rename racing this call can invalidate
// it. Callers that need a race-free result must hold an open descriptor, which
// is what the secure no-follow walks in internal/reviewtransaction already do;
// this package is for the decisions those walks are configured from, not a
// replacement for them.
//
// Contains resolves symlinks on the way up, exactly as the lexical comparator
// it replaces did not. That is not a weakening: a path that reaches outside the
// root through a symlinked component was already lexically inside the root and
// was already accepted. Callers that must refuse such a path resolve it with
// filepath.EvalSymlinks before asking, and this package does not relieve them
// of that.
package pathidentity

import (
	"os"
	"path/filepath"
)

// SameDirectory reports whether left and right name the same existing
// directory. Both operands are treated identically: neither is canonicalized,
// because the lookup already is the canonicalization.
func SameDirectory(left, right string) bool {
	leftInfo, leftErr := os.Stat(left)
	if leftErr != nil || !leftInfo.IsDir() {
		return false
	}
	rightInfo, rightErr := os.Stat(right)
	if rightErr != nil || !rightInfo.IsDir() {
		return false
	}
	return os.SameFile(leftInfo, rightInfo)
}

// Contains reports whether path is root itself or lies beneath it, where root
// must be an existing directory. The walk climbs path's lexical ancestors and
// stops at the first one that is the same directory as root, so a descendant
// that does not exist yet is answered by the deepest ancestor that does. The
// walk is bounded by the number of separators in path and terminates at the
// filesystem or volume root.
func Contains(root, path string) bool {
	rootInfo, err := os.Stat(root)
	if err != nil || !rootInfo.IsDir() {
		return false
	}
	current, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for {
		if info, statErr := os.Stat(current); statErr == nil && info.IsDir() && os.SameFile(rootInfo, info) {
			return true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}
