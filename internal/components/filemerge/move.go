package filemerge

// DisplacedSuffix is the deterministic filename the Windows move path uses
// when a held destination cannot be overwritten in place. The previous binary
// is renamed to <dst><DisplacedSuffix> so the new binary can take dst's
// place; the suffix is deterministic (not random) so a later run knows
// exactly which file is the recovery path.
//
// The cleanup of any leftover DisplacedSuffix file is the caller's
// responsibility — after read-back verification, never before. Decoded as:
//
//	<recovery-state> = <dst><DisplacedSuffix>  (always, on Windows displacement)
//	                  ∅                       (when the move succeeded without
//	                                          hitting the displacement rung)
//
// and the caller reads dst, proves the swap, then os.Remove(<dst><DisplacedSuffix>)
// which is a no-op when no displacement happened.
const DisplacedSuffix = ".gentle-ai-displaced"

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
// On the Windows displacement rung the prior binary is left at
// <dst><DisplacedSuffix> until the caller proves the swap via read-back. This
// is the contract that decode2 (2026-08-18 PR #2715) review required: the
// cleanup of the displaced file is the caller's privilege, never the move's.
//
// This function is the mechanism, never the guarantee. The caller still has to
// read the destination back — a move that returns nil has not necessarily taken
// effect, and replaceDurably is where that is proven.
func MoveFileReplace(src, dst string) error {
	return moveFileReplace(src, dst)
}
