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

// validBaseAdvanceCore builds a syntactically-valid BaseAdvanceCompatibility
// core (the five content-check fields every status shares) so each test below
// only has to vary Status/CIStatus/CI fields, the one axis
// baseAdvanceStatusAllowedForGate (receipt.go) actually polices.
func validBaseAdvanceCore(originalBase, newBase string) BaseAdvanceCompatibility {
	digest := "sha256:" + strings.Repeat("a", 64)
	return BaseAdvanceCompatibility{
		Compatible: true, OriginalMergeBaseTree: originalBase, NewBaseTree: newBase,
		OriginalPatchIdentity: digest, DeliveredPatchIdentity: digest,
		DeliveredPathsDigest: digest, BaseAdvancePathsDigest: digest, PathsDisjoint: true,
		MergedResultTree: newBase,
	}
}

// TestValidateDerivedGateGeneralizesCompatibleBaseAdvanceToLocalGates pins D2
// (#2471 option a, filed against #2388): the compatibleAdvance carve-out
// (receipt.go's validateDerivedGate) used to consult context.BaseAdvance only
// when context.Gate == GatePrePR. It must now consult it at GatePreCommit and
// GatePrePush too, using the unattested baseAdvanceCompatibleLocalStatus proof
// deriveBaseAdvanceCompatibility(requireAttestation=false) issues, and still
// allow despite BaseTree/CandidateTree/PathsDigest diverging from the receipt
// -- exactly the diagnostic #2388 reported as a hard pre-commit/pre-push
// refusal with no recovery path.
func TestValidateDerivedGateGeneralizesCompatibleBaseAdvanceToLocalGates(t *testing.T) {
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
	newBase := tree("e")
	advance := validBaseAdvanceCore(receipt.BaseTree, newBase)
	advance.Status, advance.CIStatus = baseAdvanceCompatibleLocalStatus, currentChangesBoundaryCIStatus
	if !advance.valid() {
		t.Fatalf("local advance proof is not valid: %#v", advance)
	}
	for _, gate := range []GateKind{GatePreCommit, GatePrePush} {
		context := GateContext{
			Gate: gate, LineageID: receipt.LineageID, Generation: receipt.Generation,
			BaseTree: newBase, CandidateTree: receipt.FinalCandidateTree,
			PathsDigest: receipt.PathsDigest, FixDeltaHash: receipt.FixDeltaHash,
			PolicyHash: receipt.PolicyHash, LedgerHash: receipt.LedgerHash, EvidenceHash: receipt.EvidenceHash,
			BaseRelationshipValid: false, BaseAdvance: &advance,
		}
		if got := validateDerivedGate(receipt, context); got != GateAllow {
			t.Fatalf("validateDerivedGate(%s, compatible local base advance) = %q, want allow", gate, got)
		}
	}
}

// TestValidateDerivedGateRefusesLocalStatusAtPrePRRegression is the byte-strict
// regression guard D2 requires: the unattested local proof must never
// authorize GatePrePR, even when every content-check field matches, because
// GatePrePR alone can reach real CI and D2 keeps its attestation requirement
// exactly as strict as before this change.
func TestValidateDerivedGateRefusesLocalStatusAtPrePRRegression(t *testing.T) {
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
	newBase := tree("e")
	advance := validBaseAdvanceCore(receipt.BaseTree, newBase)
	advance.Status, advance.CIStatus = baseAdvanceCompatibleLocalStatus, currentChangesBoundaryCIStatus
	context := GateContext{
		Gate: GatePrePR, LineageID: receipt.LineageID, Generation: receipt.Generation,
		BaseTree: newBase, CandidateTree: receipt.FinalCandidateTree,
		PathsDigest: receipt.PathsDigest, FixDeltaHash: receipt.FixDeltaHash,
		PolicyHash: receipt.PolicyHash, LedgerHash: receipt.LedgerHash, EvidenceHash: receipt.EvidenceHash,
		BaseRelationshipValid: false, BaseAdvance: &advance,
	}
	if got := validateDerivedGate(receipt, context); got == GateAllow {
		t.Fatalf("validateDerivedGate(pre-pr, unattested local advance) = %q, want refused", got)
	}
	if baseAdvanceStatusAllowedForGate(GatePrePR, baseAdvanceCompatibleLocalStatus) {
		t.Fatal("baseAdvanceStatusAllowedForGate(pre-pr, local) = true, want false")
	}
	for _, gate := range []GateKind{GatePreCommit, GatePrePush} {
		if baseAdvanceStatusAllowedForGate(gate, baseAdvanceCompatibleStatus) {
			t.Fatalf("baseAdvanceStatusAllowedForGate(%s, attested) = true, want false", gate)
		}
	}
}

