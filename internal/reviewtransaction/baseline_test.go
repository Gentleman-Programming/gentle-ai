package reviewtransaction

import "testing"

func TestDeterministicSevereFindingsRequireCandidateCausalityForCorrection(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		severity    string
		claim       string
		causality   CausalDisposition
		proof       string
		wantState   State
		wantFix     bool
		wantOutcome EvidenceOutcome
	}{
		{
			name: "pre-existing same-file defect", id: "R3-PREEXISTING", severity: "CRITICAL",
			claim:     "the same defect exists in an unchanged path inside a candidate-touched file",
			causality: CausalPreExisting, proof: "the same failing test reproduces on base and candidate",
			wantState: StateReadyFinalVerification, wantOutcome: OutcomeInfo,
		},
		{
			name: "behavior activated by candidate", id: "R3-ACTIVATED", severity: "BLOCKER",
			claim:     "the candidate creates a new execution path to the defect",
			causality: CausalBehaviorActivated, proof: "the differential test passes on base and fails on candidate",
			wantState: StateFixRequired, wantFix: true, wantOutcome: OutcomeCorroborated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := newTestTransaction(t, ModeOrdinary4R)
			if err := tx.StartReview(); err != nil {
				t.Fatal(err)
			}
			finding := Finding{
				ID:       tt.id,
				Location: "internal/example.go:12",
				Severity: tt.severity,
				Claim:    tt.claim,
			}
			if err := freezeTestFindings(tx, []Finding{finding}); err != nil {
				t.Fatal(err)
			}

			route, err := tx.ClassifyEvidence([]FindingEvidence{{
				FindingID: finding.ID,
				Class:     EvidenceDeterministic,
				Causality: tt.causality,
				Proof:     tt.proof,
			}})
			if err != nil {
				t.Fatalf("ClassifyEvidence() error = %v", err)
			}
			wantIDs := []string{}
			if tt.wantFix {
				wantIDs = []string{finding.ID}
			}
			if tx.State != tt.wantState || !equalStrings(tx.FixFindingIDs, wantIDs) || !equalStrings(route.AutoFixFindingIDs, wantIDs) || tx.Outcomes[finding.ID] != tt.wantOutcome {
				t.Fatalf("causal route = state %q ids %v route %#v outcome %q", tx.State, tx.FixFindingIDs, route, tx.Outcomes[finding.ID])
			}
			if tt.causality == CausalPreExisting && (len(tx.FollowUps) != 1 || tx.FollowUps[0].Observation != finding.Claim || !equalStrings(tx.FollowUps[0].ProofRefs, []string{tt.proof})) {
				t.Fatalf("pre-existing finding was not preserved as follow-up: %#v", tx.FollowUps)
			}
		})
	}
}

func TestCurrentLifecycleOperationCountBaseline(t *testing.T) {
	tests := []struct {
		name              string
		recordsLensResult bool
		withCorrection    bool
		want              int
	}{
		{name: "ordinary 4R happy path through pre-PR", want: 17},
		{name: "bounded one-lens happy path through pre-PR", recordsLensResult: true, want: 18},
		{name: "ordinary 4R one-correction path through pre-PR", withCorrection: true, want: 21},
		{name: "bounded one-lens one-correction path through pre-PR", recordsLensResult: true, withCorrection: true, want: 22},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sequence := canonicalLifecycleSequence(tt.recordsLensResult, tt.withCorrection)
			if len(sequence) != tt.want {
				t.Fatalf("canonical lifecycle operation count = %d, want %d\nsequence: %v", len(sequence), tt.want, sequence)
			}
		})
	}
}

func canonicalLifecycleSequence(recordsLensResult, withCorrection bool) []string {
	sequence := []string{
		"preserve-preimages",
		"review-start",
	}
	if recordsLensResult {
		sequence = append(sequence, "record-lens-result")
	}
	sequence = append(sequence, "freeze-findings", "classify-evidence")
	if withCorrection {
		sequence = append(sequence,
			"begin-fix",
			"apply-correction",
			"complete-fix",
			"validate-fix",
		)
	}
	return append(sequence,
		"review-resume-preterminal",
		"begin-final-verification",
		"independent-final-verification",
		"complete-final-verification",
		"review-resume-terminal",
		"review-bundle-export",
		"extract-terminal-receipt",
		"construct-post-apply-gate-request",
		"review-validate-post-apply",
		"reconcile-terminal-mirrors",
		"review-validate-pre-commit",
		"review-validate-pre-push",
		"review-validate-pre-pr",
	)
}
