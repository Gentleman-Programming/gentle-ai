//go:build darwin

package reviewtransaction

import "golang.org/x/sys/unix"

// darwinOSearch is O_SEARCH from Darwin's <sys/fcntl.h>: O_EXEC (0x40000000)
// | O_DIRECTORY. golang.org/x/sys does not generate the constant for darwin,
// so it is spelled here from the kernel definition (xnu bsd/sys/fcntl.h).
const darwinOSearch = 0x40000000 | unix.O_DIRECTORY

// secureDirectoryOpenFlags opens ancestry components for search only, the
// Darwin analogue of the Linux O_PATH walk.
//
// O_SEARCH matters because the walk must traverse a search-only (0111)
// ancestor — the managed-profile shape 1781 exists for, pinned by
// TestAcquireLocalStoreLockAllowsSearchOnlyAncestor. Since xnu-8792 (macOS
// 13) FFLAGS strips the implicit FREAD whenever O_EXEC is set, and
// vn_authorize_open_existing authorizes KAUTH_VNODE_SEARCH instead of
// KAUTH_VNODE_READ_DATA for a directory opened FSEARCH|O_DIRECTORY, so this
// open needs execute permission only. The previous choice, O_EVTONLY, keeps
// the implicit FREAD of its O_RDONLY access mode and therefore fails with
// EACCES on a search-only ancestor — the exact failure the first Darwin CI
// run of that test surfaced. On kernels older than xnu-8792 the O_EXEC bit
// is unknown to FFLAGS and the open degrades to today's read-requiring
// open, so nothing regresses there.
//
// Alternatives rejected:
//   - Following symlinks during the walk would reopen the 1781 redirect
//     hole. O_NOFOLLOW stays, and a symlinked component still fails closed:
//     the no-follow lookup yields the link vnode and O_DIRECTORY rejects it
//     with ENOTDIR.
//   - Lstat-then-open per component leaves a TOCTOU window between the
//     check and the traversal; the atomic no-follow openat stays.
func secureDirectoryOpenFlags() int {
	return darwinOSearch | unix.O_CLOEXEC
}
