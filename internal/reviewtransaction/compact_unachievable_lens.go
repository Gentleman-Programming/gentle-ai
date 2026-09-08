package reviewtransaction

import (
	"context"
	"errors"
	"strings"
	"time"
)

// CompactUnachievableLensAttemptRequest binds one host declaration that a
// selected lens slot could not be completed under current conditions (for
// example a reviewer prompt exceeding a host relay's transport bound) to the
// exact frozen capture phase revision, target identity, lens, selected order,
// and SubjectHash the collect transition already offered for that slot.
type CompactUnachievableLensAttemptRequest struct {
	ExpectedRevision string
	TargetIdentity   string
	Lens             string
	SelectedOrder    int
	SubjectHash      string
	Reason           string
	Detail           string
}

// RecordUnachievableLensAttempt persists one bound "this slot cannot be
// completed" declaration under the authority lock, so a restarted negotiated
// STATUS returns the same answer instead of re-offering the identical
// capture (issue #3442). recorded is true when this call actually appended a
// new declaration, and false on exact replay -- the same order with the same
// subject hash, reason, and detail already recorded, which returns success
// without mutation. This polarity matches WithdrawUnachievableLensAttempt's
// removed bool below (true means "this call mutated the authority"), so a
// caller never has to remember which of the two boolean returns is inverted.
// A declaration for an order that already carries a different one is refused
// rather than silently overwritten: WithdrawUnachievableLensAttempt below is
// the one way to change what a frozen phase already recorded for a slot, so
// a host that mistook a transient failure for a deterministic one is never
// stuck.
func (store CompactStore) RecordUnachievableLensAttempt(ctx context.Context, request CompactUnachievableLensAttemptRequest) (recorded bool, err error) {
	if ctx == nil {
		return false, errors.New("unachievable lens attempt context is nil") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	reason := strings.TrimSpace(request.Reason)
	if request.SelectedOrder < 0 || strings.TrimSpace(request.Lens) == "" || !validSHA256(request.ExpectedRevision) ||
		!validSHA256(request.TargetIdentity) || !validSHA256(request.SubjectHash) || reason == "" {
		return false, errors.New("unachievable lens attempt requires a valid binding and reason") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	maintenance, err := store.acquireReadMaintenance(ctx)
	if err != nil {
		return false, err
	}
	if maintenance != nil {
		defer maintenance.Release()
	}
	deadline := time.NewTimer(maintenanceLockTimeout)
	defer deadline.Stop()
	var lock *storeLock
	for {
		lock, err = acquireStoreLock(store.lockPath)
		if !errors.Is(err, ErrConcurrentUpdate) {
			break
		}
		select {
		case <-deadline.C:
			return false, &AuthorityLockTimeoutError{Timeout: maintenanceLockTimeout}
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err != nil {
		return false, err
	}
	defer lock.release()
	record, err := store.loadCompactRecordLocked()
	if err != nil {
		return false, err
	}
	if record.HistoricalCompat {
		return false, NewLegacyReadOnlyError("review/capture-unachievable", record.State.LineageID)
	}
	state := record.State
	if state.State != StateReviewing || state.CapturePhaseRevision != request.ExpectedRevision ||
		state.InitialSnapshot.Identity != request.TargetIdentity ||
		request.SelectedOrder >= len(state.SelectedLenses) || state.SelectedLenses[request.SelectedOrder] != request.Lens {
		return false, errors.New("unachievable lens attempt does not match the current reviewing authority; refresh the binding with gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition")
	}
	if _, found, activeErr := state.ActiveAdmittedLensResult(request.SelectedOrder); activeErr != nil {
		return false, activeErr
	} else if found {
		return false, errors.New("this lens slot already holds a captured reviewer result; run gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition to see its admitted artifact")
	}
	for _, existing := range state.UnachievableLensAttempts {
		if existing.SelectedOrder != request.SelectedOrder {
			continue
		}
		if existing.SubjectHash == request.SubjectHash && existing.Reason == reason && existing.Detail == request.Detail {
			// Exact replay: already recorded, nothing to mutate.
			return false, nil
		}
		return false, errors.New("a different unachievable declaration already exists for this lens slot; refresh the negotiated collection with gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition")
	}
	next := cloneCompactStateInitialAtomicStart(state)
	next.UnachievableLensAttempts = append(next.UnachievableLensAttempts, CompactUnachievableLensAttempt{
		CapturePhaseRevision: request.ExpectedRevision, TargetIdentity: request.TargetIdentity,
		Lens: request.Lens, SelectedOrder: request.SelectedOrder, SubjectHash: request.SubjectHash,
		Reason: reason, Detail: strings.TrimSpace(request.Detail),
	})
	_, payload, buildErr := makeCompactRecord(next)
	if buildErr != nil {
		return false, buildErr
	}
	if writeErr := writeAtomic(store.StatePath(), payload, 0o644); writeErr != nil {
		return false, writeErr
	}
	return true, nil
}

// CompactUnachievableLensWithdrawalRequest binds a request to retract one
// previously recorded unachievable declaration to the exact frozen capture
// phase revision, target identity, and SubjectHash the declaration was made
// against -- the same tokens RecordUnachievableLensAttempt required, minus
// the reason and detail a withdrawal does not carry.
type CompactUnachievableLensWithdrawalRequest struct {
	ExpectedRevision string
	TargetIdentity   string
	SubjectHash      string
}

// WithdrawUnachievableLensAttempt removes a bound unachievable declaration
// for the exact SubjectHash, so a host that mistook a transient failure
// (issue #3442's motivating case: a host relay transport bound) for a
// deterministic one can recover the lineage instead of losing it. It
// mutates only when a declaration with that exact SubjectHash is currently
// recorded. Its behavior splits on WHY nothing matched:
//   - a stale or wrong lineage/revision/target binding is a hard, named
//     error ("does not match the current reviewing authority ..."), exactly
//     like every other capture in this file: a caller with the wrong
//     authority in hand must refresh it, never be told "done" for the wrong
//     lineage.
//   - a SubjectHash that names no currently recorded declaration, under an
//     otherwise-current binding, is the one true no-op (removed=false,
//     err=nil): a host that already withdrew, or that withdraws
//     speculatively before ever declaring, gets the same safe "nothing to
//     retract" answer instead of an error it would have to distinguish from
//     a real problem.
//
// The one case that DOES refuse despite a current binding and a matched
// SubjectHash is the slot now holding a real admitted result: that already
// answers the slot, and withdrawing the stale declaration underneath it
// would just be confusing bookkeeping, so the caller is pointed at STATUS
// instead.
func (store CompactStore) WithdrawUnachievableLensAttempt(ctx context.Context, request CompactUnachievableLensWithdrawalRequest) (removed bool, err error) {
	if ctx == nil {
		return false, errors.New("unachievable lens withdrawal context is nil") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !validSHA256(request.ExpectedRevision) || !validSHA256(request.TargetIdentity) || !validSHA256(request.SubjectHash) {
		return false, errors.New("unachievable lens withdrawal requires a valid binding") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	maintenance, err := store.acquireReadMaintenance(ctx)
	if err != nil {
		return false, err
	}
	if maintenance != nil {
		defer maintenance.Release()
	}
	deadline := time.NewTimer(maintenanceLockTimeout)
	defer deadline.Stop()
	var lock *storeLock
	for {
		lock, err = acquireStoreLock(store.lockPath)
		if !errors.Is(err, ErrConcurrentUpdate) {
			break
		}
		select {
		case <-deadline.C:
			return false, &AuthorityLockTimeoutError{Timeout: maintenanceLockTimeout}
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err != nil {
		return false, err
	}
	defer lock.release()
	record, err := store.loadCompactRecordLocked()
	if err != nil {
		return false, err
	}
	if record.HistoricalCompat {
		return false, NewLegacyReadOnlyError("review/capture-unachievable", record.State.LineageID)
	}
	state := record.State
	if state.State != StateReviewing || state.CapturePhaseRevision != request.ExpectedRevision || state.InitialSnapshot.Identity != request.TargetIdentity {
		return false, errors.New("unachievable lens withdrawal does not match the current reviewing authority; refresh the binding with gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition")
	}
	index := -1
	for candidate, existing := range state.UnachievableLensAttempts {
		if existing.SubjectHash == request.SubjectHash {
			index = candidate
			break
		}
	}
	if index < 0 {
		// Nothing currently recorded names this exact slot: either it was
		// already withdrawn, or it was never declared. Both are the same
		// safe answer -- there is nothing to retract -- so this returns
		// success without touching the authority.
		return false, nil
	}
	order := state.UnachievableLensAttempts[index].SelectedOrder
	if _, found, activeErr := state.ActiveAdmittedLensResult(order); activeErr != nil {
		return false, activeErr
	} else if found {
		return false, errors.New("this lens slot now holds a captured reviewer result; run gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition to see its admitted artifact and continue")
	}
	next := cloneCompactStateInitialAtomicStart(state)
	next.UnachievableLensAttempts = append(append([]CompactUnachievableLensAttempt{}, state.UnachievableLensAttempts[:index]...), state.UnachievableLensAttempts[index+1:]...)
	_, payload, err := makeCompactRecord(next)
	if err != nil {
		return false, err
	}
	if err := writeAtomic(store.StatePath(), payload, 0o644); err != nil {
		return false, err
	}
	return true, nil
}
