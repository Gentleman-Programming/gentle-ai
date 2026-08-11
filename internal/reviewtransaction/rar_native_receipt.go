package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

// lockNativeReceipt prefers compact v2 authority. A present-but-invalid v2
// lineage never falls back to historical v1 authority with the same name.
func (repository *RARAuthorityRepository) lockNativeReceipt(
	ctx context.Context,
	lineageID string,
	expectedReceiptRef string,
) (
	RARNativeReceiptAuthority,
	VerificationSubject,
	func(),
	error,
) {
	base, _, err := reviewAuthorityRoot(ctx, repository.identity.RepositoryRoot)
	if err != nil {
		return RARNativeReceiptAuthority{}, VerificationSubject{}, func() {}, err
	}
	if err := repository.validateIdentity(ctx); err != nil {
		return RARNativeReceiptAuthority{}, VerificationSubject{}, func() {}, err
	}
	maintenance, err := acquireMaintenanceLock(
		ctx,
		compactMaintenanceLockPath(base),
		maintenanceExclusive,
	)
	if err != nil {
		return RARNativeReceiptAuthority{}, VerificationSubject{}, func() {}, err
	}
	release := func() { _ = maintenance.Release() }
	fail := func(err error) (
		RARNativeReceiptAuthority,
		VerificationSubject,
		func(),
		error,
	) {
		release()
		return RARNativeReceiptAuthority{}, VerificationSubject{}, func() {}, err
	}
	if err := repository.validateIdentity(ctx); err != nil {
		return fail(err)
	}

	compact, err := CompactAuthoritativeStore(ctx, repository.identity.RepositoryRoot, lineageID)
	if err != nil {
		return fail(err)
	}
	if _, statErr := os.Lstat(compact.StatePath()); statErr == nil {
		return repository.loadCompactNativeReceipt(
			ctx,
			compact,
			expectedReceiptRef,
			release,
		)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fail(statErr)
	} else if _, statErr := os.Lstat(compact.Dir); statErr == nil {
		return fail(errors.New("compact native receipt authority is incomplete"))
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fail(statErr)
	}

	historical, err := AuthoritativeStore(ctx, repository.identity.RepositoryRoot, lineageID)
	if err != nil {
		return fail(err)
	}
	return repository.loadHistoricalNativeReceipt(
		ctx,
		historical,
		expectedReceiptRef,
		release,
	)
}

