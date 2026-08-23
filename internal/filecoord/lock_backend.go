package filecoord

import (
	"errors"
	"os"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// acquireCooperativeLock backs the cooperative contract with the hardened
// authority-lock primitive instead of a second platform implementation:
// reviewtransaction.AcquireAuthorityFileLock owns the secure no-follow open
// walk (O_NOFOLLOW per component on POSIX, reparse-safe NtCreateFile on
// Windows) and kernel advisory locking (flock / LockFileEx), and never treats
// lock-file residue as ownership. Symlinked roots and lock paths are rejected
// by that walk, so callers must pass canonical lock roots. A later refactor
// may move the primitive into this package and flip the dependency; reusing
// it in place keeps one lock mechanism in the product (#3632).
func acquireCooperativeLock(path string) (*Lease, error) {
	authority, err := reviewtransaction.AcquireAuthorityFileLock(path)
	if err != nil {
		if errors.Is(err, reviewtransaction.ErrStoreLockContended) {
			return nil, &BusyError{Cause: err}
		}
		return nil, &OperationalError{Cause: err}
	}
	return &Lease{unlock: func(*os.File) error { return authority.Release() }}, nil
}
