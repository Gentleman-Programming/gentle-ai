package reviewtransaction

import (
	"path/filepath"
	"testing"
)

// canonicalTempDir hands tests the resolved spelling of t.TempDir() — the
// same shape every production store constructor produces before the secure
// lock walk ever runs (1773). On Darwin $TMPDIR lives under /var/folders and
// /var is a symlink into /private, so the raw spelling always has a symlinked
// ancestor, and the no-follow walk refuses that by design — see
// TestAcquireLocalStoreLockRefusesARawAliasedDirectory and
// TestSecureOpenLockParentRefusesASymlinkedAncestor. Any fixture that
// acquires a store lock must therefore canonicalize first, exactly like
// production callers do; handing the lock a raw t.TempDir() path reproduces
// the refusal on any platform whose temp directory sits behind a symlink.
func canonicalTempDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}
