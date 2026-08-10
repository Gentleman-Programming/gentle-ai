package reviewtransaction

// Wave 5 (Gate Cutover), Slice 4: RED/unit coverage for ProjectLegacyAuthority
// and EvaluateLegacyGate (design decision 1, legacy_projection.go). Testing
// Strategy row "projectLegacyAuthority purity" — a real Git repo and a real
// terminal legacy chain (zero-finding path, mirroring
// internal/cli/review_test.go's newLegacyCLIFixture), never a synthetic
// hash()/tree() transaction: FreezeCandidateIdentity needs real tree objects
// to diff (shadowChangedPathsModesDigest), which the package's usual
// newTestTransaction fixture deliberately does not provide.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// legacyApprovedChainFixture builds a real, terminal (approved), zero-finding
// legacy v1 chain over a real Git repo and writes its receipt to disk,
// mirroring internal/cli/review_test.go's newLegacyCLIFixture exactly (same
// operation sequence) but using only reviewtransaction-package primitives —
// legacy_projection.go must never depend on package cli.
func legacyApprovedChainFixture(t *testing.T, repo, lineage string) (Store, ValidatedChain, string) {
	t.Helper()
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.md")
	ledgerPath := filepath.Join(dir, "ledger.json")
	evidencePath := filepath.Join(dir, "evidence.txt")
	if err := os.WriteFile(policyPath, []byte("legacy bounded policy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger, err := CanonicalLedger([]Finding{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ledgerPath, ledger, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(evidencePath, []byte("legacy verification passed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	policyHash, err := HashArtifact(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	ledgerHash, err := HashLedgerArtifact(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	evidenceHash, err := HashArtifact(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := NewTransaction(Start{LineageID: lineage, Mode: ModeOrdinary4R, Generation: 1, Snapshot: snapshot, PolicyHash: policyHash})
	if err != nil {
		t.Fatal(err)
	}
	store, err := AuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	head, err := store.Append("", Record{Operation: "review/start", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.FreezeFindings([]Finding{}, ledger, ledgerHash); err != nil {
		t.Fatal(err)
	}
	head, err = store.Append(head, Record{Operation: "review/freeze-findings", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ClassifyEvidence([]FindingEvidence{}); err != nil {
		t.Fatal(err)
	}
	head, err = store.Append(head, Record{Operation: "review/classify-evidence", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.BeginFinalVerification(); err != nil {
		t.Fatal(err)
	}
	head, err = store.Append(head, Record{Operation: "review/begin-final-verification", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.CompleteFinalVerification(evidenceHash, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(head, Record{Operation: "review/complete-final-verification", Transaction: *tx}); err != nil {
		t.Fatal(err)
	}
	receipt, err := tx.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(store.Dir, "artifacts", "receipt.json")
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteReceiptAtomic(receiptPath, receipt); err != nil {
		t.Fatal(err)
	}
	chain, err := store.LoadChain()
	if err != nil {
		t.Fatal(err)
	}
	return store, chain, receiptPath
}

// TestProjectLegacyAuthorityProjectsReceiptIntoCandidateIdentity is the
// purity RED/GREEN test the design's Testing Strategy row names: a real
// terminal legacy chain projects into a CandidateIdentity whose BaseTree,
// CandidateTree, and PolicyHash come straight from the immutable receipt —
// never fabricated — and whose ChangedPathsModesDigest is genuinely
// recomputed (non-empty), proving this is a real tree-to-tree diff, not a
// copied placeholder. Read-only: neither the chain's events nor the receipt
// bytes on disk change.
func TestProjectLegacyAuthorityProjectsReceiptIntoCandidateIdentity(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, chain, receiptPath := legacyApprovedChainFixture(t, repo, "legacy-projection-purity")
	before, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}

	identity, receiptRef, err := ProjectLegacyAuthority(context.Background(), repo, chain, receiptPath)
	if err != nil {
		t.Fatalf("ProjectLegacyAuthority: %v", err)
	}
	tx := chain.Records[len(chain.Records)-1].Transaction
	if identity.BaseTree != tx.BaseTree || identity.CandidateTree != tx.FinalCandidateTree || identity.PolicyHash != tx.PolicyHash {
		t.Fatalf("projected identity = %#v, want base/candidate/policy from the terminal transaction %#v", identity, tx)
	}
	if identity.ChangedPathsModesDigest == "" {
		t.Fatal("projected identity has an empty ChangedPathsModesDigest — the tree-to-tree diff never ran")
	}
	if receiptRef.LineageID != tx.LineageID || receiptRef.AuthorityRevision == "" {
		t.Fatalf("receipt ref = %#v", receiptRef)
	}

	after, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("ProjectLegacyAuthority is not read-only: receipt bytes changed on disk")
	}
}

// TestProjectLegacyAuthorityRefusesReceiptChainMismatch pins the tamper/
// corruption guard: a receipt whose bytes no longer match its own chain's
// terminal transaction (BaseTree/CandidateTree/LineageID) must never be
// projected into a trusted CandidateIdentity.
func TestProjectLegacyAuthorityRefusesReceiptChainMismatch(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, chain, receiptPath := legacyApprovedChainFixture(t, repo, "legacy-projection-mismatch")
	payload, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := ParseReceipt(payload)
	if err != nil {
		t.Fatal(err)
	}
	// LineageID is a pure string comparison, unlike BaseTree/FinalCandidateTree
	// (which FreezeCandidateIdentity's own git resolution would separately
	// reject if pointed at a nonexistent tree object) — tampering it isolates
	// the chain-consistency guard itself from that downstream git failure.
	receipt.LineageID = receipt.LineageID + "-tampered"
	tampered, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(receiptPath, tampered, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := ProjectLegacyAuthority(context.Background(), repo, chain, receiptPath); err == nil {
		t.Fatal("ProjectLegacyAuthority allowed a receipt that does not match its chain's terminal transaction")
	}
}

// TestEvaluateLegacyGateAllowsExactAndDeniesChanged is the outcome-parity
// smoke test at the reviewtransaction layer: an exact live candidate allows,
// and a candidate that diverges since the frozen receipt denies — the same
// two outcomes TestLegacyFunnelCharacterization_RunFacadeLegacyValidateNegotiated
// (internal/cli) pins end-to-end through RunReviewFacadeValidate.
func TestEvaluateLegacyGateAllowsExactAndDeniesChanged(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, chain, receiptPath := legacyApprovedChainFixture(t, repo, "legacy-gate-outcome-parity")
	tx := chain.Records[len(chain.Records)-1].Transaction

	live, err := FreezeCandidateIdentity(context.Background(), repo, tx.Snapshot, tx.PolicyHash)
	if err != nil {
		t.Fatal(err)
	}
	evidence := CoreValidateEvidence{LiveSnapshot: tx.Snapshot, ApplicableAuthorities: 1}
	exact := EvaluateLegacyGate(context.Background(), repo, chain, receiptPath, live, true, evidence, NativeGateRequestInput{Gate: GatePreCommit})
	if exact.Result != GateAllow {
		t.Fatalf("exact live candidate = %#v, want allow", exact)
	}

	writeSnapshotFile(t, repo, "unrelated.txt", "not reviewed\n")
	if err := runSnapshotGit(repo, "add", "unrelated.txt"); err != nil {
		t.Fatal(err)
	}
	changedSnapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	changedLive, err := FreezeCandidateIdentity(context.Background(), repo, changedSnapshot, tx.PolicyHash)
	if err != nil {
		t.Fatal(err)
	}
	changedEvidence := CoreValidateEvidence{LiveSnapshot: changedSnapshot, ApplicableAuthorities: 1}
	changed := EvaluateLegacyGate(context.Background(), repo, chain, receiptPath, changedLive, true, changedEvidence, NativeGateRequestInput{Gate: GatePreCommit})
	if changed.Result == GateAllow {
		t.Fatalf("out-of-scope candidate = %#v, want deny", changed)
	}
}

// TestEvaluateLegacyGateAllowsExactAtAllFiveGates reproduces the verify
// report's CRITICAL-B probe directly: an unchanged, byte-identical legacy
// candidate must allow at every one of the five gates, not just the three
// EvaluateLegacyGateAllowsExactAndDeniesChanged already covered. Before the
// fix, pre-pr and release denied permanently with reason_code
// base_relationship_invalid / release_evidence_missing because
// EvaluateLegacyGate never populated GateContext.BaseRelationshipValid or
// Release at all (base-relationship precondition regression, W3
// EvaluateNativeGate's own gate.go:326/301-310 precedent).
func TestEvaluateLegacyGateAllowsExactAtAllFiveGates(t *testing.T) {
	repo := initSnapshotRepo(t)
	_, chain, receiptPath := legacyApprovedChainFixture(t, repo, "legacy-gate-all-five-exact")
	tx := chain.Records[len(chain.Records)-1].Transaction

	live, err := FreezeCandidateIdentity(context.Background(), repo, tx.Snapshot, tx.PolicyHash)
	if err != nil {
		t.Fatal(err)
	}
	evidence := CoreValidateEvidence{LiveSnapshot: tx.Snapshot, ApplicableAuthorities: 1}

	dir := t.TempDir()
	releaseArtifact := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	releaseInput := NativeGateRequestInput{
		Gate:                       GateRelease,
		ReleaseConfiguration:       releaseArtifact("configuration.txt", "release configuration\n"),
		ReleaseGenerated:           releaseArtifact("generated.txt", "release generated artifact\n"),
		ReleaseProvenance:          releaseArtifact("provenance.txt", "release provenance\n"),
		ReleasePublicationBoundary: releaseArtifact("publication-boundary.txt", "release publication boundary\n"),
		ReleaseEvidenceFreshness:   releaseArtifact("evidence-freshness.txt", "release evidence freshness\n"),
	}

	for _, gateInput := range []NativeGateRequestInput{
		{Gate: GatePostApply}, {Gate: GatePreCommit}, {Gate: GatePrePush}, {Gate: GatePrePR}, releaseInput,
	} {
		evaluation := EvaluateLegacyGate(context.Background(), repo, chain, receiptPath, live, true, evidence, gateInput)
		if evaluation.Result != GateAllow {
			t.Fatalf("gate %q on an unchanged legacy candidate = %#v, want allow", gateInput.Gate, evaluation)
		}
	}
}

// TestEvaluateLegacyGateValidatesReceiptFromAnInFlightCorrection is the
// design.md Migration item 4 / rdd-new-lineage-activation/spec.md:55-58
// regression: "a correction opened before cutover finalizes under the
// pre-cutover correction lifecycle; its receipt then validates through the
// new read-only path once complete." The fix lifecycle itself
// (BeginFix/CompleteFix/ValidateFixDelta) is entirely pre-cutover Transaction
// API, untouched by this slice — this test proves only that
// ProjectLegacyAuthority/EvaluateLegacyGate correctly project a
// FixDiff-terminal chain's receipt (FinalCandidateTree/PolicyHash from the
// CORRECTED state, not the original candidate) and allow an exact live
// candidate matching that corrected result.
func TestEvaluateLegacyGateValidatesReceiptFromAnInFlightCorrection(t *testing.T) {
	repo := initSnapshotRepo(t)
	const lineage = "in-flight-correction-regression"
	writeSnapshotFile(t, repo, "tracked.txt", "original candidate\n")
	initial, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.md")
	ledgerPath := filepath.Join(dir, "ledger.json")
	evidencePath := filepath.Join(dir, "evidence.txt")
	if err := os.WriteFile(policyPath, []byte("legacy bounded policy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finding := Finding{
		ID: "COR-1", Lens: "resilience", Location: "tracked.txt",
		Severity: "CRITICAL", Claim: "candidate requires a bounded correction", ProofRefs: []string{"tracked.txt:1"},
	}
	ledger, err := CanonicalLedger([]Finding{finding})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ledgerPath, ledger, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(evidencePath, []byte("legacy verification passed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	policyHash, err := HashArtifact(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	ledgerHash, err := HashLedgerArtifact(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	evidenceHash, err := HashArtifact(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := NewTransaction(Start{LineageID: lineage, Mode: ModeOrdinary4R, Generation: 1, Snapshot: initial, PolicyHash: policyHash})
	if err != nil {
		t.Fatal(err)
	}
	store, err := AuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	head, err := store.Append("", Record{Operation: "review/start", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.FreezeFindings([]Finding{finding}, ledger, ledgerHash); err != nil {
		t.Fatal(err)
	}
	head, err = store.Append(head, Record{Operation: "review/freeze-findings", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ClassifyEvidence([]FindingEvidence{
		{FindingID: "COR-1", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "candidate-causal correction required"},
	}); err != nil {
		t.Fatal(err)
	}
	head, err = store.Append(head, Record{Operation: "review/classify-evidence", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.BeginFix(hash("f")); err != nil {
		t.Fatal(err)
	}
	head, err = store.Append(head, Record{Operation: "review/begin-fix", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "corrected candidate\n")
	fixSnapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetFixDiff, BaseRef: tx.FinalCandidateTree, IntendedUntracked: []string{}, LedgerIDs: []string{"COR-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fixDeltaHash := FixDeltaHashForSnapshot(fixSnapshot)
	if err := tx.CompleteFix(fixSnapshot, fixDeltaHash, []string{"COR-1"}); err != nil {
		t.Fatal(err)
	}
	head, err = store.Append(head, Record{Operation: "review/complete-fix", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.ValidateFixDelta([]string{"COR-1"}, true); err != nil {
		t.Fatal(err)
	}
	head, err = store.Append(head, Record{Operation: "review/validate-fix-delta", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.BeginFinalVerification(); err != nil {
		t.Fatal(err)
	}
	head, err = store.Append(head, Record{Operation: "review/begin-final-verification", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.CompleteFinalVerification(evidenceHash, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(head, Record{Operation: "review/complete-final-verification", Transaction: *tx}); err != nil {
		t.Fatal(err)
	}
	receipt, err := tx.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(store.Dir, "artifacts", "receipt.json")
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteReceiptAtomic(receiptPath, receipt); err != nil {
		t.Fatal(err)
	}
	chain, err := store.LoadChain()
	if err != nil {
		t.Fatal(err)
	}

	// The corrected candidate is still uncommitted (never git-committed), so
	// the live workspace itself is the exact corrected result — proving the
	// receipt this pre-cutover correction produced validates through the new
	// read-only path once complete. Live is resolved via TargetCurrentChanges
	// (mirroring package cli's governingAuthorityLiveEvidence exactly), NOT
	// tx.Snapshot: a FixDiff transaction's own Snapshot.BaseTree is the
	// pre-fix candidate (the fix hunk's own base), not the receipt's
	// BaseTree (the original starting base) ProjectLegacyAuthority uses —
	// TargetCurrentChanges diffs from the same original base every live
	// gate call diffs from.
	liveSnapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	live, err := FreezeCandidateIdentity(context.Background(), repo, liveSnapshot, tx.PolicyHash)
	if err != nil {
		t.Fatal(err)
	}
	evidence := CoreValidateEvidence{LiveSnapshot: liveSnapshot, ApplicableAuthorities: 1}
	evaluation := EvaluateLegacyGate(context.Background(), repo, chain, receiptPath, live, true, evidence, NativeGateRequestInput{Gate: GatePreCommit})
	if evaluation.Result != GateAllow {
		t.Fatalf("in-flight-correction receipt validation = %#v, want allow", evaluation)
	}
	if evaluation.Context.CandidateTree != receipt.FinalCandidateTree || receipt.FinalCandidateTree == "" {
		t.Fatalf("projected candidate tree = %q, want the corrected FinalCandidateTree %q", evaluation.Context.CandidateTree, receipt.FinalCandidateTree)
	}
}
