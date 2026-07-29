package reviewtransaction

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestOrdinaryTransactionIsOneBoundedNonIterativeFlow(t *testing.T) {
	tx := newTestTransaction(t, ModeOrdinary4R)
	if err := tx.StartReview(); err != nil {
		t.Fatalf("StartReview() error = %v", err)
	}
	if err := freezeTestFindings(tx, []Finding{{ID: "R1-DET", Severity: "CRITICAL"}, {ID: "R2-INF", Severity: "CRITICAL"}}); err != nil {
		t.Fatalf("FreezeFindings() error = %v", err)
	}

	route, err := tx.ClassifyEvidence([]FindingEvidence{
		{FindingID: "R1-DET", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "go test exited 1"},
		{FindingID: "R2-INF", Class: EvidenceInferential, Causality: CausalIntroduced, Proof: "race requires interpretation"},
	})
	if err != nil {
		t.Fatalf("ClassifyEvidence() error = %v", err)
	}
	if len(route.RefuterClaims) != 1 || route.RefuterClaims[0].FindingID != "R2-INF" {
		t.Fatalf("RefuterClaims = %#v, want only inferential finding", route.RefuterClaims)
	}
	if got := tx.Outcomes["R1-DET"]; got != OutcomeCorroborated {
		t.Fatalf("deterministic outcome = %q, want corroborated", got)
	}

	if err := tx.ApplyRefuterOutcomes([]EvidenceResult{{FindingID: "R2-INF", Outcome: OutcomeRefuted, Proof: "locked counterexample"}}); err != nil {
		t.Fatalf("ApplyRefuterOutcomes() error = %v", err)
	}
	if tx.Counters.FullReviews != 1 || tx.Counters.RefuterBatches != 1 {
		t.Fatalf("review counters = %#v", tx.Counters)
	}
	if err := tx.BeginFix(hash("2")); err != nil {
		t.Fatalf("BeginFix() error = %v", err)
	}
	fixSnapshot := tx.Snapshot
	fixSnapshot.Kind = TargetFixDiff
	fixSnapshot.BaseTree = tx.InitialReviewTree
	fixSnapshot.CandidateTree = tree("c")
	fixSnapshot.LedgerIDs = []string{"R1-DET"}
	fixSnapshot.Identity = hash("3")
	if err := tx.CompleteFix(fixSnapshot, hash("4"), []string{"R1-DET"}); err != nil {
		t.Fatalf("CompleteFix() error = %v", err)
	}
	if err := validateOrdinaryFix(tx, []string{"R1-DET"}, true); err != nil {
		t.Fatalf("ValidateFixDelta() error = %v", err)
	}
	if err := validateOrdinaryFix(tx, []string{"R1-DET"}, true); err == nil {
		t.Fatal("ordinary transaction allowed a second scoped fix validation")
	}
	if err := tx.BeginFinalVerification(); err != nil {
		t.Fatalf("BeginFinalVerification() error = %v", err)
	}
	if err := tx.CompleteFinalVerification(hash("5"), true); err != nil {
		t.Fatalf("CompleteFinalVerification() error = %v", err)
	}
	if tx.State != StateApproved {
		t.Fatalf("State = %q, want approved", tx.State)
	}
	want := Counters{FullReviews: 1, RefuterBatches: 1, FixBatches: 1, ScopedFixValidations: 1, FinalVerifications: 1}
	if tx.Counters != want {
		t.Fatalf("Counters = %#v, want %#v", tx.Counters, want)
	}
	if err := tx.BeginFix(hash("6")); err == nil {
		t.Fatal("approved transaction reopened a fix batch")
	}
}

func TestOrdinaryScopedValidatorCanOnlyApproveOrEscalate(t *testing.T) {
	tx := ordinaryAtFixValidation(t)
	if err := validateOrdinaryFix(tx, []string{"R1-DET"}, false); err != nil {
		t.Fatalf("ValidateFixDelta() error = %v", err)
	}
	if tx.State != StateEscalated {
		t.Fatalf("State = %q, want escalated", tx.State)
	}
	if err := tx.BeginFix(hash("9")); err == nil {
		t.Fatal("failed scoped validation triggered another ordinary fix")
	}
}

func TestOrdinaryScopedValidatorRejectsFixCausedDefects(t *testing.T) {
	tx := ordinaryAtFixValidation(t)
	result := ScopedValidationResult{
		LedgerIDs:            []string{"R1-DET"},
		FollowUps:            []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("5"), FixDeltaHash: tx.FixDeltaHash, Passed: true},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("6"), FixDeltaHash: tx.FixDeltaHash, Passed: true},
		FixCausedFindings: []Finding{{
			ID:        "FIX-001",
			Lens:      "scoped-fix-validator",
			Location:  "internal/example.go:12",
			Severity:  "CRITICAL",
			Claim:     "the correction introduced a nil dereference",
			ProofRefs: []string{"go test ./internal/example: exit 1"},
		}},
	}

	if err := tx.ValidateFixDeltaResult(result); err == nil {
		t.Fatal("ordinary scoped validation accepted a new fix-caused finding")
	}
}

