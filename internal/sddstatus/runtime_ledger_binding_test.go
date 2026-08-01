package sddstatus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestRuntimeBindingImportsLegacyOnceAndReplacesPopulatedBinding(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "binding-change")
	if err != nil {
		t.Fatal(err)
	}
	legacy := runtimeTestReviewBinding("binding-change", "review-legacy", 'a', 'b')
	legacyPath := writeRuntimeLegacyBinding(t, repo, legacy)
	legacyBytes, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}

	successor := runtimeTestReviewBinding("binding-change", "review-successor", 'c', 'd')
	bound, err := store.bindPreparedReview(context.Background(), BindReviewRequest{
		ExpectedBindingRevision: legacy.Revision, RequestID: "bind-successor", LineageID: successor.Lineage,
	}, func() (ReviewBinding, error) { return successor, nil })
	if err != nil {
		t.Fatal(err)
	}
	if bound.Binding == nil || bound.Binding.Revision != successor.Revision || bound.BindingRevision != successor.Revision || countRuntimeRecords(t, store.Dir) != 1 {
		t.Fatalf("imported successor status = %#v records=%d", bound, countRuntimeRecords(t, store.Dir))
	}
	record := loadRuntimeBindingHeadRecord(t, store)
	if record.Binding == nil || record.Binding.LegacyImport == nil || record.Binding.LegacyImport.Binding.Revision != legacy.Revision ||
		record.Binding.Current.Revision != successor.Revision {
		t.Fatalf("legacy import record = %#v", record)
	}
	if after, readErr := os.ReadFile(legacyPath); readErr != nil || string(after) != string(legacyBytes) {
		t.Fatalf("legacy binding was dual-written: bytes=%q err=%v", after, readErr)
	}

	// Exact replay is record-free even if the legacy compatibility artifact is
	// subsequently damaged; once imported, only the native ledger is read.
	if err := os.WriteFile(legacyPath, []byte("corrupt legacy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	replayed, err := store.bindPreparedReview(context.Background(), BindReviewRequest{
		ExpectedBindingRevision: legacy.Revision, RequestID: "bind-successor", LineageID: successor.Lineage,
	}, func() (ReviewBinding, error) {
		return ReviewBinding{}, errors.New("exact replay revalidated external authority")
	})
	if err != nil || replayed.BindingRevision != successor.Revision || countRuntimeRecords(t, store.Dir) != 1 {
		t.Fatalf("import replay = %#v err=%v records=%d", replayed, err, countRuntimeRecords(t, store.Dir))
	}

	replacement := runtimeTestReviewBinding("binding-change", "review-replacement", 'e', 'f')
	before := replayed.Revision
	_, err = store.bindPreparedReview(context.Background(), BindReviewRequest{
		ExpectedBindingRevision: legacy.Revision, RequestID: "bind-stale", LineageID: replacement.Lineage,
	}, func() (ReviewBinding, error) { return replacement, nil })
	var conflict *BindingRevisionConflictError
	if !errors.As(err, &conflict) || conflict.Expected != legacy.Revision || conflict.Current != successor.Revision {
		t.Fatalf("populated stale conflict = %T %#v", err, err)
	}
	// The error already prints the current binding revision; it must also say
	// how to use it, or a caller who only sees the string has no route back in.
	wantRetry := fmt.Sprintf("--expected-binding-revision %q", successor.Revision)
	if !strings.Contains(conflict.Error(), wantRetry) {
		t.Fatalf("binding revision conflict error = %q, want it to name %q", conflict.Error(), wantRetry)
	}
	unchanged, statusErr := store.Status()
	if statusErr != nil || unchanged.Revision != before || countRuntimeRecords(t, store.Dir) != 1 {
		t.Fatalf("stale populated bind mutated ledger: status=%#v err=%v", unchanged, statusErr)
	}

	replaced, err := store.bindPreparedReview(context.Background(), BindReviewRequest{
		ExpectedBindingRevision: successor.Revision, RequestID: "bind-replacement", LineageID: replacement.Lineage,
	}, func() (ReviewBinding, error) { return replacement, nil })
	if err != nil {
		t.Fatal(err)
	}
	if replaced.BindingRevision != replacement.Revision || countRuntimeRecords(t, store.Dir) != 2 {
		t.Fatalf("populated binding replacement = %#v records=%d", replaced, countRuntimeRecords(t, store.Dir))
	}
}

