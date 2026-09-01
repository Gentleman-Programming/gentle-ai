package reviewtransaction

import "testing"

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
	pristine := CompactState{}
	if got := pristine.LedgerHash(); got != EmptyFixDeltaHash {
		t.Fatalf("pristine ledger hash = %q, want honest empty hash %q", got, EmptyFixDeltaHash)
	}
	fixture := newCompactReviewerCaptureFixture(t, "ledger-hash-admitted")
	findings := []Finding{{
		ID: "R3-001", Lens: "reliability", Location: "internal/a.go:1", Severity: "CRITICAL",
		Claim: "wrong value", ProofRefs: []string{"candidate-only failure"},
		EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalIntroduced,
	}}
	captureAdmittedCorrectionFinding(t, fixture.store, fixture.state, findings[0])
	record, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := record.State.LedgerHash(), frozenCompactLedgerHash(t, findings); got != want || got == EmptyFixDeltaHash {
		t.Fatalf("frozen ledger hash = %q, want canonical ledger hash %q", got, want)
	}
}
