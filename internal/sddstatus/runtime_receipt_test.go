//go:build legacy_compact_receipt

package sddstatus

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// This file is Wave 4 S5b (design.md decision 1, task 6.5): RuntimeStatus's
// terminal receipt pointer. recordPreparedReceipt mirrors bindPreparedReview's
// exact CAS/locking and one-time legacy-import idempotency semantics
// (runtime_ledger_binding_test.go is this file's model), but works with the
// two-field reviewtransaction.SDDReceiptRef instead of the richer
// ReviewBinding, and imports the legacy compatibility artifact through
// legacy_binding_read.go's parseLegacyBinding projection rather than the
// full ReviewBinding shape.
//
// This is additive alongside the existing Binding/BindingRevision fields —
// see apply-progress for why removing those fields outright is deferred: it
// touches validateRuntimeRecordShape's per-operation state machine,
// runtime_compact.go's remediation-successor CAS, and review_binding.go's
// writer functions, a materially larger and separate migration.

func runtimeTestReceipt(lineage, receiptHashChar string) reviewtransaction.SDDReceiptRef {
	return reviewtransaction.SDDReceiptRef{Lineage: lineage, ReceiptHash: runtimeTestHash(receiptHashChar[0])}
}

func TestRuntimeReceiptImportsLegacyOnceAndReplacesPopulatedReceipt(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "receipt-change")
	if err != nil {
		t.Fatal(err)
	}
	legacyBinding := runtimeTestReviewBinding("receipt-change", "review-legacy", 'a', 'b')
	legacyPath := writeRuntimeLegacyBinding(t, repo, legacyBinding)
	legacyBytes, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	legacyReceipt := reviewtransaction.SDDReceiptRef{Lineage: legacyBinding.Lineage, ReceiptHash: legacyBinding.ReceiptHash}
	legacyRevision := receiptRefDigest(legacyReceipt)

	successor := runtimeTestReceipt("review-successor", "c")
	bound, err := store.recordPreparedReceipt(context.Background(), RecordReceiptRequest{
		ExpectedReceiptRevision: legacyRevision, RequestID: "receipt-successor", Lineage: successor.Lineage,
	}, func() (reviewtransaction.SDDReceiptRef, error) { return successor, nil })
	if err != nil {
		t.Fatal(err)
	}
	if bound.Receipt == nil || *bound.Receipt != successor || countRuntimeRecords(t, store.Dir) != 1 {
		t.Fatalf("imported successor status = %#v records=%d", bound, countRuntimeRecords(t, store.Dir))
	}

	if after, readErr := os.ReadFile(legacyPath); readErr != nil || string(after) != string(legacyBytes) {
		t.Fatalf("legacy binding was dual-written: bytes=%q err=%v", after, readErr)
	}

	// Exact replay is record-free even if the legacy compatibility artifact is
	// subsequently damaged; once imported, only the native ledger is read.
	if err := os.WriteFile(legacyPath, []byte("corrupt legacy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	replayed, err := store.recordPreparedReceipt(context.Background(), RecordReceiptRequest{
		ExpectedReceiptRevision: legacyRevision, RequestID: "receipt-successor", Lineage: successor.Lineage,
	}, func() (reviewtransaction.SDDReceiptRef, error) {
		return reviewtransaction.SDDReceiptRef{}, errors.New("exact replay revalidated external authority")
	})
	if err != nil || replayed.Receipt == nil || *replayed.Receipt != successor || countRuntimeRecords(t, store.Dir) != 1 {
		t.Fatalf("import replay = %#v err=%v records=%d", replayed, err, countRuntimeRecords(t, store.Dir))
	}

	replacement := runtimeTestReceipt("review-replacement", "d")
	successorRevision := receiptRefDigest(successor)
	_, err = store.recordPreparedReceipt(context.Background(), RecordReceiptRequest{
		ExpectedReceiptRevision: legacyRevision, RequestID: "receipt-stale", Lineage: replacement.Lineage,
	}, func() (reviewtransaction.SDDReceiptRef, error) { return replacement, nil })
	var conflict *ReceiptRevisionConflictError
	if !errors.As(err, &conflict) || conflict.Expected != legacyRevision || conflict.Current != successorRevision {
		t.Fatalf("populated stale conflict = %T %#v", err, err)
	}

	replaced, err := store.recordPreparedReceipt(context.Background(), RecordReceiptRequest{
		ExpectedReceiptRevision: successorRevision, RequestID: "receipt-replacement", Lineage: replacement.Lineage,
	}, func() (reviewtransaction.SDDReceiptRef, error) { return replacement, nil })
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Receipt == nil || *replaced.Receipt != replacement || countRuntimeRecords(t, store.Dir) != 2 {
		t.Fatalf("populated receipt replacement = %#v records=%d", replaced, countRuntimeRecords(t, store.Dir))
	}
}

func TestRuntimeReceiptFirstImportRequiresEmptyExpectedRevisionWithoutLegacy(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "receipt-fresh")
	if err != nil {
		t.Fatal(err)
	}
	receipt := runtimeTestReceipt("review-fresh", "e")
	status, err := store.recordPreparedReceipt(context.Background(), RecordReceiptRequest{
		RequestID: "receipt-fresh-request", Lineage: receipt.Lineage,
	}, func() (reviewtransaction.SDDReceiptRef, error) { return receipt, nil })
	if err != nil {
		t.Fatal(err)
	}
	if status.Receipt == nil || *status.Receipt != receipt {
		t.Fatalf("fresh receipt status = %#v", status)
	}
}