func TestRuntimeBindingConflictIsTypedAndPrePublication(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "initial-binding")
	if err != nil {
		t.Fatal(err)
	}
	binding := runtimeTestReviewBinding("initial-binding", "review-initial", '1', '2')
	_, err = store.bindPreparedReview(context.Background(), BindReviewRequest{
		// Supplying the authority revision instead of the absent binding's empty
		// revision reproduces #1524.
		ExpectedBindingRevision: binding.AuthorityRevision, RequestID: "bind-wrong-token", LineageID: binding.Lineage,
	}, func() (ReviewBinding, error) { return binding, nil })
	var conflict *BindingRevisionConflictError
	if !errors.As(err, &conflict) || !errors.Is(err, ErrBindingRevisionConflict) || conflict.Expected != binding.AuthorityRevision || conflict.Current != "" {
		t.Fatalf("initial binding conflict = %T %#v", err, err)
	}
	if countRuntimeRecords(t, store.Dir) != 0 {
		t.Fatal("initial binding conflict published an immutable record")
	}
	if _, statErr := os.Stat(filepath.Join(store.Dir, "HEAD")); !os.IsNotExist(statErr) {
		t.Fatalf("initial binding conflict published HEAD: %v", statErr)
	}

	bound, err := store.bindPreparedReview(context.Background(), BindReviewRequest{
		ExpectedBindingRevision: "", RequestID: "bind-correct-token", LineageID: binding.Lineage,
	}, func() (ReviewBinding, error) { return binding, nil })
	if err != nil || bound.BindingRevision != binding.Revision {
		t.Fatalf("initial empty CAS = %#v err=%v", bound, err)
	}
}

func TestRuntimeBindingRejectsLegacyMutationDuringOneTimeImport(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "legacy-race")
	if err != nil {
		t.Fatal(err)
	}
	legacy := runtimeTestReviewBinding("legacy-race", "review-legacy", 'a', 'b')
	legacyPath := writeRuntimeLegacyBinding(t, repo, legacy)
	successor := runtimeTestReviewBinding("legacy-race", "review-successor", 'c', 'd')
	_, err = store.bindPreparedReview(context.Background(), BindReviewRequest{
		ExpectedBindingRevision: legacy.Revision, RequestID: "bind-legacy-race", LineageID: successor.Lineage,
	}, func() (ReviewBinding, error) {
		changed := runtimeTestReviewBinding("legacy-race", "review-changed", 'e', 'f')
		payload, payloadErr := bindingBytes(changed)
		if payloadErr != nil {
			return ReviewBinding{}, payloadErr
		}
		if writeErr := os.WriteFile(legacyPath, payload, 0o600); writeErr != nil {
			return ReviewBinding{}, writeErr
		}
		return successor, nil
	})
	if err == nil || !strings.Contains(err.Error(), "changed before native import") {
		t.Fatalf("legacy import race error = %v", err)
	}
	if countRuntimeRecords(t, store.Dir) != 0 {
		t.Fatal("legacy import race published an immutable record")
	}
	if _, statErr := os.Stat(filepath.Join(store.Dir, "HEAD")); !os.IsNotExist(statErr) {
		t.Fatalf("legacy import race published HEAD: %v", statErr)
	}
}

func runtimeTestReviewBinding(change, lineage string, authority, receipt byte) ReviewBinding {
	binding := ReviewBinding{
		Schema: reviewBindingSchema, Change: change, Lineage: lineage,
		AuthorityRevision: runtimeTestHash(authority), ReceiptHash: runtimeTestHash(receipt),
		GateContext: reviewtransaction.GateContext{
			Gate: reviewtransaction.GatePostApply, LineageID: lineage,
			StoreRevision: runtimeTestHash(authority), Generation: 1,
		},
	}
	binding.Revision = bindingDigest(binding)
	return binding
}

func writeRuntimeLegacyBinding(t *testing.T, repo string, binding ReviewBinding) string {
	t.Helper()
	probe, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "binding-probe")
	if err != nil {
		t.Fatal(err)
	}
	path := bindingPath(probe, binding.Change)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	payload, err := bindingBytes(binding)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func loadRuntimeBindingHeadRecord(t *testing.T, store RuntimeStore) runtimeRecord {
	t.Helper()
	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.loadRecord(status.Revision)
	if err != nil {
		t.Fatal(err)
	}
	return record
}