// TestParseGateContextRestrictsBaseAdvanceStatusPerGate proves the same
// byte-strict split at the persisted-context boundary: ParseGateContext must
// accept the local status only at GatePreCommit/GatePrePush, accept the
// attested (or current-changes-boundary) status only at GatePrePR, and a
// historical context with no base_advanced_compatible field at all -- every
// context ever persisted before this change -- must still parse unchanged
// (the additive-only, forensic-invariant requirement).
func TestParseGateContextRestrictsBaseAdvanceStatusPerGate(t *testing.T) {
	digest := "sha256:" + strings.Repeat("d", 64)
	base := strings.Repeat("a", 40)
	newBase := strings.Repeat("b", 40)
	candidate := strings.Repeat("c", 40)
	coreFields := `"compatible":true,"old_base_tree":"` + base +
		`","new_base_tree":"` + newBase + `","original_patch_identity":"` + digest + `","delivered_patch_identity":"` + digest +
		`","delivered_paths_digest":"` + digest + `","base_advance_paths_digest":"` + digest + `","paths_disjoint":true,"merged_result_tree":"` + newBase + `"`
	// localJSON has no CI fields (ci_status is the shared "not-required"
	// sentinel both baseAdvanceCompatibleLocalStatus and
	// currentChangesBoundaryCompatibleStatus use). attestedJSON adds the three
	// CI fields baseAdvanceCompatibleStatus's own valid() requires, so a
	// "does this status parse at this gate" case is never confused with "is
	// this proof's own shape valid at all".
	localJSON := `,"base_advanced_compatible":{"status":"STATUS",` + coreFields + `,"ci_status":"not-required"}`
	attestedJSON := `,"base_advanced_compatible":{"status":"` + baseAdvanceCompatibleStatus + `",` + coreFields +
		`,"ci_attestation_artifact_hash":"` + digest + `","ci_attestation_issuer":"trusted-ci","ci_status":"success"}`
	baseContext := func(gate GateKind) string {
		return `{"gate":"` + string(gate) + `","lineage_id":"lineage-one","generation":1,"store_revision":"` + digest +
			`","genesis_revision":"` + digest + `","chain_identity":"` + digest + `","bundle_digest":"` + digest +
			`","base_tree":"` + newBase + `","candidate_tree":"` + candidate + `","paths_digest":"` + digest +
			`","fix_delta_hash":"` + digest + `","policy_hash":"` + digest + `","ledger_hash":"` + digest +
			`","evidence_hash":"` + digest + `","base_relationship_valid":false`
	}
	withLocal := func(gate GateKind) string {
		return baseContext(gate) + strings.Replace(localJSON, "STATUS", baseAdvanceCompatibleLocalStatus, 1) + "}"
	}
	withAttested := func(gate GateKind) string { return baseContext(gate) + attestedJSON + "}" }
	withoutAdvance := func(gate GateKind) string { return baseContext(gate) + "}" }

	for _, tt := range []struct {
		name    string
		payload string
		wantErr bool
	}{
		{"local status at pre-commit parses", withLocal(GatePreCommit), false},
		{"local status at pre-push parses", withLocal(GatePrePush), false},
		{"attested status at pre-pr parses", withAttested(GatePrePR), false},
		{"local status at pre-pr is rejected (byte-strict regression)", withLocal(GatePrePR), true},
		{"attested status at pre-commit is rejected", withAttested(GatePreCommit), true},
		{"attested status at pre-push is rejected", withAttested(GatePrePush), true},
		{"historical context without base advance still parses at pre-commit", withoutAdvance(GatePreCommit), false},
		{"historical context without base advance still parses at pre-push", withoutAdvance(GatePrePush), false},
		{"historical context without base advance still parses at pre-pr", withoutAdvance(GatePrePR), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseGateContext([]byte(tt.payload))
			if tt.wantErr && err == nil {
				t.Fatalf("ParseGateContext(%s) accepted an invalid base-advance/gate pairing", tt.name)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ParseGateContext(%s) error = %v, want nil", tt.name, err)
			}
		})
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
