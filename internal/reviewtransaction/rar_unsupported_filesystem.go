package reviewtransaction

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/pathquote"
)

// UnsupportedAuthorityFilesystemError reports a review authority path -- the
// Git common directory or a directory under it -- whose filesystem cannot keep
// review authority private: an SMB/UNC share or mapped
// network drive (Remote), a local volume without persistent ownership such as
// exFAT (Filesystem), or a volume whose type could not be proven (Reason).
//
// #4670: these refusals used to end in "check the path and rerun", a repair
// no operator can perform because the path is already correct; the volume is
// what is wrong. The two continuations that change the outcome are turning
// reviews off machine-wide, which no longer depends on reading clone-local
// state, or moving
// the repository to a local NTFS or ReFS volume.
//
// It unwraps to the generic unsafe-path refusal, so every #1918 fail-closed
// caller matching that sentinel keeps refusing exactly as before.
type UnsupportedAuthorityFilesystemError struct {
	Path       string
	Filesystem string
	Remote     bool
	Reason     string
}

func (err *UnsupportedAuthorityFilesystemError) Error() string {
	var subject string
	switch {
	case err.Remote:
		subject = fmt.Sprintf("the review authority path %s is on a remote filesystem (an SMB/UNC share or mapped network drive)", pathquote.Quote(err.Path))
	case err.Filesystem != "":
		subject = fmt.Sprintf("the review authority path %s is on filesystem %q, which does not support persistent Windows ownership semantics", pathquote.Quote(err.Path), err.Filesystem)
	default:
		subject = fmt.Sprintf("the review authority path %s is on a filesystem whose ownership semantics cannot be proven", pathquote.Quote(err.Path))
	}
	if err.Reason != "" {
		subject += " (" + err.Reason + ")"
	}
	return subject + ", so review authority cannot be kept private there; " + UnsupportedAuthorityFilesystemContinuation
}

func (err *UnsupportedAuthorityFilesystemError) Unwrap() error { return errUnsafeRARAuthorityPath }

// UnsupportedAuthorityFilesystemContinuation is the documented way forward
// from an UnsupportedAuthorityFilesystemError, shared by every surface that
// reports one so the continuation cannot drift between them.
const UnsupportedAuthorityFilesystemContinuation = "run `gentle-ai review mode disable --scope global` to turn receipt-driven development off on this machine, or move the repository to a local NTFS or ReFS volume, then rerun the original command"

// windowsNetworkPath reports whether a Win32 path names a server share: the
// `\\server\share` form or its `\\?\UNC\` and `\\.\UNC\` namespace spellings.
//
// #5533: the Windows authority checks refuse every path in the `\\` family,
// and that refusal stays exactly as it is. Only its wording depends on this:
// the long-path and device forms of a drive letter or volume, such as
// `\\?\C:\repo\.git`, are on this machine and must not be reported as an
// SMB/UNC share. It is pure so tests execute it on every platform.
func windowsNetworkPath(path string) bool {
	for _, prefix := range []string{`\\?\UNC\`, `\\.\UNC\`} {
		if len(path) >= len(prefix) && strings.EqualFold(path[:len(prefix)], prefix) {
			return true
		}
	}
	return strings.HasPrefix(path, `\\`) &&
		!strings.HasPrefix(path, `\\?\`) && !strings.HasPrefix(path, `\\.\`)
}
