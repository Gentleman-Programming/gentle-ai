package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestCompactReviewBoundsCandidateCausalityToGenesisLocations(t *testing.T) {
	tests := []struct {
		name, location      string
		class               EvidenceClass
		causality           CausalDisposition
		refuter             []EvidenceResult
		wantState           State
		wantCausality       CausalDisposition
		wantOutcome         EvidenceOutcome
		wantFix, wantFollow bool
		wantRefuterRequired bool
	}{
		{name: "missing location", causality: CausalIntroduced, class: EvidenceDeterministic, wantState: StateEscalated, wantCausality: CausalUnknown, wantOutcome: OutcomeInconclusive},
		{name: "malformed location", location: "tracked.txt", causality: CausalBehaviorActivated, class: EvidenceDeterministic, wantState: StateEscalated, wantCausality: CausalUnknown, wantOutcome: OutcomeInconclusive},
		{name: "absolute location", location: "/tracked.txt:1", causality: CausalWorsened, class: EvidenceDeterministic, wantState: StateEscalated, wantCausality: CausalUnknown, wantOutcome: OutcomeInconclusive},
		{name: "windows absolute location", location: "C:/tracked.txt:1", causality: CausalWorsened, class: EvidenceDeterministic, wantState: StateEscalated, wantCausality: CausalUnknown, wantOutcome: OutcomeInconclusive},
		{name: "traversal location", location: "../tracked.txt:1", causality: CausalIntroduced, class: EvidenceDeterministic, wantState: StateEscalated, wantCausality: CausalUnknown, wantOutcome: OutcomeInconclusive},
		{name: "outside genesis", location: "legacy/unsafe.go:5", causality: CausalBehaviorActivated, class: EvidenceDeterministic, wantState: StateEscalated, wantCausality: CausalUnknown, wantOutcome: OutcomeInconclusive},
		{name: "in genesis introduced", location: "tracked.txt:1", causality: CausalIntroduced, class: EvidenceDeterministic, wantState: StateCorrectionRequired, wantCausality: CausalIntroduced, wantOutcome: OutcomeCorroborated, wantFix: true},
		{name: "pre-existing remains follow-up", location: "legacy/unsafe.go:5", causality: CausalPreExisting, class: EvidenceDeterministic, wantState: StateValidating, wantCausality: CausalPreExisting, wantOutcome: OutcomeInfo, wantFollow: true},
		{name: "base-only remains follow-up", location: "legacy/unsafe.go:5", causality: CausalBaseOnly, class: EvidenceDeterministic, wantState: StateValidating, wantCausality: CausalBaseOnly, wantOutcome: OutcomeInfo, wantFollow: true},
		{name: "explicit unknown remains escalated", location: "tracked.txt:1", causality: CausalUnknown, class: EvidenceDeterministic, wantState: StateEscalated, wantCausality: CausalUnknown, wantOutcome: OutcomeInconclusive},
		{name: "in genesis inferential requires refuter", location: "tracked.txt:1", causality: CausalIntroduced, class: EvidenceInferential, wantRefuterRequired: true},
		{name: "in genesis inferential uses one refuter", location: "tracked.txt:1", causality: CausalIntroduced, class: EvidenceInferential, refuter: []EvidenceResult{{FindingID: "R3-001", Outcome: OutcomeCorroborated, Proof: "independent reproduction"}}, wantState: StateCorrectionRequired, wantCausality: CausalIntroduced, wantOutcome: OutcomeCorroborated, wantFix: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
			state := newCompactTestState(t, repo, "causal-scope")
			finding := Finding{ID: "R3-001", Lens: "reliability", Location: tt.location, Severity: "CRITICAL", Claim: "observable failure", ProofRefs: []string{"defect reproduced"}}
			err := state.CompleteReview(CompactReviewInput{
				LensResults:     []LensResult{{Lens: LensReliability, Findings: []Finding{finding}, Evidence: []string{"reviewed exact candidate"}}},
				Classifications: []FindingEvidence{{FindingID: finding.ID, Class: tt.class, Causality: tt.causality, Proof: "defect reproduced"}},
				RefuterOutcomes: tt.refuter,
			})
			if tt.wantRefuterRequired {
				if err == nil {
					t.Fatal("inferential finding without refuter was accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			classification := state.Classifications[finding.ID]
			if state.State != tt.wantState || classification.Causality != tt.wantCausality || state.Outcomes[finding.ID] != tt.wantOutcome || (len(state.FixFindingIDs) == 1) != tt.wantFix || (len(state.FollowUps) == 1) != tt.wantFollow {
				t.Fatalf("routing = state %q, causality %q, outcome %q, fixes %v, follow-ups %v", state.State, classification.Causality, state.Outcomes[finding.ID], state.FixFindingIDs, state.FollowUps)
			}
		})
	}
}

func TestCompactStoreLoadsHistoricalOutOfGenesisCausalityWithoutRewrite(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	store, record, payload := persistHistoricalOutOfGenesisCompactState(t, repo, "historical-causal-scope")
	loaded, loadErr := store.Load()
	after, readErr := os.ReadFile(store.StatePath())
	if loadErr != nil || readErr != nil || !bytes.Equal(after, payload) || loaded.Revision != record.Revision {
		t.Fatalf("historical record changed: load=%v read=%v loaded=%q want=%q", loadErr, readErr, loaded.Revision, record.Revision)
	}
	transport := CompactTransport{Schema: CompactTransportSchema, Record: record}
	transport.BundleDigest = compactTransportDigest(transport)
	transportPayload, _ := json.Marshal(transport)
	if parsedTransport, err := ParseCompactTransport(transportPayload); err != nil || parsedTransport.Record.Revision != record.Revision || parsedTransport.BundleDigest != transport.BundleDigest {
		t.Fatalf("historical transport changed: %#v, %v", parsedTransport, err)
	}
}

func TestCompactStartIgnoresUnrelatedHistoricalOutOfGenesisCausality(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	historicalStore, _, historicalPayload := persistHistoricalOutOfGenesisCompactState(t, repo, "historical-causal-scope")
	writeSnapshotFile(t, repo, "tracked.txt", "base\n")
	writeSnapshotFile(t, repo, "deleted.txt", "advanced base\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt", "deleted.txt")
	gitSnapshot(t, repo, "commit", "-m", "advance unrelated base")
	writeSnapshotFile(t, repo, "deleted.txt", "new unrelated target\n")
	requested := newCompactTestState(t, repo, "new-causal-scope")
	started, startErr := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
	after, readErr := os.ReadFile(historicalStore.StatePath())
	if startErr != nil || readErr != nil || !bytes.Equal(after, historicalPayload) || started.Action != CompactStartCreated || started.Record.State.LineageID != requested.LineageID {
		t.Fatalf("unrelated START = %#v, start=%v read=%v bytes changed=%v", started, startErr, readErr, !bytes.Equal(after, historicalPayload))
	}
}

func persistHistoricalOutOfGenesisCompactState(t *testing.T, repo, lineage string) (CompactStore, CompactRecord, []byte) {
	t.Helper()
	state := newCompactTestState(t, repo, lineage)
	finding := Finding{ID: "R3-001", Lens: "reliability", Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "observable failure", ProofRefs: []string{"candidate failure"}}
	if err := state.CompleteReview(CompactReviewInput{
		LensResults:     []LensResult{{Lens: LensReliability, Findings: []Finding{finding}, Evidence: []string{"reviewed exact candidate"}}},
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "candidate failure"}},
	}); err != nil {
		t.Fatal(err)
	}
	state.Findings[0].Location = "legacy/unsafe.go:1"
	state.LensResults[0].Findings[0].Location = state.Findings[0].Location
	state.LensResults[0].ResultHash = LensResultHash(state.LensResults[0])
	store, storeErr := CompactAuthoritativeStore(context.Background(), repo, lineage)
	record, payload, recordErr := makeCompactRecord(state)
	if storeErr != nil || recordErr != nil {
		t.Fatalf("build historical record: store=%v record=%v", storeErr, recordErr)
	}
	if err := writeAtomic(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	return store, record, payload
}
