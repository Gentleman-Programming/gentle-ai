package sddstatus

import "github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"

// legacy_binding_read.go is Wave 4 S5's read-only migration path (design.md
// decision 2): an existing gentle-ai.sdd-review-binding/v1 file parses via
// the unchanged parseBinding validation (review_binding.go) and projects,
// in memory only, to a reviewtransaction.SDDReceiptRef. It is never
// rewritten and never unlinked by gentle-ai — deletion is Wave 7's own,
// separate, explicitly destructive step (task 8.1), after the replacement
// path is proven.
//
// This file is additive: parseBinding's writer-half callers (bindPrepared
// Review, readLegacyBinding) are unchanged, and nothing in this wave wires
// parseLegacyBinding's projection into runtime_ledger.go or review_gate.go
// yet (design.md tasks 6.5-6.7 — deferred to a follow-up slice given their
// confirmed blast radius: 26+ existing call sites on ReviewBinding across
// the runtime ledger's CAS/locking and legacy-import subsystem).

// parseLegacyBinding reuses parseBinding's exact validation and projects
// the result to the two fields SDD's terminal pointer needs. It performs no
// file I/O of its own — like parseBinding, it operates only on bytes a
// caller already read — so it structurally cannot rewrite or unlink
// anything.
func parseLegacyBinding(payload []byte) (reviewtransaction.SDDReceiptRef, error) {
	binding, err := parseBinding(payload)
	if err != nil {
		return reviewtransaction.SDDReceiptRef{}, err
	}
	return reviewtransaction.SDDReceiptRef{Lineage: binding.Lineage, ReceiptHash: binding.ReceiptHash}, nil
}
