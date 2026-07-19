package sddstatus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
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
	revision, err = store.Replace(revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("verification passed\n"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-verification", state); err != nil {
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

func TestBoundReviewBindsLedgerAcceptsLegacyEmptyAndRejectsForgedHash(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	state := writeApprovedCompactAuthorityWithWarningFinding(t, root, changeRoot, "approved-ledger")
	want := canonicalBindingLedgerHash(t, state.Findings)
	if want == reviewtransaction.EmptyFixDeltaHash {
		t.Fatalf("frozen findings ledger collides with empty-input hash %q", want)
	}

	binding, err := BindApprovedReview(context.Background(), root, "thin", "approved-ledger", "")
	if err != nil {
		t.Fatal(err)
	}
	if binding.GateContext.LedgerHash != want {
		t.Fatalf("fresh binding ledger_hash = %q, want frozen findings ledger %q", binding.GateContext.LedgerHash, want)
	}
	if _, _, err := validateBoundReview(context.Background(), root, "thin"); err != nil {
		t.Fatalf("fresh binding validation: %v", err)
	}

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), root, "approved-ledger")
	if err != nil {
		t.Fatal(err)
	}

	// A binding persisted before gate contexts bound the frozen findings
	// ledger recorded the empty-input hash; it must keep validating against
	// the now-populated live context.
	legacy := binding
	legacy.GateContext.LedgerHash = reviewtransaction.EmptyFixDeltaHash
	legacy.Revision = bindingDigest(legacy)
	payload, err := bindingBytes(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bindingPath(store, "thin"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := validateBoundReview(context.Background(), root, "thin"); err != nil {
		t.Fatalf("legacy empty-ledger binding rejected: %v", err)
	}

	// Any other ledger hash is a real divergence and must stay rejected.
	forged := binding
	forged.GateContext.LedgerHash = "sha256:" + strings.Repeat("ab", 32)
	forged.Revision = bindingDigest(forged)
	payload, err = bindingBytes(forged)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bindingPath(store, "thin"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := validateBoundReview(context.Background(), root, "thin"); err == nil {
		t.Fatal("forged ledger hash binding was accepted")
	}
}
