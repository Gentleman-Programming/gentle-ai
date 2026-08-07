package filemerge

// MoveFileReplace publishes src over dst, replacing whatever dst holds.
//
// On Unix this is os.Rename and nothing else: a same-directory rename is already
// atomic and already replaces an executing binary, because the running process
// keeps its descriptor to the old inode while the directory entry moves.
//
// On Windows os.Rename is only the first attempt. A destination held by
// antivirus, the search indexer, or the running image itself makes MoveFileEx
// fail with ERROR_ACCESS_DENIED, ERROR_SHARING_VIOLATION, or
// ERROR_LOCK_VIOLATION, and those are the shapes #2319 reported as "upgrade
// succeeded, binary unchanged". moveFileReplace on that platform walks a
// fallback chain instead of surrendering; see move_windows.go for the order and
// for why MOVEFILE_DELAY_UNTIL_REBOOT is not part of it.
//
// This function is the mechanism, never the guarantee. The caller still has to
// read the destination back — a move that returns nil has not necessarily taken
// effect, and replaceDurably is where that is proven.
func MoveFileReplace(src, dst string) error {
	return moveFileReplace(src, dst)
}
