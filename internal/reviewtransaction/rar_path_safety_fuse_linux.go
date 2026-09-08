//go:build linux

package reviewtransaction

import "golang.org/x/sys/unix"

// posixFUSEProjectedOwnership reports whether path is hosted on a FUSE mount
// (statfs f_type == FUSE_SUPER_MAGIC, 0x65735546 -- the magic Linux reports
// for both plain FUSE and fuseblk, which is what ntfs-3g registers as). #2838
// was reported against exactly this shape: ntfs-3g mounted with a fixed
// user_id=0, which makes every path on the volume report owner uid 0
// regardless of who the mount is actually usable by.
//
// unix.Statfs_t's numeric filesystem-type field only exists on Linux; other
// platforms report filesystem type as a string (Statfs_t.Fstypename) with no
// comparable magic number, so this detection is Linux-only by construction --
// see rar_path_safety_fuse_other.go for every other POSIX platform.
func posixFUSEProjectedOwnership(path string) (bool, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return false, err
	}
	return uint32(stat.Type) == unix.FUSE_SUPER_MAGIC, nil
}
