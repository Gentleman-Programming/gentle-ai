package reviewtransaction

import (
	"bytes"
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
			state, store := startReviewingCompactAuthority(t, repo, state)
			finding := Finding{
				ID: "R3-001", Lens: "reliability", Location: tt.location, Severity: "CRITICAL",
				Claim: "observable failure", ProofRefs: []string{"defect reproduced"},
				EvidenceClass: tt.class, CausalDisposition: tt.causality,
			}
			if tt.location != "tracked.txt:1" {
				// Active capture now rejects an invalid or out-of-manifest location
				// before it can become semantic authority. This is stronger than
				// the retired projection's post-capture causality downgrade.
				if _, err := store.CaptureAdmittedReviewerResult(t.Context(), compactLensCaptureRequest(t, store, state, 0, finding)); err == nil {
					t.Fatalf("out-of-scope finding %q was admitted", tt.location)
				}
				record := requireCompactRoleCount(t, store, 0)
				if record.State.CapturePhaseRevision != state.CapturePhaseRevision {
					t.Fatal("rejected capture changed the active retry phase")
				}
				return
			}
			if tt.wantRefuterRequired {
				captureCompactLens(t, store, state, 0, finding)
				record := requireCompactRoleCount(t, store, 1)
				view, err := record.State.CompactReviewView()
				if err != nil {
					t.Fatal(err)
				}
				if err := record.State.CompleteReview(CompactReviewInput{LensResults: view.LensResults}); err == nil {
					t.Fatal("inferential finding without refuter was accepted")
				}
				return
			}
			state, _ = captureAndCompleteCompactReview(t, store, state, CompactReviewInput{
				LensResults:     []LensResult{{Lens: LensReliability, Findings: []Finding{finding}, Evidence: []string{"reviewed exact candidate"}}},
				Classifications: []FindingEvidence{{FindingID: finding.ID, Class: tt.class, Causality: tt.causality, Proof: "defect reproduced"}},
				RefuterOutcomes: tt.refuter,
			})
			view, err := state.CompactReviewView()
			if err != nil {
				t.Fatal(err)
			}
			classification := view.Classifications[finding.ID]
			if state.State != tt.wantState || classification.Causality != tt.wantCausality || view.Outcomes[finding.ID] != tt.wantOutcome || (len(state.FixFindingIDs) == 1) != tt.wantFix || (len(view.FollowUps) == 1) != tt.wantFollow {
				t.Fatalf("routing = state %q, causality %q, outcome %q, fixes %v, follow-ups %v", state.State, classification.Causality, view.Outcomes[finding.ID], state.FixFindingIDs, view.FollowUps)
			}
		})
	}
}

func TestCompactReviewViewDowngradesCandidateCausalityMissingFromAdmission(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "derived-view-admission-causality")
	if _, err := fixture.store.CaptureAdmittedReviewerResult(t.Context(), fixture.request); err != nil {
		t.Fatal(err)
	}
	record := requireCompactRoleCount(t, fixture.store, 1)
	state := record.State
	entry := state.AdmittedRoleResults[0]
	var envelope compactAdmittedReviewerResult
	if err := json.Unmarshal(entry.Value, &envelope); err != nil {
		t.Fatal(err)
	}
	finding := Finding{ID: "R3-001", Lens: LensReliability, Location: "internal/a.go:1", Severity: "CRITICAL", Claim: "candidate defect", ProofRefs: []string{"internal/a.go:1"}, EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalIntroduced}
	result, err := CanonicalCompactLensResult(LensResult{Lens: LensReliability, Findings: []Finding{finding}, Evidence: []string{"internal/a.go:1 inspected"}})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(compactProviderReviewerResult{SubjectHash: envelope.Subject.SubjectHash, Inspection: fixture.request.Inspection, Lens: result.Lens, Findings: result.Findings, Evidence: result.Evidence})
	if err != nil {
		t.Fatal(err)
	}
	envelope.Result = payload
	envelope.Admission.ResultHash = result.ResultHash
	envelope.Admission.RawSHA256 = payloadSHA256(append(append([]byte(nil), payload...), '\n'))
	envelope.Admission.CanonicalSHA256 = envelope.Admission.RawSHA256
	envelope.Admission.CandidateCausalFindingIDs = []string{}
	entry.Value, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	entry.ArtifactDigest = compactPreservedPayloadDigest(append(append([]byte(nil), entry.Value...), '\n'))
	entry.ResultHash = result.ResultHash
	state.AdmittedRoleResults = []CompactAdmittedRoleResult{entry}

	view, err := state.CompactReviewView()
	if err != nil {
		t.Fatal(err)
	}
	if got := view.Classifications[finding.ID].Causality; got != CausalUnknown || view.Outcomes[finding.ID] != OutcomeInconclusive || len(view.FixFindingIDs) != 0 {
		t.Fatalf("unadmitted causal route = classification %#v, outcomes %#v, fixes %v", view.Classifications, view.Outcomes, view.FixFindingIDs)
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

func persistHistoricalOutOfGenesisCompactState(t *testing.T, repo, lineage string) (CompactStore, CompactRecord, []byte) {
	t.Helper()
	state := newCompactTestState(t, repo, lineage)
	state, store := startReviewingCompactAuthority(t, repo, state)
	finding := Finding{
		ID: "R3-001", Lens: "reliability", Location: "tracked.txt:1", Severity: "CRITICAL",
		Claim: "observable failure", ProofRefs: []string{"candidate failure"},
		EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalIntroduced,
	}
	state, _ = captureAndCompleteCompactReview(t, store, state, CompactReviewInput{
		LensResults:     []LensResult{{Lens: LensReliability, Findings: []Finding{finding}, Evidence: []string{"reviewed exact candidate"}}},
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "candidate failure"}},
	})
	record, payload, recordErr := makeCompactRecord(state)
	if recordErr != nil {
		t.Fatalf("build historical record: %v", recordErr)
	}
	if err := writeAtomic(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	return store, record, payload
}