func TestJudgmentDayCarriesEarlierSevereFixCausedFindingIntoNextCorrection(t *testing.T) {
	tx := newTestTransaction(t, ModeJudgmentDay)
	_ = tx.StartReview()
	_ = tx.RecordJudgeProofs([]JudgeProof{{JudgeID: "a", ExecutionHash: hash("1"), ResultHash: hash("2"), Blind: true, Confirmed: true}, {JudgeID: "b", ExecutionHash: hash("3"), ResultHash: hash("4"), Blind: true, Confirmed: true}}, hash("5"))
	_ = freezeTestFindings(tx, []Finding{{ID: "JD-001", Severity: "CRITICAL"}})
	_, _ = tx.ClassifyEvidence([]FindingEvidence{{FindingID: "JD-001", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "reproduced"}})
	_ = tx.BeginFix(hash("7"))
	fix := tx.Snapshot
	fix.Kind, fix.BaseTree, fix.CandidateTree, fix.LedgerIDs, fix.Identity = TargetFixDiff, tx.FinalCandidateTree, tree("c"), []string{"JD-001"}, hash("8")
	_ = tx.CompleteFix(fix, hash("9"), []string{"JD-001"})
	if err := tx.ValidateFixDeltaResult(ScopedValidationResult{LedgerIDs: []string{"JD-001"}, FixCausedFindings: []Finding{{ID: "FIX-001", Lens: "scoped", Location: "x.go:1", Severity: "CRITICAL", Claim: "new defect", ProofRefs: []string{"test failure"}}}}); err != nil {
		t.Fatal(err)
	}
	if tx.State != StateFixRequired || !equalStrings(tx.FixFindingIDs, []string{"FIX-001", "JD-001"}) {
		t.Fatalf("severe earlier fix-caused finding was not correction-bound: state=%q ids=%v", tx.State, tx.FixFindingIDs)
	}
}

func TestFrozenLedgerFindingsHashDetectsTamperedFrozenFindings(t *testing.T) {
	tx := newTestTransaction(t, ModeOrdinary4R)
	_ = tx.StartReview()
	_ = freezeTestFindings(tx, []Finding{{ID: "R1-001", Severity: "CRITICAL"}})
	tx.Findings[0].Severity = "WARNING"
	if _, err := ParseTransaction(mustMarshalTransaction(t, *tx)); err == nil {
		t.Fatal("ParseTransaction() accepted frozen findings that no longer match their ledger binding")
	}
}

func TestCompleteFixDerivesDeltaIdentityInsteadOfTrustingCallerArtifact(t *testing.T) {
	tx := ordinaryAtFixValidation(t)
	if tx.FixDeltaHash != FixDeltaHashForSnapshot(tx.Snapshot) {
		t.Fatalf("FixDeltaHash = %q, want authoritative snapshot-derived identity %q", tx.FixDeltaHash, FixDeltaHashForSnapshot(tx.Snapshot))
	}
	if tx.FixDeltaHash == hash("4") {
		t.Fatal("arbitrary caller artifact hash satisfied fix-delta binding")
	}
}

func TestBudgetedBeginFixRejectsInvalidForecastBeforeMutation(t *testing.T) {
	tests := []struct {
		name          string
		forecast      int
		wantErr       bool
		wantEscalated bool
	}{
		{name: "zero", forecast: 0, wantErr: true},
		{name: "within", forecast: 1},
		{name: "exact", forecast: 98},
		{name: "over", forecast: 99, wantEscalated: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := budgetedAtFixRequired(t, 196)
			before := mustMarshalTransaction(t, *tx)
			err := tx.BeginFix(hash("2"), tt.forecast)
			if tt.wantErr {
				if err == nil {
					t.Fatal("BeginFix() accepted invalid correction forecast")
				}
				if got := mustMarshalTransaction(t, *tx); !bytes.Equal(got, before) {
					t.Fatalf("rejected forecast mutated transaction:\nbefore=%s\nafter=%s", before, got)
				}
				return
			}
			if tt.wantEscalated {
				if err != nil || tx.State != StateEscalated || tx.Counters.FixBatches != 0 || tx.ProposedCorrectionLines == nil || *tx.ProposedCorrectionLines != tt.forecast || tx.FailedEvidenceRevision != hash("2") {
					t.Fatalf("BeginFix(over budget) = state %q counters %#v forecast %v evidence %q err %v", tx.State, tx.Counters, tx.ProposedCorrectionLines, tx.FailedEvidenceRevision, err)
				}
				if retryErr := tx.BeginFix(hash("3"), 1); retryErr == nil {
					t.Fatal("escalated forecast remained retryable")
				}
				return
			}
			if err != nil || tx.State != StateFixing || tx.Counters.FixBatches != 1 || tx.ProposedCorrectionLines == nil || *tx.ProposedCorrectionLines != tt.forecast {
				t.Fatalf("BeginFix() = state %q counters %#v forecast %v err %v", tx.State, tx.Counters, tx.ProposedCorrectionLines, err)
			}
		})
	}
}

