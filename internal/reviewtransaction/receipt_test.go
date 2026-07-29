package reviewtransaction

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReceiptDistinguishesReviewedAndFinalTreesAndValidatesExactGateInputs(t *testing.T) {
	tx := ordinaryAtFixValidation(t)
	if err := tx.ValidateFixDelta([]string{"R1-DET"}, true); err != nil {
		t.Fatalf("ValidateFixDelta() error = %v", err)
	}
	if err := tx.BeginFinalVerification(); err != nil {
		t.Fatalf("BeginFinalVerification() error = %v", err)
	}
	if err := tx.CompleteFinalVerification(hash("5"), true); err != nil {
		t.Fatalf("CompleteFinalVerification() error = %v", err)
	}
	receipt, err := tx.Receipt()
	if err != nil {
		t.Fatalf("Receipt() error = %v", err)
	}
	if receipt.InitialReviewTree == receipt.FinalCandidateTree || receipt.FixDeltaHash == "" {
		t.Fatalf("receipt mislabeled post-fix content: %#v", receipt)
	}

	context := GateContext{
		Gate: GatePrePR, LineageID: receipt.LineageID, Generation: receipt.Generation,
		BaseTree: receipt.BaseTree, CandidateTree: receipt.FinalCandidateTree,
		PathsDigest: receipt.PathsDigest, FixDeltaHash: receipt.FixDeltaHash,
		PolicyHash: receipt.PolicyHash, LedgerHash: receipt.LedgerHash, EvidenceHash: receipt.EvidenceHash,
		BaseRelationshipValid: true,
	}
	for _, gate := range []GateKind{GatePostApply, GatePreCommit, GatePrePush, GatePrePR} {
		context.Gate = gate
		if got := validateDerivedGate(receipt, context); got != GateAllow {
			t.Fatalf("validateDerivedGate(%s exact) = %q, want allow", gate, got)
		}
	}
	context.Gate = GateRelease
	if got := validateDerivedGate(receipt, context); got != GateInvalidated {
		t.Fatalf("validateDerivedGate(release without publication boundary) = %q, want invalidated", got)
	}
	context.Gate = GatePrePR
	changed := context
	changed.CandidateTree = tree("f")
	if got := validateDerivedGate(receipt, changed); got != GateScopeChanged {
		t.Fatalf("validateDerivedGate(scope change) = %q, want scope-changed", got)
	}
	invalid := context
	invalid.PolicyHash = hash("f")
	if got := validateDerivedGate(receipt, invalid); got != GateInvalidated {
		t.Fatalf("validateDerivedGate(policy change) = %q, want invalidated", got)
	}
	escalating := context
	escalating.ExternalEvidence = ExternalEvidenceEscalating
	if got := validateDerivedGate(receipt, escalating); got != GateEscalated {
		t.Fatalf("validateDerivedGate(new failure evidence) = %q, want escalated", got)
	}
}

func TestReceiptParserIsStrictAndTerminalVocabularyIsClosed(t *testing.T) {
	tx := newTestTransaction(t, ModeOrdinary4R)
	_ = tx.StartReview()
	_ = freezeTestFindings(tx, []Finding{})
	_, _ = tx.ClassifyEvidence(nil)
	_ = tx.BeginFinalVerification()
	_ = tx.CompleteFinalVerification(hash("2"), true)
	receipt, err := tx.Receipt()
	if err != nil {
		t.Fatalf("Receipt() error = %v", err)
	}
	payload, _ := json.Marshal(receipt)
	if _, err := ParseReceipt(payload); err != nil {
		t.Fatalf("ParseReceipt() error = %v", err)
	}
	withUnknown := strings.Replace(string(payload), "{", `{"unknown":true,`, 1)
	if _, err := ParseReceipt([]byte(withUnknown)); err == nil {
		t.Fatal("ParseReceipt() accepted an unknown field")
	}
	pending := strings.Replace(string(payload), `"terminal_state":"approved"`, `"terminal_state":"pending"`, 1)
	if _, err := ParseReceipt([]byte(pending)); err == nil {
		t.Fatal("ParseReceipt() accepted a non-terminal receipt state")
	}
}

func TestReceiptRemainsCompatibleWithoutCausalRoutingFields(t *testing.T) {
	tx := newTestTransaction(t, ModeOrdinary4R)
	_ = tx.StartReview()
	_ = freezeTestFindings(tx, []Finding{{ID: "R1-PRE", Severity: "CRITICAL", Claim: "pre-existing defect"}})
	_, _ = tx.ClassifyEvidence([]FindingEvidence{{
		FindingID: "R1-PRE", Class: EvidenceDeterministic, Causality: CausalPreExisting, Proof: "same failure on base and candidate",
	}})
	_ = tx.BeginFinalVerification()
	_ = tx.CompleteFinalVerification(hash("2"), true)
	receipt, err := tx.Receipt()
	if err != nil {
		t.Fatalf("Receipt() error = %v", err)
	}
	payload, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "causal_disposition") || strings.Contains(string(payload), "follow_ups") {
		t.Fatalf("causal routing expanded receipt schema: %s", payload)
	}
	if _, err := ParseReceipt(payload); err != nil {
		t.Fatalf("ParseReceipt() error = %v", err)
	}
}

