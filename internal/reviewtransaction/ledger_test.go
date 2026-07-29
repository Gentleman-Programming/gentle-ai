package reviewtransaction

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCanonicalLedgerValidation(t *testing.T) {
	finding := Finding{ID: "R1-001", Lens: "risk", Severity: "CRITICAL", Claim: "unsafe transition", ProofRefs: []string{"store.go:1"}}
	canonical, err := CanonicalLedger([]Finding{finding})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := CanonicalLedger([]Finding{})
	if err != nil {
		t.Fatal(err)
	}
	if string(empty) != CanonicalEmptyLedger {
		t.Fatalf("canonical empty ledger = %q", empty)
	}
	tests := []struct {
		name     string
		payload  string
		findings []Finding
		supplied string
		wantErr  string
	}{
		{name: "malformed", payload: `{`, findings: []Finding{finding}, wantErr: "parse canonical ledger"},
		{name: "missing schema", payload: `{"findings":[]}`, findings: []Finding{}, wantErr: "requires gentle-ai.review-ledger/v1"},
		{name: "wrong schema", payload: `{"schema":"gentle-ai.review-ledger/v2","findings":[]}`, findings: []Finding{}, wantErr: "requires gentle-ai.review-ledger/v1"},
		{name: "missing findings", payload: `{"schema":"gentle-ai.review-ledger/v1"}`, findings: []Finding{}, wantErr: "explicit findings array"},
		{name: "unknown field", payload: `{"schema":"gentle-ai.review-ledger/v1","findings":[],"extra":true}`, findings: []Finding{}, wantErr: "unknown field"},
		{name: "orchestration metadata is not native", payload: `{"schema":"gentle-ai.review-ledger/v1","findings":[{"id":"R1-001","severity":"CRITICAL","evidence_class":"deterministic","status":"open"}]}`, findings: []Finding{{ID: "R1-001", Severity: "CRITICAL"}}, wantErr: "unknown field"},
		{name: "trailing newline", payload: CanonicalEmptyLedger + "\n", findings: []Finding{}, wantErr: "not the canonical compact JSON"},
		{name: "alternate field order", payload: `{"findings":[],"schema":"gentle-ai.review-ledger/v1"}`, findings: []Finding{}, wantErr: "not the canonical compact JSON"},
		{name: "semantic mismatch", payload: string(canonical), findings: []Finding{}, wantErr: "do not exactly match"},
		{name: "supplied hash mismatch", payload: string(canonical), findings: []Finding{finding}, supplied: hash("f"), wantErr: "does not match canonical ledger bytes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := validateCanonicalLedger([]byte(tt.payload), tt.findings, tt.supplied)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateCanonicalLedger() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestFreezeFindingsRejectsInvalidLedgerWithoutMutatingTransaction(t *testing.T) {
	tx := newTestTransaction(t, ModeOrdinary4R)
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	before := *tx
	err := tx.FreezeFindings([]Finding{}, []byte(`{"findings":[]}`), "")
	if err == nil {
		t.Fatal("FreezeFindings() accepted a ledger without the canonical schema")
	}
	if !transactionsEqual(before, *tx) {
		t.Fatal("rejected ledger mutated native transaction state")
	}
}

func TestFreezeFindingsNormalizesWhitespacePaddedIDBeforeBindingLedgerHash(t *testing.T) {
	tx := newTestTransaction(t, ModeOrdinary4R)
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	findings := []Finding{{ID: " R1-001 ", Severity: "CRITICAL"}}
	canonicalFindings := []Finding{{ID: "R1-001", Severity: "CRITICAL"}}
	ledger, err := CanonicalLedger(canonicalFindings)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.FreezeFindings(findings, ledger, ""); err != nil {
		t.Fatalf("FreezeFindings() error = %v", err)
	}
	if len(tx.Findings) != 1 || tx.Findings[0].ID != "R1-001" {
		t.Fatalf("normalized findings = %#v", tx.Findings)
	}
	wantLedgerHash, wantFindingsHash, err := validateCanonicalLedger(ledger, canonicalFindings, "")
	if err != nil {
		t.Fatal(err)
	}
	if tx.LedgerHash != wantLedgerHash || tx.LedgerFindingsHash != wantFindingsHash {
		t.Fatalf("ledger bindings = %q, %q; want %q, %q", tx.LedgerHash, tx.LedgerFindingsHash, wantLedgerHash, wantFindingsHash)
	}
}

func TestLegacyTransactionLedgerHashUsesStableCausalFieldProjection(t *testing.T) {
	tx := newTestTransaction(t, ModeOrdinary4R)
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	findings := []Finding{{
		ID: "R1-001", Lens: "risk", Location: "security/auth.go:42", Severity: "CRITICAL",
		Claim: "candidate bypasses authorization", ProofRefs: []string{"diff: security/auth.go:42"},
		EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalIntroduced,
	}}
	ledger, err := CanonicalLedger(findings)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.FreezeFindings(findings, ledger, ""); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(tx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseTransaction(payload); err != nil {
		t.Fatalf("causal-field transaction no longer validates against its stable v1 ledger projection: %v", err)
	}
}