func TestBudgetedCompleteFixRejectsOverBudgetActualBeforeMutation(t *testing.T) {
	tests := []struct {
		name    string
		actual  int
		wantErr bool
	}{
		{name: "within", actual: 1},
		{name: "exact", actual: 98},
		{name: "over", actual: 99, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := budgetedAtFixRequired(t, 196)
			if err := tx.BeginFix(hash("2"), 98); err != nil {
				t.Fatal(err)
			}
			fix := tx.Snapshot
			fix.Kind, fix.BaseTree, fix.CandidateTree = TargetFixDiff, tx.FinalCandidateTree, tree("c")
			fix.LedgerIDs, fix.Identity = []string{"REL-001"}, hash("3")
			before := mustMarshalTransaction(t, *tx)
			err := tx.CompleteFix(fix, hash("4"), fix.LedgerIDs, tt.actual)
			if tt.wantErr {
				if err == nil {
					t.Fatal("CompleteFix() accepted over-budget actual correction")
				}
				if got := mustMarshalTransaction(t, *tx); !bytes.Equal(got, before) {
					t.Fatalf("rejected actual correction mutated transaction:\nbefore=%s\nafter=%s", before, got)
				}
				return
			}
			if err != nil || tx.State != StateFixValidating || tx.ActualCorrectionLines == nil || *tx.ActualCorrectionLines != tt.actual {
				t.Fatalf("CompleteFix() = state %q actual %v err %v", tx.State, tx.ActualCorrectionLines, err)
			}
			if tx.RiskLevel != RiskMedium || !equalStrings(tx.SelectedLenses, []string{LensReliability}) || *tx.OriginalChangedLines != 196 || *tx.CorrectionBudget != 98 {
				t.Fatalf("correction changed frozen risk inputs: %#v", tx)
			}
		})
	}
}

func mustMarshalTransaction(t *testing.T, transaction Transaction) []byte {
	t.Helper()
	payload, err := json.Marshal(transaction)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestInsufficientEvidenceIsInconclusiveAndNeverAutoFixed(t *testing.T) {
	tx := newTestTransaction(t, ModeOrdinary4R)
	_ = tx.StartReview()
	_ = freezeTestFindings(tx, []Finding{{ID: "R3-INS", Severity: "CRITICAL"}})
	route, err := tx.ClassifyEvidence([]FindingEvidence{{FindingID: "R3-INS", Class: EvidenceInsufficient, Causality: CausalUnknown, Proof: "no observable behavior"}})
	if err != nil {
		t.Fatalf("ClassifyEvidence() error = %v", err)
	}
	if len(route.AutoFixFindingIDs) != 0 || tx.Outcomes["R3-INS"] != OutcomeInconclusive {
		t.Fatalf("route/outcomes = %#v / %#v", route, tx.Outcomes)
	}
	if tx.State != StateEscalated {
		t.Fatalf("State = %q, want escalated", tx.State)
	}
}

func TestCausalDispositionControlsDeterministicSevereAdmission(t *testing.T) {
	tests := []struct {
		name        string
		causality   CausalDisposition
		wantState   State
		wantOutcome EvidenceOutcome
		wantFix     bool
		wantFollow  bool
	}{
		{name: "introduced", causality: CausalIntroduced, wantState: StateFixRequired, wantOutcome: OutcomeCorroborated, wantFix: true},
		{name: "behavior activated", causality: CausalBehaviorActivated, wantState: StateFixRequired, wantOutcome: OutcomeCorroborated, wantFix: true},
		{name: "worsened", causality: CausalWorsened, wantState: StateFixRequired, wantOutcome: OutcomeCorroborated, wantFix: true},
		{name: "pre-existing", causality: CausalPreExisting, wantState: StateReadyFinalVerification, wantOutcome: OutcomeInfo, wantFollow: true},
		{name: "base only", causality: CausalBaseOnly, wantState: StateReadyFinalVerification, wantOutcome: OutcomeInfo, wantFollow: true},
		{name: "unknown", causality: CausalUnknown, wantState: StateEscalated, wantOutcome: OutcomeInconclusive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := newTestTransaction(t, ModeOrdinary4R)
			_ = tx.StartReview()
			finding := Finding{ID: "R1-001", Severity: "CRITICAL", Claim: "observable severe defect", ProofRefs: []string{"internal/example.go:12"}}
			_ = freezeTestFindings(tx, []Finding{finding})
			route, err := tx.ClassifyEvidence([]FindingEvidence{{
				FindingID: finding.ID, Class: EvidenceDeterministic, Causality: tt.causality, Proof: "candidate/base differential proof",
			}})
			if err != nil {
				t.Fatalf("ClassifyEvidence() error = %v", err)
			}
			if tx.State != tt.wantState || tx.Outcomes[finding.ID] != tt.wantOutcome || (len(tx.FixFindingIDs) == 1) != tt.wantFix || (len(route.AutoFixFindingIDs) == 1) != tt.wantFix || (len(tx.FollowUps) == 1) != tt.wantFollow {
				t.Fatalf("route = %#v state=%q outcomes=%#v fixes=%v follow-ups=%#v", route, tx.State, tx.Outcomes, tx.FixFindingIDs, tx.FollowUps)
			}
			if tt.wantFollow && !hasFollowUp(tx.FollowUps, causalFollowUp(finding, "candidate/base differential proof")) {
				t.Fatalf("unrelated finding proof was not preserved: %#v", tx.FollowUps)
			}
		})
	}
}

