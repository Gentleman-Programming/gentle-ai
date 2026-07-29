package reviewtransaction

import "testing"

func TestIncidentTransactionIsSeparateOnlyWhileImmutableTargetsMatch(t *testing.T) {
	receipt := Receipt{
		Schema: ReceiptSchema, LineageID: "lineage-1", Mode: ModeOrdinary4R, Generation: 1,
		BaseTree: tree("a"), InitialReviewTree: tree("b"), FinalCandidateTree: tree("b"),
		PathsDigest: hash("1"), FixDeltaHash: EmptyFixDeltaHash, PolicyHash: hash("2"),
		LedgerHash: hash("3"), EvidenceHash: hash("4"),
		Counters: Counters{FullReviews: 1, FinalVerifications: 1}, TerminalState: TerminalApproved,
	}
	target := IncidentTarget{
		CodeTree: receipt.FinalCandidateTree, ConfigHash: hash("5"),
		GeneratedArtifactsHash: hash("6"), ProvenanceHash: hash("7"),
	}
	if !CanUseSeparateIncident(receipt, target, target) {
		t.Fatal("unchanged immutable targets could not use a separate incident transaction")
	}
	changed := target
	changed.ProvenanceHash = hash("8")
	if CanUseSeparateIncident(receipt, target, changed) {
		t.Fatal("provenance change incorrectly remained a separate incident transaction")
	}
	changed = target
	changed.CodeTree = tree("c")
	if CanUseSeparateIncident(receipt, target, changed) {
		t.Fatal("code change incorrectly remained a separate incident transaction")
	}
}
