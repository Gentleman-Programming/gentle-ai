package reviewtransaction

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

// lockNativeReceipt resolves only the historical v1 receipt authority. Compact
// review state burns on its terminal event and no longer publishes receipts.
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
	maintenance, err := acquireMaintenanceLock(ctx, compactMaintenanceLockPath(base), maintenanceExclusive)
	if err != nil {
		return RARNativeReceiptAuthority{}, VerificationSubject{}, func() {}, err
	}
	release := func() { _ = maintenance.Release() }
	fail := func(err error) (RARNativeReceiptAuthority, VerificationSubject, func(), error) {
		release()
		return RARNativeReceiptAuthority{}, VerificationSubject{}, func() {}, err
	}
	if err := repository.validateIdentity(ctx); err != nil {
		return fail(err)
	}
	historical, err := AuthoritativeStore(ctx, repository.identity.RepositoryRoot, lineageID)
	if err != nil {
		return fail(err)
	}
	return repository.loadHistoricalNativeReceipt(ctx, historical, expectedReceiptRef, release)
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
	fail := func(err error) (RARNativeReceiptAuthority, VerificationSubject, func(), error) {
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
	receiptPath := filepath.Join(store.Dir, "artifacts", "receipt.json")
	payload, err := os.ReadFile(receiptPath)
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
	receiptRef, err := HashArtifact(receiptPath)
	if err != nil {
		return fail(err)
	}
	if receiptRef != expectedReceiptRef {
		return fail(errors.New("historical review receipt does not match the expected ref"))
	}
	subject, err := VerificationSubjectFromSnapshot(record.Transaction.Snapshot)
	if err != nil {
		return fail(fmt.Errorf("historical review snapshot cannot satisfy the current exact subject contract: %w", err))
	}
	if receipt.FinalCandidateTree != subject.CandidateTree || receipt.PolicyHash != record.Transaction.PolicyHash {
		return fail(errors.New("historical receipt does not bind its live review subject and policy"))
	}
	native := RARNativeReceiptAuthority{
		Schema: RARNativeReceiptSchema, Version: RARReceiptHistoricalV1,
		AuthorityRevision: revision, ReceiptRef: receiptRef, Historical: &receipt,
	}
	if err := native.Validate(); err != nil {
		return fail(err)
	}
	return native, subject, release, nil
}
