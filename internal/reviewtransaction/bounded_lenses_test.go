package reviewtransaction

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOrdinaryBoundedSupportsOnlyZeroOneOrCanonicalFourLenses(t *testing.T) {
	tests := []struct {
		name    string
		lenses  []string
		wantErr bool
	}{
		{name: "zero", lenses: []string{}},
		{name: "one", lenses: []string{"review-reliability"}},
		{name: "four", lenses: append([]string(nil), supportedLenses...)},
		{name: "two", lenses: []string{"review-risk", "review-resilience"}, wantErr: true},
		{name: "unknown", lenses: []string{"review-performance"}, wantErr: true},
		{name: "out of order", lenses: []string{"review-resilience", "review-risk", "review-readability", "review-reliability"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := boundedStart(t, tt.lenses)
			tx, err := NewTransaction(start)
			if tt.wantErr {
				if err == nil {
					t.Fatal("NewTransaction() accepted invalid lens selection")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewTransaction() error = %v", err)
			}
			if tx.Counters != (Counters{}) {
				t.Fatalf("genesis counters = %#v, want all zero", tx.Counters)
			}
			if err := tx.StartReview(); err != nil {
				t.Fatal(err)
			}
			if tx.Counters != (Counters{}) {
				t.Fatalf("started counters = %#v, want all zero", tx.Counters)
			}
		})
	}
}

func TestOrdinaryBoundedRecordsEachSelectedLensExactlyOnceBeforeFreeze(t *testing.T) {
	tx, err := NewTransaction(boundedStart(t, supportedLenses))
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.StartReview()
	if err := freezeTestFindings(tx, []Finding{}); err == nil {
		t.Fatal("FreezeFindings() accepted incomplete selected lenses")
	}
	for _, result := range []LensResult{
		{Lens: "review-performance", Findings: []Finding{}, Evidence: []string{"complete performance sweep"}},
		{Lens: "review-reliability", Findings: []Finding{}, Evidence: []string{"complete reliability sweep"}},
		{Lens: "review-risk", Findings: nil, Evidence: []string{"complete risk sweep"}},
	} {
		if err := tx.RecordLensResult(result); err == nil {
			t.Fatalf("RecordLensResult(%#v) accepted invalid result", result)
		}
	}
	for index, lens := range supportedLenses {
		result := boundedLensResult(lens, string(rune('1'+index)))
		if err := tx.RecordLensResult(result); err != nil {
			t.Fatalf("RecordLensResult(%q) error = %v", lens, err)
		}
		if err := tx.RecordLensResult(result); err == nil {
			t.Fatalf("RecordLensResult(%q) accepted duplicate", lens)
		}
	}
	if err := freezeTestFindings(tx, []Finding{}); err != nil {
		t.Fatalf("FreezeFindings() error = %v", err)
	}
}

func TestOrdinaryBoundedRejectsSupportedButUnselectedLens(t *testing.T) {
	tx, err := NewTransaction(boundedStart(t, []string{LensRisk}))
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.StartReview()
	if err := tx.RecordLensResult(boundedLensResult(LensResilience, "1")); err == nil || !strings.Contains(err.Error(), "was not selected") {
		t.Fatalf("RecordLensResult(unselected) error = %v", err)
	}
}

func TestZeroLensOrdinaryBoundedRequiresExplicitEmptyLedger(t *testing.T) {
	tx, err := NewTransaction(boundedStart(t, []string{}))
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.StartReview()
	if tx.OriginalChangedLines == nil || *tx.OriginalChangedLines != 0 || tx.CorrectionBudget == nil || *tx.CorrectionBudget != 0 {
		t.Fatalf("empty low-risk candidate lines/budget = %v/%v", tx.OriginalChangedLines, tx.CorrectionBudget)
	}
	if err := tx.FreezeFindings(nil, []byte(CanonicalEmptyLedger), ""); err == nil {
		t.Fatal("FreezeFindings(nil) accepted an implicit zero-lens ledger")
	}
	if err := freezeTestFindings(tx, []Finding{}); err != nil {
		t.Fatalf("FreezeFindings(explicit empty) error = %v", err)
	}
}

func TestOrdinaryBoundedLensStateRoundTripsAndLegacyJSONRemainsAdditive(t *testing.T) {
	bounded, err := NewTransaction(boundedStart(t, []string{"review-reliability"}))
	if err != nil {
		t.Fatal(err)
	}
	_ = bounded.StartReview()
	_ = bounded.RecordLensResult(boundedLensResult("review-reliability", "1"))
	payload, err := json.Marshal(bounded)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseTransaction(payload)
	if err != nil {
		t.Fatalf("ParseTransaction() error = %v", err)
	}
	if len(parsed.SelectedLenses) != 1 || len(parsed.LensResults) != 1 || parsed.Counters.ReliabilityExecutions != 1 || parsed.OriginalChangedLines == nil || *parsed.OriginalChangedLines != 2 || parsed.CorrectionBudget == nil || *parsed.CorrectionBudget != 1 {
		t.Fatalf("round-tripped bounded state = %#v", parsed)
	}

	legacy := newTestTransaction(t, ModeOrdinary4R)
	_ = legacy.StartReview()
	legacyPayload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	for _, additiveField := range []string{"risk_level", "selected_lenses", "lens_results", "original_changed_lines", "correction_budget", "proposed_correction_lines", "actual_correction_lines", "risk_executions", "resilience_executions", "readability_executions", "reliability_executions"} {
		if strings.Contains(string(legacyPayload), additiveField) {
			t.Fatalf("legacy ordinary_4r JSON unexpectedly contains %q", additiveField)
		}
	}
	if legacy.Counters.FullReviews != 1 {
		t.Fatalf("legacy FullReviews = %d, want 1", legacy.Counters.FullReviews)
	}
	legacyRevision, err := (Store{Dir: t.TempDir()}).Append("", Record{Operation: "review/start", Transaction: *legacy})
	if err != nil {
		t.Fatal(err)
	}
	const baselineRevision = "sha256:077a4d5e483613817b335c69976e874f37f5112488e70c08dc5ad94ca9bb04a5"
	if legacyRevision != baselineRevision {
		t.Fatalf("legacy ordinary_4r genesis revision = %q, want baseline %q", legacyRevision, baselineRevision)
	}
}

func TestStoreValidatesContentAddressedLensResultSuccessorsAndReplay(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
	tx, err := NewTransaction(boundedStart(t, []string{"review-reliability"}))
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.StartReview()
	genesis, err := store.Append("", Record{Operation: "review/start", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.RecordLensResult(boundedLensResult("review-reliability", "1")); err != nil {
		t.Fatal(err)
	}
	record := Record{Operation: "review/record-lens-result", Transaction: *tx}
	completed, err := store.Append(genesis, record)
	if err != nil {
		t.Fatal(err)
	}
	if retried, err := store.Append(genesis, record); err != nil || retried != completed {
		t.Fatalf("Append(exact retry) = %q, %v; want %q, nil", retried, err, completed)
	}
	chain, err := store.LoadChain()
	if err != nil {
		t.Fatal(err)
	}
	got := chain.Records[len(chain.Records)-1].Transaction
	if chain.HeadRevision != completed || len(got.LensResults) != 1 || got.Counters.ReliabilityExecutions != 1 {
		t.Fatalf("replayed chain = %#v", chain)
	}
	if err := freezeTestFindings(tx, []Finding{}); err != nil {
		t.Fatal(err)
	}
	completed, err = store.Append(completed, Record{Operation: "review/freeze-findings", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ClassifyEvidence([]FindingEvidence{}); err != nil {
		t.Fatal(err)
	}
	completed, err = store.Append(completed, Record{Operation: "review/classify-evidence", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.BeginFinalVerification(); err != nil {
		t.Fatal(err)
	}
	completed, err = store.Append(completed, Record{Operation: "review/begin-final-verification", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.CompleteFinalVerification(hash("4"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(completed, Record{Operation: "review/complete-final-verification", Transaction: *tx}); err != nil {
		t.Fatal(err)
	}
	bundle, err := store.ExportBundle()
	if err != nil {
		t.Fatalf("ExportBundle() error = %v", err)
	}
	payload, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	parsedBundle, err := ParseChainBundle(payload)
	if err != nil {
		t.Fatalf("ParseChainBundle() error = %v", err)
	}
	terminal := parsedBundle.Events[len(parsedBundle.Events)-1]
	if len(terminal.Payload) == 0 || parsedBundle.TerminalReceipt == nil || len(parsedBundle.TerminalReceipt.LensResults) != 1 {
		t.Fatalf("bounded bundle did not preserve lens completion: %#v", parsedBundle)
	}

	forged := *tx
	forged.LensResults = append([]LensResult(nil), tx.LensResults...)
	forged.LensResults[0].ResultHash = hash("2")
	if _, err := store.Append(genesis, Record{Operation: "review/freeze-findings", Transaction: forged}); !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("Append(forged stale result) error = %v, want ErrInvalidSuccessor", err)
	}
	otherStore := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
	fresh, err := NewTransaction(boundedStart(t, []string{"review-reliability"}))
	if err != nil {
		t.Fatal(err)
	}
	_ = fresh.StartReview()
	otherGenesis, err := otherStore.Append("", Record{Operation: "review/start", Transaction: *fresh})
	if err != nil {
		t.Fatal(err)
	}
	if err := fresh.RecordLensResult(boundedLensResult("review-reliability", "2")); err != nil {
		t.Fatal(err)
	}
	forgedResult := *fresh
	forgedResult.PolicyHash = hash("f")
	if _, err := otherStore.Append(otherGenesis, Record{Operation: "review/record-lens-result", Transaction: forgedResult}); !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("Append(forged lens successor) error = %v, want ErrInvalidSuccessor", err)
	}
	if _, err := otherStore.Append(otherGenesis, Record{Operation: "review/freeze-findings", Transaction: *fresh}); !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("Append(forged operation) error = %v, want ErrInvalidSuccessor", err)
	}
}

func TestOrdinaryBoundedDerivesLensIdentityFromStructuredContent(t *testing.T) {
	tx, err := NewTransaction(boundedStart(t, []string{LensRisk, LensResilience, LensReadability, LensReliability}))
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.StartReview()
	risk := boundedLensResult(LensRisk, "shared evidence")
	risk.ResultHash = hash("f")
	if err := tx.RecordLensResult(risk); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("RecordLensResult(caller hash) error = %v", err)
	}
	risk.ResultHash = ""
	if err := tx.RecordLensResult(risk); err != nil {
		t.Fatal(err)
	}
	resilience := boundedLensResult(LensResilience, "shared evidence")
	if err := tx.RecordLensResult(resilience); err != nil {
		t.Fatal(err)
	}
	if tx.LensResults[0].ResultHash == tx.LensResults[1].ResultHash {
		t.Fatal("lens-bound canonical identities allowed cross-lens hash reuse")
	}
	payload, err := json.Marshal(tx)
	if err != nil {
		t.Fatal(err)
	}
	var forged Transaction
	if err := json.Unmarshal(payload, &forged); err != nil {
		t.Fatal(err)
	}
	forged.LensResults[0].Evidence[0] = "tampered evidence"
	forgedPayload, _ := json.Marshal(forged)
	if _, err := ParseTransaction(forgedPayload); err == nil {
		t.Fatal("ParseTransaction() accepted content that no longer matches its canonical lens result hash")
	}
}

func TestOrdinaryBoundedCanonicalizesModelLensOutputInGo(t *testing.T) {
	tx, err := NewTransaction(boundedStart(t, []string{LensReliability}))
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.StartReview()
	input := LensResult{
		Findings: []Finding{{
			Location: " internal/example.go:12 ", Severity: " critical ",
			Claim: " candidate skips rollback ", ProofRefs: []string{" focused test fails on candidate only "},
		}},
		Evidence: []string{" reviewed candidate diff and focused test output "},
	}
	if err := tx.RecordLensResult(input); err != nil {
		t.Fatalf("RecordLensResult(model output) error = %v", err)
	}
	got := tx.LensResults[0]
	wantFinding := Finding{
		ID: "R3-001", Lens: "reliability", Location: "internal/example.go:12", Severity: "CRITICAL",
		Claim: "candidate skips rollback", ProofRefs: []string{"focused test fails on candidate only"},
	}
	if len(got.Findings) != 1 || !reflect.DeepEqual(got.Findings[0], wantFinding) || !equalStrings(got.Evidence, []string{"reviewed candidate diff and focused test output"}) {
		t.Fatalf("canonical lens result = %#v", got)
	}
	if got.ResultHash != LensResultHash(got) {
		t.Fatalf("result hash = %q, want Go-derived hash", got.ResultHash)
	}
	if input.Lens != "" || input.Findings[0].ID != "" || input.Findings[0].Lens != "" || input.Findings[0].Severity != " critical " {
		t.Fatalf("RecordLensResult mutated caller input: %#v", input)
	}
}

func TestCanonicalLensResultAcceptsTechnicalPunctuationAndRejectsExactSentinels(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "git tree peel", value: "HEAD^{tree}"},
		{name: "empty object notation", value: "{}"},
		{name: "symbolic ref", value: "<A>"},
		{name: "transition arrow", value: "candidate => base"},
		{name: "blank", value: "   ", wantErr: true},
		{name: "not applicable", value: "n/a", wantErr: true},
		{name: "none", value: "none", wantErr: true},
		{name: "todo", value: "TODO", wantErr: true},
		{name: "to be determined", value: "tbd", wantErr: true},
		{name: "generic pass", value: "pass", wantErr: true},
		{name: "generic passed", value: "PASSED", wantErr: true},
		{name: "generic success", value: "success", wantErr: true},
		{name: "placeholder", value: "placeholder", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CanonicalLensResult(LensResult{
				Lens: LensReliability,
				Findings: []Finding{{
					Location: "internal/example.go:12", Severity: "CRITICAL", Claim: "candidate behavior differs",
					ProofRefs: []string{tt.value},
				}},
				Evidence: []string{tt.value},
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("CanonicalLensResult(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestStoreRejectsForgedIncompleteLensFreezeBeforeAppend(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
	tx, err := NewTransaction(boundedStart(t, []string{LensReliability}))
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.StartReview()
	genesis, err := store.Append("", Record{Operation: "review/start", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	forged := *tx
	forged.State = StateFindingsFrozen
	forged.Findings = []Finding{}
	forged.LedgerHash = hash("1")
	forged.LedgerFindingsHash = findingsHash(forged.Findings)
	if _, err := store.Append(genesis, Record{Operation: "review/freeze-findings", Transaction: forged}); !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("Append(forged incomplete freeze) error = %v, want ErrInvalidSuccessor", err)
	}
	if chain, err := store.LoadChain(); err != nil || chain.HeadRevision != genesis {
		t.Fatalf("forged successor changed authoritative HEAD: %#v, %v", chain, err)
	}
}

func TestStoreRequiresExactNativeFreezeOperation(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "review-store")}
	tx, err := NewTransaction(boundedStart(t, []string{LensReliability}))
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.StartReview()
	genesis, err := store.Append("", Record{Operation: "review/start", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.RecordLensResult(boundedLensResult(LensReliability, "complete")); err != nil {
		t.Fatal(err)
	}
	completed, err := store.Append(genesis, Record{Operation: "review/record-lens-result", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := freezeTestFindings(tx, []Finding{}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(completed, Record{Operation: "review/complete-final-verification", Transaction: *tx}); !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("Append(forged freeze operation) error = %v, want ErrInvalidSuccessor", err)
	}
}

func boundedStart(t *testing.T, lenses []string) Start {
	t.Helper()
	template := newTestTransaction(t, ModeOrdinary4R)
	originalChangedLines := 2
	riskLevel := RiskHigh
	if len(lenses) == 0 {
		riskLevel = RiskLow
		originalChangedLines = 0
	} else if len(lenses) == 1 {
		riskLevel = RiskMedium
	}
	return Start{
		LineageID: "bounded-lineage", Mode: ModeOrdinaryBounded, Generation: 1,
		Snapshot: template.Snapshot, PolicyHash: hash("d"), RiskLevel: riskLevel, SelectedLenses: append([]string(nil), lenses...),
		OriginalChangedLines: &originalChangedLines,
	}
}

func boundedLensResult(lens, evidence string) LensResult {
	return LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"completed " + evidence + " sweep"}}
}
