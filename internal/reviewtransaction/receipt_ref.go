// Package reviewtransaction — SDDReceiptRef (Wave 4 S5, design.md decision 1).
// SDD stores exactly this terminal pointer plus its own work-unit attempts;
// every review-validity question is one call to ValidateSDDReceiptRef. This
// file is additive: it does not yet replace RuntimeStatus's existing
// BindingRevision/Binding *ReviewBinding fields (design.md task 6.5) or
// collapse resolveReviewAuthority/resolveCompactRemediationAuthority into
// this single entry point (task 6.7) — those are a materially larger,
// separate migration across internal/sddstatus's runtime ledger, CAS/
// locking, and legacy-import subsystem (26+ existing call sites on
// ReviewBinding alone) and are deliberately deferred to their own slice
// rather than attempted here without matching verification budget. See
// apply-progress for the explicit blast-radius investigation.
package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

// SDDReceiptRef is SDD's one terminal pointer into review authority:
// "what do I ask about" (Lineage) and "which bytes did I see" (ReceiptHash).
// Any third field would be a value SDD has to keep in sync — the mirror
// this wave retires (design.md decision 1's own rationale; GateContext on
// the ref IS the re-derivation, CON-06).
type SDDReceiptRef struct {
	Lineage     string `json:"lineage"`
	ReceiptHash string `json:"receipt_hash"`
}

// strictDecodeSDDReceiptRef decodes untrusted JSON into a SDDReceiptRef, refusing
// any field beyond the exact two the schema names. A third field arriving
// over the wire is exactly the re-derivation shape decision 1 forbids: it
// must fail loudly, not be silently dropped.
func strictDecodeSDDReceiptRef(payload []byte, ref *SDDReceiptRef) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	return decoder.Decode(ref)
}

func receiptRefHash(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ValidateSDDReceiptRef is Wave 4 S5's one native validation entry point
// (design.md decision 1): "validity is one call". It resolves the
// repository through the same common-dir authority every other native
// entry point uses (SnapshotBuilder.ResolveRepositoryRoot — relative --cwd,
// linked worktrees, and symlinked common dirs all resolve identically;
// a path outside any Git repository refuses), loads the compact
// authoritative store for ref.Lineage, verifies its terminal receipt bytes
// hash to exactly ref.ReceiptHash, and evaluates the post-apply gate
// against that receipt. It re-derives nothing: the result and reason it
// returns are the provider's own verdict, stored verbatim by callers.
func ValidateSDDReceiptRef(ctx context.Context, repo string, ref SDDReceiptRef) (GateResult, string, error) {
	if err := ctx.Err(); err != nil {
		return GateInvalidated, "", err
	}
	if ref.Lineage == "" || ref.ReceiptHash == "" {
		return GateInvalidated, "receipt ref requires both lineage and receipt_hash", nil
	}
	root, err := (SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return GateInvalidated, "", fmt.Errorf("resolve receipt ref repository: %w", err)
	}
	store, err := CompactAuthoritativeStore(ctx, root, ref.Lineage)
	if err != nil {
		return GateInvalidated, "receipt ref lineage has no compact authority", nil
	}
	record, err := store.Load()
	if err != nil {
		return GateInvalidated, "receipt ref compact authority cannot be loaded", nil
	}
	payload, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		return GateInvalidated, "receipt ref terminal receipt is missing", nil
	}
	if receiptRefHash(payload) != ref.ReceiptHash {
		return GateInvalidated, "receipt ref hash does not match the stored terminal receipt", nil
	}
	receipt, err := ParseCompactReceipt(payload)
	if err != nil {
		return GateInvalidated, "receipt ref terminal receipt is invalid", nil
	}
	if receipt.LineageID != ref.Lineage {
		return GateInvalidated, "receipt ref lineage does not match the terminal receipt", nil
	}
	if record.State.State != StateApproved {
		return GateInvalidated, "receipt ref compact authority is not approved", nil
	}
	evaluation := EvaluateCompactGate(ctx, root, receipt, NativeGateRequestInput{Gate: GatePostApply, LineageID: ref.Lineage})
	return evaluation.Result, evaluation.Reason, nil
}