func TestMixedCausalLedgerCorrectsOnlyCandidateCausalFindings(t *testing.T) {
	tx := newTestTransaction(t, ModeOrdinary4R)
	_ = tx.StartReview()
	findings := []Finding{
		{ID: "R1-INTRO", Severity: "CRITICAL", Claim: "candidate introduced failure"},
		{ID: "R2-WORSE", Severity: "BLOCKER", Claim: "candidate worsened race"},
		{ID: "R3-PRE", Severity: "CRITICAL", Claim: "defect exists unchanged on base"},
		{ID: "R4-BASE", Severity: "BLOCKER", Claim: "failure occurs only on base"},
	}
	_ = freezeTestFindings(tx, findings)
	route, err := tx.ClassifyEvidence([]FindingEvidence{
		{FindingID: "R1-INTRO", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk fails focused test"},
		{FindingID: "R2-WORSE", Class: EvidenceInferential, Causality: CausalWorsened, Proof: "candidate trace increases race window"},
		{FindingID: "R3-PRE", Class: EvidenceDeterministic, Causality: CausalPreExisting, Proof: "same test fails on base and candidate"},
		{FindingID: "R4-BASE", Class: EvidenceDeterministic, Causality: CausalBaseOnly, Proof: "test fails on base and passes on candidate"},
	})
	if err != nil {
		t.Fatalf("ClassifyEvidence() error = %v", err)
	}
	if tx.State != StateEvidenceClassified || !equalStrings(tx.FixFindingIDs, []string{"R1-INTRO"}) || !equalStrings(tx.PendingRefuterIDs, []string{"R2-WORSE"}) || len(route.RefuterClaims) != 1 || len(tx.FollowUps) != 2 {
		t.Fatalf("mixed classification route = %#v transaction=%#v", route, tx)
	}
	if err := tx.ApplyRefuterOutcomes([]EvidenceResult{{FindingID: "R2-WORSE", Outcome: OutcomeCorroborated, Proof: "independent before/after trace"}}); err != nil {
		t.Fatalf("ApplyRefuterOutcomes() error = %v", err)
	}
	if tx.State != StateFixRequired || !equalStrings(tx.FixFindingIDs, []string{"R1-INTRO", "R2-WORSE"}) || len(tx.FollowUps) != 2 {
		t.Fatalf("mixed resolved ledger = state %q fixes %v follow-ups %#v", tx.State, tx.FixFindingIDs, tx.FollowUps)
	}
}

func TestClassifyEvidenceRejectsMissingOrInvalidCausalityBeforeMutation(t *testing.T) {
	tests := []struct {
		name      string
		causality CausalDisposition
	}{
		{name: "missing"},
		{name: "invalid", causality: "candidate-adjacent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := newTestTransaction(t, ModeOrdinary4R)
			_ = tx.StartReview()
			_ = freezeTestFindings(tx, []Finding{{ID: "R1-001", Severity: "CRITICAL"}})
			before := string(mustMarshalTransaction(t, *tx))
			if _, err := tx.ClassifyEvidence([]FindingEvidence{{FindingID: "R1-001", Class: EvidenceDeterministic, Causality: tt.causality, Proof: "concrete differential proof"}}); err == nil {
				t.Fatal("ClassifyEvidence() accepted missing or invalid causality")
			}
			if after := string(mustMarshalTransaction(t, *tx)); after != before {
				t.Fatalf("rejected classification mutated transaction\nbefore: %s\nafter:  %s", before, after)
			}
		})
	}
}

func TestJudgmentDayCorroborationDoesNotBypassCausalScope(t *testing.T) {
	tests := []struct {
		name      string
		class     EvidenceClass
		causality CausalDisposition
		wantState State
		wantFix   bool
	}{
		{name: "candidate causal inferential agreement", class: EvidenceInferential, causality: CausalBehaviorActivated, wantState: StateFixRequired, wantFix: true},
		{name: "pre-existing deterministic agreement", class: EvidenceDeterministic, causality: CausalPreExisting, wantState: StateReadyFinalVerification},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := newTestTransaction(t, ModeJudgmentDay)
			_ = tx.StartReview()
			_ = tx.RecordJudgeProofs([]JudgeProof{
				{JudgeID: "judge-a", ExecutionHash: hash("1"), ResultHash: hash("2"), Blind: true, Confirmed: true},
				{JudgeID: "judge-b", ExecutionHash: hash("3"), ResultHash: hash("4"), Blind: true, Confirmed: true},
			}, hash("5"))
			_ = freezeTestFindings(tx, []Finding{{ID: "JD-001", Severity: "CRITICAL", Claim: "agreed defect"}})
			route, err := tx.ClassifyEvidence([]FindingEvidence{{FindingID: "JD-001", Class: tt.class, Causality: tt.causality, Proof: "two-judge concrete scope proof"}})
			if err != nil {
				t.Fatalf("ClassifyEvidence() error = %v", err)
			}
			if tx.State != tt.wantState || (len(tx.FixFindingIDs) == 1) != tt.wantFix || (len(route.AutoFixFindingIDs) == 1) != tt.wantFix || len(route.RefuterClaims) != 0 {
				t.Fatalf("Judgment Day causal route = %#v state=%q fixes=%v", route, tx.State, tx.FixFindingIDs)
			}
		})
	}
}

