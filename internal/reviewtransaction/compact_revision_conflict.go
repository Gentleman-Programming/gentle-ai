package reviewtransaction

import "fmt"

// CompactRevisionConflictError reports the one compare-and-set refusal that
// proves, from where it is produced rather than from its text, that this call
// wrote nothing: replaceContextGuarded compares the expected revision against
// the current one while holding the authority lock, and that comparison sits in
// front of the only write in the function. A caller that loses it did acquire
// the lock, did read the authority, and then refused to publish a successor
// derived from a revision another writer had already advanced.
//
// It is deliberately narrower than ErrConcurrentUpdate, which preconditions all
// over this package also report — batch reconciliation guards, gate chain
// validation, legacy HEAD checks, and several early request pre-checks. Only
// this type describes the guarded authority write itself, so only this type is
// safe to treat as a proven non-mutation, and only when nothing in the same
// operation committed a native transition first. That remains genuinely
// unknown, and the caller-side classification keeps it that way.
//
// It mirrors storeLockBusyError, which types the adjacent "never acquired the
// lock" refusal: both stay inside the ErrConcurrentUpdate chain so every
// existing bounded retry helper and precondition check behaves identically.
type CompactRevisionConflictError struct {
	LineageID string
	Expected  string
	Current   string
}

// Error renders byte-identically to the fmt.Errorf form this type replaced, so
// nothing that reads or matches the refusal text observes a change.
func (err *CompactRevisionConflictError) Error() string {
	return fmt.Sprintf("%v: expected compact revision %q, current %q", ErrConcurrentUpdate, err.Expected, err.Current)
}

func (err *CompactRevisionConflictError) Unwrap() error { return ErrConcurrentUpdate }
