package reviewtransaction

import (
	"context"
	"testing"
)

// frozenCompactLedgerHash derives the canonical review-ledger hash for the
// exact findings frozen by the authoritative compact state, independently of
// the production code under test.
func frozenCompactLedgerHash(t *testing.T, findings []Finding) string {
	t.Helper()
	ledger, err := CanonicalLedger(findings)
	if err != nil {
		t.Fatal(err)
	}
	ledgerHash, _, err := validateCanonicalLedger(ledger, findings, "")
	if err != nil {
		t.Fatal(err)
	}
	return ledgerHash
}

func TestCompactStateLedgerHashDistinguishesPristineFromFrozenFindings(t *testing.T) {
	pristine := CompactState{Findings: []Finding{}}
	if got := pristine.LedgerHash(); got != EmptyFixDeltaHash {
		t.Fatalf("pristine ledger hash = %q, want honest empty hash %q", got, EmptyFixDeltaHash)
	}
	if got := (CompactState{}).LedgerHash(); got != EmptyFixDeltaHash {
		t.Fatalf("nil-findings ledger hash = %q, want honest empty hash %q", got, EmptyFixDeltaHash)
	}
	findings := []Finding{{ID: "R3-001", Lens: "reliability", Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "wrong value", ProofRefs: []string{"candidate-only failure"}}}
	frozen := CompactState{Findings: findings}
	if got, want := frozen.LedgerHash(), frozenCompactLedgerHash(t, findings); got != want || got == EmptyFixDeltaHash {
		t.Fatalf("frozen ledger hash = %q, want canonical ledger hash %q", got, want)
	}
}

func TestCompactAllowGateContextBindsFrozenFindingsLedger(t *testing.T) {
	repo, state, receipt, _ := approvedCompactFixDiffFixture(t, "compact-ledger-allow")
	if len(state.Findings) == 0 || len(state.FixFindingIDs) == 0 {
		t.Fatalf("fixture must freeze severe findings with a completed correction: %#v", state)
	}
	want := frozenCompactLedgerHash(t, state.Findings)
	if want == EmptyFixDeltaHash {
		t.Fatalf("canonical ledger hash of frozen findings collides with the empty-input hash %q", want)
	}
	gitSnapshot(t, repo, "add", "other.txt")
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePreCommit, LineageID: state.LineageID})
	if got.Result != GateAllow {
		t.Fatalf("exact corrected pre-commit gate = %#v", got)
	}
	if got.Context.LedgerHash != want {
		t.Fatalf("allow gate context ledger_hash = %q, want frozen findings ledger %q", got.Context.LedgerHash, want)
	}
}

func TestCompactDenialGateContextBindsFrozenFindingsLedger(t *testing.T) {
	repo, state, receipt, _ := approvedCompactFixDiffFixture(t, "compact-ledger-denial")
	want := frozenCompactLedgerHash(t, state.Findings)
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePR, LineageID: state.LineageID, BaseRef: "missing-reviewed-base"})
	if got.Result != GateInvalidated || got.Context.Denial == nil || got.Context.Denial.Stage != "boundary-selection" {
		t.Fatalf("unavailable pre-PR boundary denial = %#v", got)
	}
	if got.Context.LedgerHash != want {
		t.Fatalf("denial gate context ledger_hash = %q, want frozen findings ledger %q", got.Context.LedgerHash, want)
	}
}

func TestCompactScopeChangedGateContextBindsFrozenFindingsLedger(t *testing.T) {
	repo, state, receipt, _ := approvedCompactFixDiffFixture(t, "compact-ledger-scope")
	want := frozenCompactLedgerHash(t, state.Findings)
	gitSnapshot(t, repo, "add", "other.txt")
	writeSnapshotFile(t, repo, "extra.go", "package extra\n")
	gitSnapshot(t, repo, "add", "extra.go")
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePreCommit, LineageID: state.LineageID})
	if got.Result != GateScopeChanged || got.Context.Denial == nil {
		t.Fatalf("scope-changed pre-commit gate = %#v", got)
	}
	if got.Context.LedgerHash != want {
		t.Fatalf("scope-changed gate context ledger_hash = %q, want frozen findings ledger %q", got.Context.LedgerHash, want)
	}
}

func TestCompactPristineGateContextKeepsHonestEmptyLedgerHash(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "candidate")
	state, _, receipt := approvedCompactRevisionFixture(t, repo, "compact-ledger-pristine")
	if len(state.Findings) != 0 {
		t.Fatalf("pristine fixture froze findings: %#v", state.Findings)
	}
	base := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD^"))
	branch := currentBranch(context.Background(), repo)
	remote := configurePublicationRemote(t, repo, branch)
	gitSnapshot(t, repo, "--git-dir", remote, "branch", "reviewed-base", base)
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePR, LineageID: state.LineageID, BaseRef: "origin/reviewed-base"})
	if got.Result != GateAllow {
		t.Fatalf("pristine pre-PR gate = %#v", got)
	}
	if got.Context.LedgerHash != EmptyFixDeltaHash {
		t.Fatalf("pristine gate context ledger_hash = %q, want honest empty hash %q", got.Context.LedgerHash, EmptyFixDeltaHash)
	}
}
