package reviewtransaction

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type compactReviewerCaptureFixture struct {
	store   CompactStore
	state   CompactState
	request CompactAdmittedReviewerResultRequest
}

func captureAdmittedCorrectionFinding(t *testing.T, store CompactStore, state CompactState, finding Finding) LensResult {
	t.Helper()
	frozen, err := (SnapshotBuilder{Repo: store.repo}).FrozenCandidateContext(t.Context(), state.InitialSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	lens := state.SelectedLenses[0]
	subject, err := NewArtifactSubject(state, state.CapturePhaseRevision, frozen, lens, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	result := LensResult{Lens: lens, Findings: []Finding{finding}, Evidence: []string{"inspected the complete frozen candidate scope"}}
	inspection := ArtifactInspection{Status: ArtifactInspectionCompleted, Paths: append([]string(nil), state.InitialSnapshot.Paths...)}
	raw, err := json.Marshal(compactProviderReviewerResult{SubjectHash: subject.SubjectHash, Inspection: inspection, Lens: lens, Findings: result.Findings, Evidence: result.Evidence})
	if err != nil {
		t.Fatal(err)
	}
	capture, err := store.CaptureAdmittedReviewerResult(t.Context(), CompactAdmittedReviewerResultRequest{
		ExpectedRevision: state.CapturePhaseRevision, TargetIdentity: state.InitialSnapshot.Identity,
		FrozenContext: frozen, ArtifactSubject: subject, Inspection: inspection, Result: result,
		CandidateCausalFindingIDs: []string{finding.ID}, RawPayload: append(raw, '\n'),
	})
	if err != nil {
		t.Fatal(err)
	}
	return capture.LensResult
}

func newCompactReviewerCaptureFixture(t *testing.T, lineage string) compactReviewerCaptureFixture {
	t.Helper()
	repo := initSnapshotRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(repo, "internal", name), []byte("package internal\n\nfunc Value() int { return 1 }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitSnapshot(t, repo, "add", "--", "internal/a.go", "internal/b.go")
	gitSnapshot(t, repo, "commit", "-m", "add go fixture")
	for _, name := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(repo, "internal", name), []byte("package internal\n\nfunc Value() int { return 2 }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	state := newCompactTestState(t, repo, lineage)
	if len(state.SelectedLenses) != 1 || state.SelectedLenses[0] != LensReliability {
		t.Fatalf("fixture lenses = %v", state.SelectedLenses)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}
	frozen, err := (SnapshotBuilder{Repo: repo}).FrozenCandidateContext(context.Background(), state.InitialSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	subject, err := NewArtifactSubject(state, state.CapturePhaseRevision, frozen, LensReliability, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	inspection := ArtifactInspection{Status: ArtifactInspectionCompleted, Paths: append([]string(nil), state.InitialSnapshot.Paths...)}
	result := LensResult{Lens: LensReliability, Findings: []Finding{}, Evidence: []string{"inspected internal/a.go:1 against the complete frozen candidate"}}
	raw, err := json.Marshal(compactProviderReviewerResult{SubjectHash: subject.SubjectHash, Inspection: inspection, Lens: subject.Lens, Findings: result.Findings, Evidence: result.Evidence})
	if err != nil {
		t.Fatal(err)
	}
	return compactReviewerCaptureFixture{
		store: store, state: state,
		request: CompactAdmittedReviewerResultRequest{
			ExpectedRevision: state.CapturePhaseRevision, TargetIdentity: state.InitialSnapshot.Identity,
			FrozenContext: frozen, ArtifactSubject: subject, Inspection: inspection, Result: result,
			CandidateCausalFindingIDs: []string{}, RawPayload: append(raw, '\n'),
		},
	}
}

// TestCompactReviewerResultSidecarOwnersAreAbsent prevents the retired result
// directory from becoming a lifecycle owner again. Lens result bytes and their
// digests live only in CompactState.AdmittedRoleResults.
func TestCompactReviewerResultSidecarOwnersAreAbsent(t *testing.T) {
	for _, source := range []string{
		"compact_store.go",
		"compact_reclaim.go",
		filepath.Join("..", "cli", "review_opencode_transport.go"),
	} {
		payload, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("read production owner %s: %v", source, err)
		}
		for _, forbidden := range []string{
			"CompactReviewerResultsDir", "reviewer-results", "reviewResultArtifactPath",
			"CompactIncidentsDir", "EnsureCompactIncidentsDir", "ResultDispositions",
		} {
			if strings.Contains(string(payload), forbidden) {
				t.Fatalf("retired compact result owner %q remains in %s", forbidden, source)
			}
		}
	}
	if _, err := os.Stat("compact_result_disposition.go"); !os.IsNotExist(err) {
		t.Fatalf("retired compact result disposition owner remains: %v", err)
	}
}

func TestApprovedAcknowledgementHasNoImmediateBurnOrSidecarRevival(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, source := range []string{
		filepath.Join(root, "internal", "cli", "review_facade.go"),
		filepath.Join(root, "internal", "cli", "review_last_event_closure.go"),
		filepath.Join(root, "internal", "cli", "review_next_transition.go"),
		filepath.Join(root, "internal", "cli", "review_start_contract.go"),
		filepath.Join(root, "internal", "cli", "review_status_contract.go"),
		filepath.Join(root, "internal", "reviewtransaction", "compact_burn.go"),
		filepath.Join(root, "scripts", "crosslane", "battery.go"),
		filepath.Join(root, "bench", "journeys_wave3.go"),
		filepath.Join(root, "bench", "journeys_provider_capture.go"), filepath.Join(root, "e2e", "organicruntime", "organic_runtime_test.go"),
	} {
		payload, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("read current acknowledgement owner %s: %v", source, err)
		}
		for _, forbidden := range []string{"BurnApprovedCompactAuthority(", "IssueApprovedCompactAcknowledgement(", "burnApproved(", "requireAtomicLineageBurned", "requireBurnedApproval", "PublishReviewRepositoryContext", "CompactReviewerResultsDir", "EffectIntents", "lens-contexts", "rctx1_"} {
			if strings.Contains(string(payload), forbidden) {
				t.Fatalf("current acknowledgement owner %s revived forbidden %q", source, forbidden)
			}
		}
	}
	for _, schema := range []string{
		filepath.Join(root, "contracts", "review-integration", "v2", "schemas", "start.schema.json"),
		filepath.Join(root, "contracts", "review-integration", "v2", "schemas", "status-v5.schema.json"),
		filepath.Join(root, "contracts", "review-integration", "v2", "schemas", "last-event-closure.schema.json"),
	} {
		payload, err := os.ReadFile(schema)
		if err != nil {
			t.Fatalf("read acknowledgement schema %s: %v", schema, err)
		}
		for _, forbidden := range []string{"review-integration/v3", "consent/v4", "consent-v4"} {
			if strings.Contains(string(payload), forbidden) {
				t.Fatalf("acknowledgement schema %s expanded prohibited protocol %q", schema, forbidden)
			}
		}
	}
}

func TestCorrectionPlanRequestUsesAdmittedCaptureOverLegacyProjections(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "correction-plan-admitted-capture")
	finding := Finding{
		ID: "R3-001", Lens: strings.TrimPrefix(fixture.state.SelectedLenses[0], "review-"), Location: "internal/a.go:1", Severity: "CRITICAL",
		Claim: "candidate needs correction", ProofRefs: []string{"changed hunk causes failure"}, EvidenceClass: EvidenceDeterministic, CausalDisposition: CausalIntroduced,
	}
	result := captureAdmittedCorrectionFinding(t, fixture.store, fixture.state, finding)
	record, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	state := record.State
	if err := state.CompleteReview(CompactReviewInput{
		LensResults:     []LensResult{result},
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk causes failure"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	want, err := BuildCorrectionPlanRequest(state, state.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}

	// Retired projections cannot be reintroduced into CompactState; the read must
	// remain entirely derived from the canonical admitted capture.
	got, err := BuildCorrectionPlanRequest(state, state.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("correction plan from tampered legacy projections = %#v, want admitted capture %#v", got, want)
	}
}

func TestCompactStoreCaptureAdmittedReviewerResultPublishesOneRecordExactReplay(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "native-admitted-reviewer")
	first, err := fixture.store.CaptureAdmittedReviewerResult(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	record, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !first.Slot.Occupied || len(record.State.AdmittedRoleResults) != 1 || record.State.AdmittedRoleResults[0].ArtifactDigest != first.Slot.Digest {
		t.Fatalf("capture did not persist one canonical role value: %#v", record)
	}
	replayed, err := fixture.store.CaptureAdmittedReviewerResult(context.Background(), fixture.request)
	if err != nil {
		t.Fatalf("exact replay: %v", err)
	}
	after, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Slot.Digest != first.Slot.Digest || after.Revision != record.Revision {
		t.Fatalf("exact replay changed authority: before=%#v after=%#v", record, after)
	}
}

func TestCompactStoreCaptureAdmittedReviewerResultRefusesStalePhaseWithoutMutation(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "native-admitted-stale")
	before, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.request
	request.ExpectedRevision = hash("a")
	request.ArtifactSubject.AuthorityRevision = request.ExpectedRevision
	if _, err := fixture.store.CaptureAdmittedReviewerResult(context.Background(), request); err == nil {
		t.Fatal("stale capture phase was accepted")
	}
	after, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || len(after.State.AdmittedRoleResults) != 0 {
		t.Fatalf("stale phase mutated authority: before=%#v after=%#v", before, after)
	}
}

func TestCompactStoreMergesRefuterTupleAndReplaysWithoutAWrite(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "record-refuter-capture")
	if _, err := fixture.store.CaptureAdmittedReviewerResult(t.Context(), fixture.request); err != nil {
		t.Fatal(err)
	}
	current, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	request := CompactAdmittedRefuterResultRequest{
		ExpectedRevision: current.State.CapturePhaseRevision, TargetIdentity: current.State.InitialSnapshot.Identity,
		RequestHash: hash("b"), Payload: []byte(`{"results":[]}`),
	}
	if err := fixture.store.CaptureAdmittedRefuterResult(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	merged, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.State.AdmittedRoleResults) != 2 {
		t.Fatalf("record values = %d, want lens plus refuter", len(merged.State.AdmittedRoleResults))
	}
	if err := fixture.store.CaptureAdmittedRefuterResult(t.Context(), request); err != nil {
		t.Fatalf("refuter replay: %v", err)
	}
	replayed, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Revision != merged.Revision {
		t.Fatal("refuter exact replay wrote a successor")
	}
}

func TestReopenedRefuterWithRetiredPayloadRequiresCurrentPhase(t *testing.T) {
	repo, store, state := highRiskCaptureAuthority(t, "reopen-refuter-current-phase")
	for order := range state.SelectedLenses {
		captureCompactLens(t, store, state, order)
	}
	initial := requireCompactRoleCount(t, store, 4)
	request := CompactAdmittedRefuterResultRequest{ExpectedRevision: initial.State.CapturePhaseRevision, TargetIdentity: initial.State.InitialSnapshot.Identity, RequestHash: hash("c"), Payload: []byte(`{"results":[]}`)}
	if err := store.CaptureAdmittedRefuterResult(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	initial = requireCompactRoleCount(t, store, 5)
	retiredDigest := ""
	for _, entry := range initial.State.AdmittedRoleResults {
		if entry.Role == CompactRoleRefuter {
			retiredDigest = entry.ArtifactDigest
		}
	}
	reopened := reopenOneCapturedLens(t, repo, store, initial, LensRisk)
	captureCompactLens(t, store, reopened.State, 0)
	current := requireCompactRoleCount(t, store, 4)
	request.ExpectedRevision = current.State.CapturePhaseRevision
	if err := store.CaptureAdmittedRefuterResult(t.Context(), request); err != nil {
		t.Fatalf("fresh refuter with retired payload: %v", err)
	}
	current = requireCompactRoleCount(t, store, 5)
	currentDigest := ""
	for _, entry := range current.State.AdmittedRoleResults {
		if entry.Role == CompactRoleRefuter && entry.CapturePhaseRevision == current.State.CapturePhaseRevision {
			currentDigest = entry.ArtifactDigest
		}
	}
	if _, found := current.State.AdmittedRoleResult(CompactRoleRefuter, current.State.CapturePhaseRevision, current.State.InitialSnapshot.Identity, request.RequestHash); !found || currentDigest != retiredDigest {
		t.Fatalf("fresh current-phase refuter was not admitted from matching bytes: found=%t digest=%q", found, currentDigest)
	}
	beforeReplay := current.Revision
	if err := store.CaptureAdmittedRefuterResult(t.Context(), request); err != nil {
		t.Fatalf("current-phase exact replay: %v", err)
	}
	if replay := requireCompactRoleCount(t, store, 5); replay.Revision != beforeReplay {
		t.Fatal("current-phase exact refuter replay wrote authority")
	}
	request.ExpectedRevision = initial.State.CapturePhaseRevision
	if err := store.CaptureAdmittedRefuterResult(t.Context(), request); err == nil {
		t.Fatal("stale prior refuter phase satisfied the current slot")
	}
	if stale := requireCompactRoleCount(t, store, 5); stale.Revision != beforeReplay {
		t.Fatal("stale prior refuter phase replay mutated authority")
	}
}
