package sddstatus

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestEditGrantLockContentionPreservesTheNonMutationProof pins the 1861
// boundary for the surviving edit-grant store. Grant acquires its lock
// strictly before commitRecordLocked, so a refusal at acquisition wrote
// nothing. The mapped sentinel must therefore keep the underlying contention
// error in its chain instead of flattening it into prose: without that proof a
// caller can only report an unknown mutation outcome, which tells it retry is
// unsafe for an operation that provably never started.
func TestEditGrantLockContentionPreservesTheNonMutationProof(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "contention-change")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ensureDirectories(); err != nil {
		t.Fatal(err)
	}

	held, err := reviewtransaction.AcquireAuthorityFileLock(filepath.Join(store.Dir, "LOCK"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Release() }()

	lock, err := store.acquireLock()
	if err == nil {
		_ = lock.Release()
		t.Fatal("SDD runtime ledger acquired a lock another holder owns")
	}
	if !errors.Is(err, ErrRuntimeConcurrentUpdate) {
		t.Fatalf("contended runtime ledger acquisition = %v, want ErrRuntimeConcurrentUpdate", err)
	}
	if !errors.Is(err, reviewtransaction.ErrStoreLockContended) {
		t.Fatalf("contended runtime ledger acquisition lost the non-mutation proof: %v", err)
	}
}

// TestEditGrantMutationContentionIsNotDoubleWrapped keeps the mapped
// refusal readable. mutate carried a second re-wrap of the already-mapped
// error. It was unreachable only because the first mapping flattened
// ErrConcurrentUpdate out of the chain; the moment that chain is preserved the
// re-wrap fires and duplicates the sentinel prefix, so it is removed with the
// flattening rather than after it.
func TestEditGrantMutationContentionIsNotDoubleWrapped(t *testing.T) {
	store, request := grantTestStore(t)
	if err := store.ensureDirectories(); err != nil {
		t.Fatal(err)
	}
	held, err := reviewtransaction.AcquireAuthorityFileLock(filepath.Join(store.Dir, "LOCK"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Release() }()

	_, mutateErr := store.Grant(context.Background(), request)
	if !errors.Is(mutateErr, ErrRuntimeConcurrentUpdate) || !errors.Is(mutateErr, reviewtransaction.ErrStoreLockContended) {
		t.Fatalf("contended runtime ledger mutation = %v, want a mapped contention refusal with its proof intact", mutateErr)
	}
	status, err := store.Status()
	if err != nil || status.Revision != "" || len(status.GrantedRoots) != 0 || countRuntimeRecords(t, store.Dir) != 0 {
		t.Fatalf("contended grant published authority: %#v, err=%v", status, err)
	}
	prefix := ErrRuntimeConcurrentUpdate.Error()
	if strings.Count(mutateErr.Error(), prefix) != 1 {
		t.Fatalf("contended runtime ledger mutation duplicated its sentinel prefix: %v", mutateErr)
	}
}
