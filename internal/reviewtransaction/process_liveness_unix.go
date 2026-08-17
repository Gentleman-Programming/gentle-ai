//go:build unix

package reviewtransaction

import (
	"errors"
	"syscall"
)

// processVerifiedDead reports whether pid verifiably does not name a running
// process on this host. Only a definitive "no such process" answer counts:
// permission refusals and every other ambiguity report false, so stale-lock
// removal guidance is only ever derived from proof (issue #3342).
func processVerifiedDead(pid int) bool {
	if pid <= 0 {
		return false
	}
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}
