package sddstatus

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// review_gate_receipt_test.go is Wave 4 S5c' (design.md's decision-1
// amendment, 2026-08-03): the archive-gate evaluation path (status.go's two
// explicit-governance branches plus resolveCompactRemediationAuthority)
// reroutes from validateBoundReview/validateRuntimeBoundCandidate's stored
// reviewtransaction.GateContext comparison onto a fresh
// reviewtransaction.ValidateSDDReceiptRef re-derivation. Binding/
// BindingRevision, `review bind-sdd`, and the remediation-successor CAS stay
// untouched (Wave 7 territory) — only the archive-gate's SOURCE of validity
// changes, not who populates the underlying binding data.

// TestResolveGoverningReceiptRefRequiresAParsedNativeBinding ports
// TestBindingExistsRequiresAParsedNativeBinding (deleted in S7's re-scoped
// 8.1 alongside bindingExists itself, now genuinely orphaned by this
// slice's rerouting) onto resolveGoverningReceiptRef, which reuses the same
// underlying native-store read and must preserve the same two properties:
// an attempt-only runtime HEAD (no binding at all) is not treated as
// governance, and a corrupt native runtime HEAD fails closed with an error
// rather than silently reporting absence.
func TestResolveGoverningReceiptRefRequiresAParsedNativeBinding(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "attempts-only")
	if _, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "begin-attempts-only", WorkUnit: "apply",
		EvidenceGoal: "prove attempts do not imply review authority", MaxAttempts: 2, MaxChangedLines: 20,
	}); err != nil {
		t.Fatal(err)
	}
	ref, err := resolveGoverningReceiptRef(context.Background(), repo, "attempts-only")
	if err != nil {
		t.Fatal(err)
	}
	if ref != nil {
		t.Fatalf("attempt-only runtime HEAD was treated as an explicit review binding: %#v", ref)
	}

	if err := os.WriteFile(filepath.Join(store.Dir, "HEAD"), []byte("corrupt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveGoverningReceiptRef(context.Background(), repo, "attempts-only"); err == nil {
		t.Fatal("corrupt native runtime HEAD was accepted as a review binding")
	}
}

func TestResolveGoverningReceiptRefPresenceAndAbsence(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")

	absent, err := resolveGoverningReceiptRef(context.Background(), root, "thin")
	if err != nil || absent != nil {
		t.Fatalf("resolveGoverningReceiptRef() before bind = %#v, %v, want nil, nil", absent, err)
	}

	binding, err := BindApprovedReview(context.Background(), root, "thin", "approved-thin", "")
	if err != nil {
		t.Fatal(err)
	}
	present, err := resolveGoverningReceiptRef(context.Background(), root, "thin")
	if err != nil || present == nil || present.Lineage != binding.Lineage || present.ReceiptHash != binding.ReceiptHash {
		t.Fatalf("resolveGoverningReceiptRef() after bind = %#v, %v, want {Lineage:%q ReceiptHash:%q}", present, err, binding.Lineage, binding.ReceiptHash)
	}
}

// TestBoundReviewArchiveGateIgnoresStaleGateContextViaReDerivation proves the
// re-derivation this slice wires: a governing receipt whose persisted
// GateContext no longer matches the live post-apply evaluation (the exact
// shape validateBoundReview's boundGateContextMatches used to fail closed on)
// is now genuinely irrelevant, because ValidateSDDReceiptRef never reads or
// compares GateContext at all — the receipt hash plus a fresh gate
// evaluation is the whole re-derivation (design.md decision 1: "GateContext
// on the ref IS the re-derivation").
func TestBoundReviewArchiveGateIgnoresStaleGateContextViaReDerivation(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")
	write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("a"), "pass"))

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), root, "approved-thin")
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	receiptPayload, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := reviewtransaction.ParseCompactReceipt(receiptPayload)
	if err != nil {
		t.Fatal(err)
	}
	gate := reviewtransaction.EvaluateCompactGate(context.Background(), root, receipt, reviewtransaction.NativeGateRequestInput{
		Gate: reviewtransaction.GatePostApply, LineageID: "approved-thin",
	})
	if gate.Result != reviewtransaction.GateAllow {
		t.Fatalf("fixture live gate = %#v, want allow", gate)
	}

	binding := ReviewBinding{
		Schema: reviewBindingSchema, Change: "thin", Lineage: "approved-thin",
		AuthorityRevision: record.Revision, ReceiptHash: bindingHash(receiptPayload),
		GateContext: gate.Context,
	}
	// Corrupt only the persisted GateContext's LedgerHash — a well-formed but
	// wrong value, deliberately not reviewtransaction.EmptyFixDeltaHash, so
	// boundGateContextMatches's own empty-hash exemption cannot mask this.
	binding.GateContext.LedgerHash = "sha256:" + strings.Repeat("9", 64)
	if binding.GateContext.LedgerHash == gate.Context.LedgerHash {
		t.Fatal("fixture bug: corrupted LedgerHash accidentally matches the live value")
	}
	binding.Revision = bindingDigest(binding)
	writeRuntimeLegacyBinding(t, root, binding)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateAllow {
		t.Fatalf("ReviewGate = %#v, want allow via re-derivation despite stale persisted GateContext", status.ReviewGate)
	}
	if status.Dependencies.Archive == DependencyBlocked && status.NextRecommended == "resolve-review" {
		t.Fatalf("status still routed through the retired stored-GateContext-comparison rejection: %#v", status)
	}
}
