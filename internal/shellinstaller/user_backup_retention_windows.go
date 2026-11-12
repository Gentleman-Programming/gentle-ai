//go:build windows

package shellinstaller

// userWindowsRemoveReplacedBackup removes one <destination>.replaced-<UTC>
// tree the Windows installer left behind when it retired an installation
// during a reconfigure.
//
// Contract (implementation is T2): remove backup only when it is an exact
// sibling of destination whose name is filepath.Base(destination)+".replaced-"
// followed by a UTC timestamp, and only when its canonical manifest, canonical
// selection, retained lock, original active destination, current SID and
// physical backup identity all validate. The physical identity and cleanup
// inventory are rechecked immediately before removal. Invalid, unrelated,
// uncertain or noncanonical trees are preserved and reported as an error, so
// nothing is ever removed by an unvalidated name glob.
//
// This is the T1 no-op stub: it reports success without touching the disk so
// the Windows regression tests compile and fail behaviorally until validated
// removal lands.
func userWindowsRemoveReplacedBackup(destination, backup string) error {
	_, _ = destination, backup
	return nil
}