func TestNewLineageRequiresExplicitDifferentIdentity(t *testing.T) {
	start := Start{LineageID: "lineage-1", Mode: ModeOrdinary4R, Generation: 1, Snapshot: newTestTransaction(t, ModeOrdinary4R).Snapshot, PolicyHash: hash("d")}
	if _, err := NewLineage("lineage-1", start); err == nil {
		t.Fatal("NewLineage() silently reused the exhausted lineage ID")
	}
	start.LineageID = "lineage-2"
	if _, err := NewLineage("lineage-1", start); err != nil {
		t.Fatalf("NewLineage(explicit new ID) error = %v", err)
	}
}

func TestReleaseGateRequiresCompleteImmutablePublicationBoundary(t *testing.T) {
	tx := newTestTransaction(t, ModeOrdinary4R)
	_ = tx.StartReview()
	_ = freezeTestFindings(tx, []Finding{})
	_, _ = tx.ClassifyEvidence([]FindingEvidence{})
	release := ReleaseEvidence{
		ReleaseTree:             tx.FinalCandidateTree,
		ConfigurationHash:       hash("2"),
		GeneratedArtifactHash:   hash("3"),
		ProvenanceHash:          hash("4"),
		PublicationBoundaryHash: hash("5"),
		PublicationState:        PublicationStateSealed,
		EvidenceFreshnessHash:   hash("6"),
		EvidenceFreshnessState:  EvidenceFreshnessCurrent,
	}
	if err := tx.BindReleaseEvidence(release); err != nil {
		t.Fatalf("BindReleaseEvidence() error = %v", err)
	}
	_ = tx.BeginFinalVerification()
	_ = tx.CompleteFinalVerification(hash("7"), true)
	receipt, err := tx.Receipt()
	if err != nil {
		t.Fatal(err)
	}

	generic := GateContext{
		Gate: GateRelease, LineageID: receipt.LineageID, Generation: receipt.Generation,
		BaseTree: receipt.BaseTree, CandidateTree: receipt.FinalCandidateTree,
		PathsDigest: receipt.PathsDigest, FixDeltaHash: receipt.FixDeltaHash,
		PolicyHash: receipt.PolicyHash, LedgerHash: receipt.LedgerHash,
		EvidenceHash: receipt.EvidenceHash, BaseRelationshipValid: true,
	}
	if got := validateDerivedGate(receipt, generic); got != GateInvalidated {
		t.Fatalf("validateDerivedGate(generic release context) = %q, want invalidated", got)
	}

	exact := generic
	exact.Release = &release
	if got := validateDerivedGate(receipt, exact); got != GateAllow {
		t.Fatalf("validateDerivedGate(exact release evidence) = %q, want allow", got)
	}
	changed := exact
	changed.Release = &ReleaseEvidence{
		ReleaseTree:             release.ReleaseTree,
		ConfigurationHash:       release.ConfigurationHash,
		GeneratedArtifactHash:   release.GeneratedArtifactHash,
		ProvenanceHash:          release.ProvenanceHash,
		PublicationBoundaryHash: hash("8"),
		PublicationState:        release.PublicationState,
		EvidenceFreshnessHash:   release.EvidenceFreshnessHash,
		EvidenceFreshnessState:  release.EvidenceFreshnessState,
	}
	if got := validateDerivedGate(receipt, changed); got != GateInvalidated {
		t.Fatalf("validateDerivedGate(changed publication boundary) = %q, want invalidated", got)
	}
}

func TestJudgmentDayReceiptCarriesTwoJudgeProof(t *testing.T) {
	tx := newTestTransaction(t, ModeJudgmentDay)
	_ = tx.StartReview()
	if err := tx.RecordJudgeProofs([]JudgeProof{
		{JudgeID: "judge-a", ExecutionHash: hash("1"), ResultHash: hash("2"), Blind: true, Confirmed: true},
		{JudgeID: "judge-b", ExecutionHash: hash("3"), ResultHash: hash("4"), Blind: true, Confirmed: true},
	}, hash("5")); err != nil {
		t.Fatal(err)
	}
	_ = freezeTestFindings(tx, []Finding{})
	_, _ = tx.ClassifyEvidence([]FindingEvidence{})
	_ = tx.BeginFinalVerification()
	_ = tx.CompleteFinalVerification(hash("7"), true)
	receipt, err := tx.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if receipt.JudgeProofHash != tx.JudgeProofHash || receipt.Counters.JudgeExecutions != 2 {
		t.Fatalf("receipt judge proof = %q counters=%#v", receipt.JudgeProofHash, receipt.Counters)
	}
	withoutProof := receipt
	withoutProof.JudgeProofHash = ""
	if payload, err := json.Marshal(withoutProof); err != nil {
		t.Fatal(err)
	} else if _, err := ParseReceipt(payload); err == nil {
		t.Fatal("ParseReceipt() accepted Judgment Day approval without judge proof")
	}
}