func TestTransactionCausalityRoundTripAndLegacyReadCompatibility(t *testing.T) {
	tx := newTestTransaction(t, ModeOrdinary4R)
	_ = tx.StartReview()
	_ = freezeTestFindings(tx, []Finding{{ID: "R1-001", Severity: "CRITICAL"}})
	_, _ = tx.ClassifyEvidence([]FindingEvidence{{FindingID: "R1-001", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk fails focused test"}})
	payload := mustMarshalTransaction(t, *tx)
	parsed, err := ParseTransaction(payload)
	if err != nil {
		t.Fatalf("ParseTransaction(new) error = %v", err)
	}
	if got := parsed.Classifications["R1-001"].Causality; got != CausalIntroduced || parsed.legacyCausality {
		t.Fatalf("new causality round trip = %q legacy=%v", got, parsed.legacyCausality)
	}

	legacyPayload := []byte(strings.Replace(string(payload), `"causal_disposition":"introduced",`, "", 1))
	legacy, err := ParseTransaction(legacyPayload)
	if err != nil {
		t.Fatalf("ParseTransaction(legacy) error = %v", err)
	}
	if !legacy.legacyCausality || legacy.Classifications["R1-001"].Causality != "" || legacy.State != StateFixRequired || !equalStrings(legacy.FixFindingIDs, []string{"R1-001"}) {
		t.Fatalf("legacy classification was not preserved fail-closed: %#v", legacy)
	}
}

func TestMalformedRefuterBatchIsConsumedAndTerminal(t *testing.T) {
	tests := []struct {
		name    string
		results []EvidenceResult
	}{
		{name: "missing output", results: nil},
		{name: "incomplete output", results: []EvidenceResult{{FindingID: "R2-INF", Outcome: OutcomeCorroborated, Proof: "independent trace"}}},
		{name: "malformed output", results: []EvidenceResult{
			{FindingID: "R2-INF", Outcome: OutcomeCorroborated, Proof: "independent trace"},
			{FindingID: "R3-INF", Outcome: "unknown", Proof: "independent trace"},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := newTestTransaction(t, ModeOrdinary4R)
			_ = tx.StartReview()
			_ = freezeTestFindings(tx, []Finding{
				{ID: "R2-INF", Severity: "CRITICAL"},
				{ID: "R3-INF", Severity: "BLOCKER"},
			})
			_, _ = tx.ClassifyEvidence([]FindingEvidence{
				{FindingID: "R2-INF", Class: EvidenceInferential, Causality: CausalIntroduced, Proof: "race requires interpretation"},
				{FindingID: "R3-INF", Class: EvidenceInferential, Causality: CausalWorsened, Proof: "ordering requires interpretation"},
			})

			if err := tx.ApplyRefuterOutcomes(tt.results); err == nil {
				t.Fatal("ApplyRefuterOutcomes() accepted malformed terminal output")
			}
			if tx.Counters.RefuterBatches != 1 || tx.State != StateEscalated || len(tx.PendingRefuterIDs) != 0 {
				t.Fatalf("terminal state = %q counters=%#v pending=%v", tx.State, tx.Counters, tx.PendingRefuterIDs)
			}
			for _, id := range []string{"R2-INF", "R3-INF"} {
				if tx.Outcomes[id] != OutcomeInconclusive {
					t.Fatalf("Outcomes[%s] = %q, want inconclusive", id, tx.Outcomes[id])
				}
			}
			if err := tx.ApplyRefuterOutcomes([]EvidenceResult{
				{FindingID: "R2-INF", Outcome: OutcomeCorroborated, Proof: "late retry"},
				{FindingID: "R3-INF", Outcome: OutcomeCorroborated, Proof: "late retry"},
			}); err == nil {
				t.Fatal("terminal malformed batch remained retryable")
			}
		})
	}
}

func TestOnlySevereFindingsEnterEvidenceRouting(t *testing.T) {
	tx := newTestTransaction(t, ModeOrdinary4R)
	_ = tx.StartReview()
	findings := []Finding{
		{ID: "R1-BLOCKER", Severity: "BLOCKER"},
		{ID: "R2-CRITICAL", Severity: "CRITICAL"},
		{ID: "R3-WARNING", Severity: "WARNING"},
		{ID: "R4-SUGGESTION", Severity: "SUGGESTION"},
	}
	if err := freezeTestFindings(tx, findings); err != nil {
		t.Fatalf("FreezeFindings() error = %v", err)
	}
	route, err := tx.ClassifyEvidence([]FindingEvidence{
		{FindingID: "R1-BLOCKER", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "failing security test"},
		{FindingID: "R2-CRITICAL", Class: EvidenceInferential, Causality: CausalWorsened, Proof: "concurrency trace"},
	})
	if err != nil {
		t.Fatalf("ClassifyEvidence() error = %v", err)
	}
	if len(route.AutoFixFindingIDs) != 1 || route.AutoFixFindingIDs[0] != "R1-BLOCKER" || len(route.RefuterClaims) != 1 || route.RefuterClaims[0].FindingID != "R2-CRITICAL" {
		t.Fatalf("route = %#v", route)
	}
	for _, id := range []string{"R3-WARNING", "R4-SUGGESTION"} {
		if tx.Outcomes[id] != OutcomeInfo {
			t.Fatalf("Outcomes[%s] = %q, want info", id, tx.Outcomes[id])
		}
	}
}

func TestJudgmentDayRequiresTwoDistinctBlindJudgeProofs(t *testing.T) {
	proofA := JudgeProof{JudgeID: "judge-a", ExecutionHash: hash("1"), ResultHash: hash("2"), Blind: true, Confirmed: true}
	proofB := JudgeProof{JudgeID: "judge-b", ExecutionHash: hash("3"), ResultHash: hash("4"), Blind: true, Confirmed: true}

	tests := []struct {
		name   string
		proofs []JudgeProof
	}{
		{name: "zero judges", proofs: nil},
		{name: "one judge", proofs: []JudgeProof{proofA}},
		{name: "duplicate execution", proofs: []JudgeProof{proofA, {JudgeID: "judge-b", ExecutionHash: proofA.ExecutionHash, ResultHash: hash("4"), Blind: true, Confirmed: true}}},
		{name: "duplicate result", proofs: []JudgeProof{proofA, {JudgeID: "judge-b", ExecutionHash: hash("3"), ResultHash: proofA.ResultHash, Blind: true, Confirmed: true}}},
		{name: "not blind", proofs: []JudgeProof{proofA, {JudgeID: "judge-b", ExecutionHash: hash("3"), ResultHash: hash("4"), Confirmed: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := newTestTransaction(t, ModeJudgmentDay)
			_ = tx.StartReview()
			if len(tt.proofs) > 0 {
				if err := tx.RecordJudgeProofs(tt.proofs, hash("5")); err == nil {
					t.Fatal("RecordJudgeProofs() accepted incomplete or duplicate judge proof")
				}
			}
			if err := freezeTestFindings(tx, []Finding{{ID: "JD-001", Severity: "CRITICAL"}}); err == nil {
				t.Fatal("FreezeFindings() accepted Judgment Day without two confirmed judges")
			}
		})
	}

	tx := newTestTransaction(t, ModeJudgmentDay)
	_ = tx.StartReview()
	if err := tx.RecordJudgeProofs([]JudgeProof{proofA, proofB}, hash("5")); err != nil {
		t.Fatalf("RecordJudgeProofs(valid) error = %v", err)
	}
	if err := freezeTestFindings(tx, []Finding{{ID: "JD-001", Severity: "CRITICAL"}}); err != nil {
		t.Fatalf("FreezeFindings(valid proof) error = %v", err)
	}
	if tx.Counters.JudgeExecutions != 2 || tx.JudgeProofHash == "" {
		t.Fatalf("judge proof = %q counters=%#v", tx.JudgeProofHash, tx.Counters)
	}
}

func TestJudgmentDayHasExactlyTwoFixAndScopedRejudgmentRounds(t *testing.T) {
	tx := newTestTransaction(t, ModeJudgmentDay)
	_ = tx.StartReview()
	if err := tx.RecordJudgeProofs([]JudgeProof{
		{JudgeID: "judge-a", ExecutionHash: hash("a"), ResultHash: hash("b"), Blind: true, Confirmed: true},
		{JudgeID: "judge-b", ExecutionHash: hash("c"), ResultHash: hash("d"), Blind: true, Confirmed: true},
	}, hash("e")); err != nil {
		t.Fatal(err)
	}
	_ = freezeTestFindings(tx, []Finding{{ID: "JD-001", Severity: "CRITICAL"}})
	if _, err := tx.ClassifyEvidence([]FindingEvidence{{FindingID: "JD-001", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "confirmed by both judges"}}); err != nil {
		t.Fatalf("ClassifyEvidence() error = %v", err)
	}
	for round := 1; round <= 2; round++ {
		if err := tx.BeginFix(hash(string(rune('1' + round)))); err != nil {
			t.Fatalf("BeginFix(round %d) error = %v", round, err)
		}
		fix := tx.Snapshot
		fix.Kind = TargetFixDiff
		fix.BaseTree = tx.FinalCandidateTree
		fix.CandidateTree = tree(string(rune('c' + round)))
		fix.LedgerIDs = []string{"JD-001"}
		fix.Identity = hash(string(rune('5' + round)))
		if err := tx.CompleteFix(fix, hash(string(rune('7'+round))), []string{"JD-001"}); err != nil {
			t.Fatalf("CompleteFix(round %d) error = %v", round, err)
		}
		if err := tx.ValidateFixDelta([]string{"JD-001"}, false); err != nil {
			t.Fatalf("ValidateFixDelta(round %d) error = %v", round, err)
		}
	}
	if tx.Counters.FixRounds != 2 || tx.Counters.ScopedRejudgments != 2 || tx.State != StateEscalated {
		t.Fatalf("final judgment-day state = %q counters=%#v", tx.State, tx.Counters)
	}
	if err := tx.BeginFix(hash("f")); err == nil {
		t.Fatal("Judgment Day allowed a third fix round")
	}
}

func TestCompleteFixEnforcesImmutableGenesisPathSubsetBeforeMutation(t *testing.T) {
	tests := []struct {
		name    string
		paths   []string
		wantErr bool
	}{
		{name: "canonical subset", paths: []string{"internal/example.go"}},
		{name: "added path", paths: []string{"internal/example.go", "internal/added.go"}, wantErr: true},
		{name: "renamed destination", paths: []string{"internal/renamed.go"}, wantErr: true},
		{name: "deleted out of scope path", paths: []string{"internal/deleted.go"}, wantErr: true},
		{name: "intended untracked path", paths: []string{"internal/untracked.go"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := ordinaryAtFixing(t)
			before := *tx
			fix := tx.Snapshot
			fix.Kind, fix.BaseTree, fix.CandidateTree = TargetFixDiff, tx.FinalCandidateTree, tree("c")
			fix.LedgerIDs, fix.Paths, fix.Identity = []string{"R1-DET"}, tt.paths, hash("3")

			err := tx.CompleteFix(fix, hash("4"), []string{"R1-DET"})
			if tt.wantErr {
				if err == nil {
					t.Fatal("CompleteFix() accepted an out-of-scope correction")
				}
				if tx.State != before.State || tx.Counters != before.Counters || tx.FinalCandidateTree != before.FinalCandidateTree {
					t.Fatalf("rejected correction mutated transaction: %#v", tx)
				}
				return
			}
			if err != nil {
				t.Fatalf("CompleteFix() error = %v", err)
			}
		})
	}
}

func TestScopedValidationRequiresBoundEvidenceAndKeepsFollowUpsNonBlocking(t *testing.T) {
	tx := ordinaryAtFixValidation(t)
	result := ScopedValidationResult{
		LedgerIDs:            []string{"R1-DET"},
		FixCausedFindings:    []Finding{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("5"), FixDeltaHash: tx.FixDeltaHash, Passed: true},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("6"), FixDeltaHash: tx.FixDeltaHash, Passed: true},
		FollowUps:            []FollowUp{{Observation: "new optional cleanup", ProofRefs: []string{"internal/example.go:1"}}},
	}
	if err := tx.ValidateFixDeltaResult(result); err != nil {
		t.Fatalf("ValidateFixDeltaResult() error = %v", err)
	}
	if tx.State != StateReadyFinalVerification || len(tx.FollowUps) != 1 {
		t.Fatalf("targeted validation state/follow-ups = %q/%#v", tx.State, tx.FollowUps)
	}
	if len(tx.FixCausedFindings) != 0 || !equalStrings(tx.FixFindingIDs, []string{"R1-DET"}) {
		t.Fatalf("follow-up mutated correction ledger: findings=%#v ids=%v", tx.FixCausedFindings, tx.FixFindingIDs)
	}
}

func TestScopedValidationRejectsIncompleteOrStaleEvidenceAndEscalatesRegression(t *testing.T) {
	tests := []struct {
		name   string
		result ScopedValidationResult
	}{
		{name: "missing criteria evidence", result: ScopedValidationResult{LedgerIDs: []string{"R1-DET"}}},
		{name: "stale regression evidence", result: ScopedValidationResult{LedgerIDs: []string{"R1-DET"}, OriginalCriteria: ValidationCheck{EvidenceHash: hash("5"), FixDeltaHash: hash("9"), Passed: true}, CorrectionRegression: ValidationCheck{EvidenceHash: hash("6"), FixDeltaHash: hash("9"), Passed: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := ordinaryAtFixValidation(t)
			if err := tx.ValidateFixDeltaResult(tt.result); err == nil {
				t.Fatal("ValidateFixDeltaResult() accepted incomplete or stale targeted evidence")
			}
			if tx.State != StateFixValidating || tx.Counters.ScopedFixValidations != 0 {
				t.Fatalf("rejected validation mutated state/counters: %q/%#v", tx.State, tx.Counters)
			}
		})
	}

	tx := ordinaryAtFixValidation(t)
	result := ScopedValidationResult{
		LedgerIDs:            []string{"R1-DET"},
		FixCausedFindings:    []Finding{},
		FollowUps:            []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("5"), FixDeltaHash: tx.FixDeltaHash, Passed: true},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("6"), FixDeltaHash: tx.FixDeltaHash, Passed: false},
	}
	if err := tx.ValidateFixDeltaResult(result); err != nil {
		t.Fatal(err)
	}
	if tx.State != StateEscalated || tx.Counters.FixBatches != 1 {
		t.Fatalf("failed regression did not escalate one-correction transaction: %q/%#v", tx.State, tx.Counters)
	}
}

func TestFinalVerificationContradictionEscalatesWithoutReopeningReviewBudgets(t *testing.T) {
	tx := ordinaryAtFixValidation(t)
	if err := validateOrdinaryFix(tx, []string{"R1-DET"}, true); err != nil {
		t.Fatal(err)
	}
	before := tx.Counters
	if err := tx.BeginFinalVerification(); err != nil {
		t.Fatal(err)
	}
	if err := tx.CompleteFinalVerification(hash("f"), false); err != nil {
		t.Fatal(err)
	}
	if tx.State != StateEscalated {
		t.Fatalf("State = %q, want escalated", tx.State)
	}
	if tx.Counters.FullReviews != before.FullReviews || tx.Counters.RefuterBatches != before.RefuterBatches || tx.Counters.FixBatches != before.FixBatches || tx.Counters.ScopedFixValidations != before.ScopedFixValidations {
		t.Fatalf("contradiction reopened review budgets: before=%#v after=%#v", before, tx.Counters)
	}
	if err := tx.BeginFix(hash("e")); err == nil {
		t.Fatal("final-verification contradiction reopened an ordinary correction")
	}
}

func TestMultipleFrozenFindingsShareOneFixBatch(t *testing.T) {
	tx := newTestTransaction(t, ModeOrdinary4R)
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	findings := []Finding{{ID: "R1-001", Severity: "CRITICAL"}, {ID: "R1-002", Severity: "CRITICAL"}, {ID: "R1-003", Severity: "CRITICAL"}}
	if err := freezeTestFindings(tx, findings); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ClassifyEvidence([]FindingEvidence{
		{FindingID: "R1-001", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "first work unit"},
		{FindingID: "R1-002", Class: EvidenceDeterministic, Causality: CausalBehaviorActivated, Proof: "second work unit"},
		{FindingID: "R1-003", Class: EvidenceDeterministic, Causality: CausalWorsened, Proof: "third work unit"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.BeginFix(hash("2")); err != nil {
		t.Fatal(err)
	}
	fix := tx.Snapshot
	fix.Kind, fix.BaseTree, fix.CandidateTree = TargetFixDiff, tx.FinalCandidateTree, tree("c")
	fix.LedgerIDs, fix.Identity = []string{"R1-001", "R1-002", "R1-003"}, hash("3")
	if err := tx.CompleteFix(fix, hash("4"), fix.LedgerIDs); err != nil {
		t.Fatal(err)
	}
	if tx.Counters.FixBatches != 1 || !equalStrings(tx.FixFindingIDs, fix.LedgerIDs) {
		t.Fatalf("three work units did not share one correction budget: counters=%#v ids=%v", tx.Counters, tx.FixFindingIDs)
	}
}

func ordinaryAtFixValidation(t *testing.T) *Transaction {
	t.Helper()
	tx := ordinaryAtFixing(t)
	fix := tx.Snapshot
	fix.Kind = TargetFixDiff
	fix.BaseTree = tx.InitialReviewTree
	fix.CandidateTree = tree("c")
	fix.LedgerIDs = []string{"R1-DET"}
	fix.Identity = hash("3")
	if err := tx.CompleteFix(fix, hash("4"), []string{"R1-DET"}); err != nil {
		t.Fatalf("CompleteFix() error = %v", err)
	}
	return tx
}

func validateOrdinaryFix(tx *Transaction, ledgerIDs []string, approved bool) error {
	return tx.ValidateFixDeltaResult(ScopedValidationResult{
		LedgerIDs: ledgerIDs, Approved: approved, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("5"), FixDeltaHash: tx.FixDeltaHash, Passed: approved},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("6"), FixDeltaHash: tx.FixDeltaHash, Passed: approved},
	})
}

func ordinaryAtFixing(t *testing.T) *Transaction {
	t.Helper()
	tx := newTestTransaction(t, ModeOrdinary4R)
	_ = tx.StartReview()
	_ = freezeTestFindings(tx, []Finding{{ID: "R1-DET", Severity: "CRITICAL"}})
	_, _ = tx.ClassifyEvidence([]FindingEvidence{{FindingID: "R1-DET", Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "failing test"}})
	_ = tx.BeginFix(hash("2"))
	return tx
}

func budgetedAtFixRequired(t *testing.T, originalChangedLines int) *Transaction {
	t.Helper()
	tx, err := NewTransaction(Start{
		LineageID: "budgeted-lineage", Mode: ModeOrdinaryBounded, Generation: 1,
		Snapshot: newTestTransaction(t, ModeOrdinary4R).Snapshot, PolicyHash: hash("d"),
		RiskLevel: RiskMedium, SelectedLenses: []string{LensReliability}, OriginalChangedLines: &originalChangedLines,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	finding := Finding{ID: "REL-001", Lens: "reliability", Location: "internal/example.go:1", Severity: "CRITICAL", Claim: "candidate regression", ProofRefs: []string{"focused test failed"}}
	if err := tx.RecordLensResult(LensResult{Lens: LensReliability, Findings: []Finding{finding}, Evidence: []string{"focused test exited 1"}}); err != nil {
		t.Fatal(err)
	}
	if err := freezeTestFindings(tx, []Finding{finding}); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ClassifyEvidence([]FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk causes focused test failure"}}); err != nil {
		t.Fatal(err)
	}
	return tx
}

func newTestTransaction(t *testing.T, mode Mode) *Transaction {
	t.Helper()
	tx, err := NewTransaction(Start{
		LineageID: "lineage-1", Mode: mode, Generation: 1,
		Snapshot: Snapshot{
			Kind: TargetCurrentChanges, BaseTree: tree("a"), CandidateTree: tree("b"),
			PathsDigest: hash("a"), IntendedUntracked: []string{},
			IntendedUntrackedProof: hash("b"), Paths: []string{"internal/example.go"}, Identity: hash("c"),
		},
		PolicyHash: hash("d"),
	})
	if err != nil {
		t.Fatalf("NewTransaction() error = %v", err)
	}
	return tx
}

func hash(char string) string {
	return "sha256:" + strings.Repeat(char, 64)
}

func freezeTestFindings(tx *Transaction, findings []Finding) error {
	ledger, err := CanonicalLedger(findings)
	if err != nil {
		return err
	}
	return tx.FreezeFindings(findings, ledger, "")
}

func tree(char string) string {
	return strings.Repeat(char, 40)
}
