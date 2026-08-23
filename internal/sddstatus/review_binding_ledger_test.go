//go:build legacy_compact_receipt

package sddstatus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// writeApprovedCompactAuthorityWithWarningFinding persists an approved compact
// authority whose completed review froze one non-severe finding, so the frozen
// findings ledger is non-empty without a correction transaction.
func writeApprovedCompactAuthorityWithWarningFinding(t *testing.T, repo, changeRoot, lineage string) reviewtransaction.CompactState {
	t.Helper()
	write(t, filepath.Join(repo, "feature.go"), "package feature\n")
	runSDDStatusGit(t, repo, "init", "-q")
	runSDDStatusGit(t, repo, "config", "user.email", "status@example.com")
	runSDDStatusGit(t, repo, "config", "user.name", "Status Test")
	runSDDStatusGit(t, repo, "add", ".")
	runSDDStatusGit(t, repo, "commit", "-qm", "base")
	write(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Done\n# approved compact scope\n")
	write(t, filepath.Join(repo, "feature.go"), "package feature\n\nfunc Feature() int { return 1 }\n")
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if risk != reviewtransaction.RiskMedium {
		t.Fatalf("tracked Go change risk = %q, want medium", risk)
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1, Snapshot: snapshot,
		PolicyHash: shaID("c"), RiskLevel: risk, SelectedLenses: []string{reviewtransaction.LensReliability}, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	finding := reviewtransaction.Finding{
		ID: "R3-001", Lens: "reliability", Location: "feature.go:3", Severity: "WARNING",
		Claim: "return value lacks a covering assertion", ProofRefs: []string{"feature.go:3 has no differential test"},
	}
	results := []reviewtransaction.LensResult{{Lens: reviewtransaction.LensReliability, Findings: []reviewtransaction.Finding{finding}, Evidence: []string{"review completed"}}}
	if err := state.CompleteReview(reviewtransaction.CompactReviewInput{LensResults: results, Classifications: []reviewtransaction.FindingEvidence{}, RefuterOutcomes: []reviewtransaction.EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	if err := state.CloseCleanReviewOnLastEvent(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-review", state); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	return state
}

func canonicalBindingLedgerHash(t *testing.T, findings []reviewtransaction.Finding) string {
	t.Helper()
	ledger, err := reviewtransaction.CanonicalLedger(findings)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(ledger)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// validateCurrentBoundCandidate and the ledger-forgery test it served are
// gone with their only production caller. Both existed to exercise
// validateRuntimeBoundCandidate, which ran exclusively inside the
// bound-passing-finish gate that #1993 removed, so the check they proved had
// already stopped running in production the moment that gate did. Keeping a
// green test over deleted code would assert an enforcement that no longer
// exists. Delivery still re-derives the receipt from the candidate actually
// being delivered, which is where this property is enforced now.
