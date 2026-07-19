package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

// approveTrackedGoChangeWithWarningFinding persists an approved compact
// authority over a tracked Go modification whose completed review froze one
// non-severe finding, so the frozen findings ledger is non-empty without
// requiring a correction transaction.
func approveTrackedGoChangeWithWarningFinding(t *testing.T, repo, lineage string) reviewtransaction.CompactState {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "feature.go"), []byte("package feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "feature.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "add feature base")
	if err := os.WriteFile(filepath.Join(repo, "feature.go"), []byte("package feature\n\nfunc Feature() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
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
	lenses := []string{reviewtransaction.LensReliability}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1, Snapshot: snapshot,
		PolicyHash: "sha256:" + hexDigest("policy"), RiskLevel: risk, SelectedLenses: lenses, OriginalChangedLines: &lines,
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
	if err := state.CompleteVerification([]byte("independent verification passed\n"), true); err != nil {
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

func hexDigest(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:])
}

func canonicalLedgerHash(t *testing.T, findings []reviewtransaction.Finding) string {
	t.Helper()
	ledger, err := reviewtransaction.CanonicalLedger(findings)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(ledger)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestFacadeScopeChangedDiscoveryContextBindsFrozenFindingsLedger(t *testing.T) {
	repo := initReviewCLIRepo(t)
	state := approveTrackedGoChangeWithWarningFinding(t, repo, "facade-ledger-scope")
	if len(state.Findings) == 0 {
		t.Fatalf("fixture must freeze findings: %#v", state)
	}
	want := canonicalLedgerHash(t, state.Findings)
	if want == reviewtransaction.EmptyFixDeltaHash {
		t.Fatalf("frozen findings ledger collides with empty-input hash %q", want)
	}
	if err := os.WriteFile(filepath.Join(repo, "feature.go"), []byte("package feature\n\nfunc Feature() int { return 2 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := discoverCompactFacadeGateReview(context.Background(), repo, "", reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePostApply})
	var discovery *ReviewReceiptDiscoveryError
	if !errors.As(err, &discovery) || discovery.Kind != ReviewReceiptScopeChanged || discovery.Context == nil {
		t.Fatalf("drifted target discovery = %#v, %v", discovery, err)
	}
	if discovery.Context.LedgerHash != want {
		t.Fatalf("scope-changed discovery context ledger_hash = %q, want frozen findings ledger %q", discovery.Context.LedgerHash, want)
	}
}
