//go:build !windows && !linux

package reviewtransaction

// posixFUSEProjectedOwnership is the non-Linux POSIX default: #2838's
// evidence (ntfs-3g/fuseblk reporting FUSE_SUPER_MAGIC) is Linux-specific --
// golang.org/x/sys/unix.Statfs_t reports filesystem type as a string
// (Fstypename) rather than a numeric magic on Darwin and the BSDs, with no
// cross-referenceable magic number in scope here -- so every other POSIX
// platform reports "not FUSE-projected" rather than guessing at one.
func posixFUSEProjectedOwnership(string) (bool, error) {
	return false, nil
}