func (repository *RARAuthorityRepository) loadCompactNativeReceipt(
	ctx context.Context,
	store CompactStore,
	expectedReceiptRef string,
	release func(),
) (
	RARNativeReceiptAuthority,
	VerificationSubject,
	func(),
	error,
) {
	fail := func(err error) (
		RARNativeReceiptAuthority,
		VerificationSubject,
		func(),
		error,
	) {
		release()
		return RARNativeReceiptAuthority{}, VerificationSubject{}, func() {}, err
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	record, err := store.loadCompactRecordLocked()
	if err != nil {
		return fail(err)
	}
	if record.State.State != StateApproved {
		return fail(errors.New("RAR authority requires approved compact review authority"))
	}
	superseded, err := compactLineageSupersededUnderMaintenance(
		ctx,
		repository.identity.RepositoryRoot,
		record.State.LineageID,
	)
	if err != nil {
		return fail(err)
	}
	if superseded {
		return fail(errors.New("approved compact review authority has been superseded"))
	}
	observation, err := inspectCompactTargetReceipt(store, record.State)
	if err != nil {
		return fail(err)
	}
	if !observation.published ||
		!bytes.Equal(observation.artifact.content, observation.artifact.canonical) ||
		observation.artifact.identity != expectedReceiptRef {
		return fail(errors.New("compact review receipt is absent, non-canonical, or does not match the expected ref"))
	}
	receipt, err := ParseCompactReceipt(observation.artifact.content)
	if err != nil {
		return fail(err)
	}
	subject, err := VerificationSubjectFromSnapshot(record.State.CurrentSnapshot)
	if err != nil {
		return fail(err)
	}
	if receipt.FinalCandidateTree != subject.CandidateTree ||
		receipt.PolicyHash != record.State.PolicyHash {
		return fail(errors.New("compact receipt does not bind its live review subject and policy"))
	}
	native := RARNativeReceiptAuthority{
		Schema:            RARNativeReceiptSchema,
		Version:           RARReceiptCompactV2,
		AuthorityRevision: record.Revision,
		ReceiptRef:        observation.artifact.identity,
		Compact:           &receipt,
	}
	if err := native.Validate(); err != nil {
		return fail(err)
	}
	return native, subject, release, nil
}

func (repository *RARAuthorityRepository) loadHistoricalNativeReceipt(
	ctx context.Context,
	store Store,
	expectedReceiptRef string,
	release func(),
) (
	RARNativeReceiptAuthority,
	VerificationSubject,
	func(),
	error,
) {
	fail := func(err error) (
		RARNativeReceiptAuthority,
		VerificationSubject,
		func(),
		error,
	) {
		release()
		return RARNativeReceiptAuthority{}, VerificationSubject{}, func() {}, err
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	record, revision, err := store.Load()
	if err != nil {
		return fail(err)
	}
	if record.Transaction.State != StateApproved {
		return fail(errors.New("RAR authority requires approved historical review authority"))
	}
	receiptRef, err := inspectLegacyTargetReceipt(store, record.Transaction)
	if err != nil {
		return fail(err)
	}
	if receiptRef != expectedReceiptRef {
		return fail(errors.New("historical review receipt does not match the expected ref"))
	}
	payload, err := os.ReadFile(filepath.Join(store.Dir, "artifacts", "receipt.json"))
	if err != nil {
		return fail(err)
	}
	receipt, err := ParseReceipt(payload)
	if err != nil {
		return fail(err)
	}
	expected, err := record.Transaction.Receipt()
	if err != nil || !reflect.DeepEqual(receipt, expected) {
		return fail(errors.New("historical review receipt changed from native authority"))
	}
	subject, err := VerificationSubjectFromSnapshot(record.Transaction.Snapshot)
	if err != nil {
		return fail(fmt.Errorf("historical review snapshot cannot satisfy the current exact subject contract: %w", err))
	}
	if receipt.FinalCandidateTree != subject.CandidateTree ||
		receipt.PolicyHash != record.Transaction.PolicyHash {
		return fail(errors.New("historical receipt does not bind its live review subject and policy"))
	}
	native := RARNativeReceiptAuthority{
		Schema:            RARNativeReceiptSchema,
		Version:           RARReceiptHistoricalV1,
		AuthorityRevision: revision,
		ReceiptRef:        receiptRef,
		Historical:        &receipt,
	}
	if err := native.Validate(); err != nil {
		return fail(err)
	}
	return native, subject, release, nil
}

func compactLineageSupersededUnderMaintenance(
	ctx context.Context,
	repo string,
	lineageID string,
) (bool, error) {
	stores, err := DiscoverCompactStores(ctx, repo)
	if err != nil {
		return false, err
	}
	for _, store := range stores {
		record, loadErr := store.loadCompactRecordLocked()
		if loadErr != nil {
			// Scoped like every other authority walk (#2743): the probe
			// answers whether THIS lineage has a recovery successor, so an
			// unreadable foreign record is absent from the graph, not a
			// receipt-derivation failure. The caller already holds the
			// maintenance lock, hence the locked read instead of
			// scanCompactAuthority; operational failures still propagate.
			if IsCompactAuthorityOperationalFailure(loadErr) {
				return false, loadErr
			}
			continue
		}
		if record.State.Recovery != nil &&
			record.State.Recovery.PredecessorLineageID == lineageID {
			return true, nil
		}
	}
	return false, nil
}
