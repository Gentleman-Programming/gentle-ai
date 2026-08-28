package reviewtransaction

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type compactReviewerCaptureFixture struct {
	store   CompactStore
	state   CompactState
	request CompactAdmittedReviewerResultRequest
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
		"compact_result_disposition.go",
		filepath.Join("..", "cli", "review_opencode_transport.go"),
	} {
		payload, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("read production owner %s: %v", source, err)
		}
		for _, forbidden := range []string{"CompactReviewerResultsDir", "reviewer-results", "reviewResultArtifactPath"} {
			if strings.Contains(string(payload), forbidden) {
				t.Fatalf("retired reviewer-result sidecar owner %q remains in %s", forbidden, source)
			}
		}
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
