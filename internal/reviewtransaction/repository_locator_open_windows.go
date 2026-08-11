//go:build windows

package reviewtransaction

import (
	"errors"

	"golang.org/x/sys/windows"
)

// isTransientSharingViolation reports whether err is the Windows
// ERROR_SHARING_VIOLATION returned by the final-file open of an immutable
// repository-context record. The locker is outside the process: publishers
// close temporary handles before MoveFileEx, but the kernel can still hold
// the destination open for a brief sharing-violation window after the rename,
// so the open must be tolerant of that transient state.
func isTransientSharingViolation(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}
