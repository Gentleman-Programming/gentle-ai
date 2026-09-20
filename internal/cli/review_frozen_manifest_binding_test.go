package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
)

// A capture input without an inlined manifest is bound to the frozen candidate
// through its subject digest: the real STATUS facade publishes the digest it
// re-derived from the repository in `frozen`, every validation path refuses a
// self-consistent subject whose digest differs from it, and an envelope that
// offers a manifest-less capture without that digest is refused outright.
func TestReviewStatusPreservesHistoricalGeneratedManifestBinding(t *testing.T) {
	for _, test := range []struct {
		name     string
		admitted bool
	}{
		{name: "unadmitted", admitted: false},
		{name: "after partial admission", admitted: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			reviewEnabledHome(t)
			repo, store, record := historicalGeneratedReviewingStatusFixture(t)
			legacyFrozen := historicalGeneratedFrozenContext(t, repo, record.State.InitialSnapshot)
			wantSubjects := make([]reviewtransaction.ArtifactSubject, len(record.State.SelectedLenses))
			for order, lens := range record.State.SelectedLenses {
				subject, err := reviewtransaction.NewArtifactSubject(record.State, record.State.CapturePhaseRevision, legacyFrozen, lens, order, "")
				if err != nil {
					t.Fatalf("derive historical subject %s: %v", lens, err)
				}
				wantSubjects[order] = subject
			}

			before := explicitFrozenReviewingStatus(t, repo, record.State.LineageID)
			assertHistoricalPendingSubject(t, before, wantSubjects[0], "before admission")
			if !test.admitted {
				after := explicitFrozenReviewingStatus(t, repo, record.State.LineageID)
				assertHistoricalPendingSubject(t, after, wantSubjects[0], "unadmitted after reconstruction")
				return
			}

			lens, order := record.State.SelectedLenses[0], 0
			result := admittedReviewerResultForTest(t, repo, record, lens, order)
			result.SubjectHash = wantSubjects[0].SubjectHash
			payload, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			input := filepath.Join(t.TempDir(), "historical-admitted-result.json")
			if err := os.WriteFile(input, payload, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := RunReviewCaptureResult([]string{
				"--cwd", repo, "--lineage", record.State.LineageID, "--target", record.State.InitialSnapshot.Identity,
				"--lens", lens, "--order", "0", "--input", input,
			}, &bytes.Buffer{}); err != nil {
				t.Fatalf("admit historical reviewer result: %v", err)
			}

			loaded, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if len(loaded.State.AdmittedRoleResults) != 1 {
				t.Fatalf("historical admitted role results = %d, want one", len(loaded.State.AdmittedRoleResults))
			}
			var admitted admittedReviewerResult
			if err := json.Unmarshal(loaded.State.AdmittedRoleResults[0].Value, &admitted); err != nil {
				t.Fatalf("decode admitted historical result: %v", err)
			}
			if admitted.Subject.SubjectHash != wantSubjects[0].SubjectHash {
				t.Fatalf("admitted historical subject hash = %q, want %q", admitted.Subject.SubjectHash, wantSubjects[0].SubjectHash)
			}

			after := explicitFrozenReviewingStatus(t, repo, record.State.LineageID)
			assertHistoricalPendingSubject(t, after, wantSubjects[1], "after partial admission")
		})
	}
}

func assertHistoricalPendingSubject(t *testing.T, status ReviewTargetStatusResult, want reviewtransaction.ArtifactSubject, phase string) {
	t.Helper()
	if status.NextTransition == nil || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) == 0 ||
		status.NextTransition.Collect.Inputs[0].ArtifactSubject == nil {
		t.Fatalf("historical %s STATUS = %#v", phase, status)
	}
	if got := *status.NextTransition.Collect.Inputs[0].ArtifactSubject; !reflect.DeepEqual(got, want) {
		t.Fatalf("historical pending subject %s = %#v, want %#v", phase, got, want)
	}
}

func historicalGeneratedReviewingStatusFixture(t *testing.T) (string, reviewtransaction.CompactStore, reviewtransaction.CompactRecord) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "go.sum", "base dependency\n", 0o644)
	runReviewCLIGit(t, repo, "add", "go.sum")
	runReviewCLIGit(t, repo, "commit", "-qm", "historical generated base")
	writeReviewStartCandidate(t, repo, "go.sum", "candidate dependency\n", 0o644)
	writeReviewStartCandidate(t, repo, "service-token.ts", "export const token = 'candidate'\n", 0o644)
	store, record := createFrozenReviewingStatusRecord(t, repo, "historical-generated-status", reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{},
	})
	// Re-persist the state as an old writer would have: the optional
	// interpretation field is absent, while the content-only snapshot identity
	// and capture-phase binding remain unchanged.
	record.State.InitialSnapshot.GeneratedPathInterpretation = ""
	record.State.CurrentSnapshot.GeneratedPathInterpretation = ""
	writeReconcileCLIRecord(t, repo, record.State)
	var err error
	record, err = store.Load()
	if err != nil {
		t.Fatalf("load historical snapshot authority: %v", err)
	}
	if len(record.State.SelectedLenses) < 2 {
		t.Fatalf("historical fixture selected %d lenses, want a partial-admission case", len(record.State.SelectedLenses))
	}
	return repo, store, record
}

func historicalGeneratedFrozenContext(t *testing.T, repo string, snapshot reviewtransaction.Snapshot) reviewtransaction.FrozenCandidateContext {
	t.Helper()
	frozen, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).FrozenCandidateContext(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for index := range frozen.ChangedPathManifest {
		frozen.ChangedPathManifest[index].Generated = false
	}
	return frozen
}

func TestNegotiatedStatusBindsManifestlessCaptureSubjectsToTheFrozenDigest(t *testing.T) {
	repo, _, record := frozenReviewingStatusFixture(t, reviewtransaction.TargetCurrentChanges, nil)
	status := explicitFrozenReviewingStatus(t, repo, record.State.LineageID)
	inputs := status.NextTransition.Collect.Inputs
	if len(inputs) == 0 || inputs[0].ChangedPathManifest != nil {
		t.Fatalf("fixture status must offer manifest-less capture inputs: %+v", inputs)
	}
	if status.Frozen == nil || status.Frozen.ChangedPathManifestSHA256 != inputs[0].ArtifactSubject.ChangedPathManifestSHA256 {
		t.Fatalf("the facade must publish the frozen manifest digest the subjects were built from: %+v", status.Frozen)
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("frozen digest must validate without authority: %v", err)
	}
	// The digest only vouches for a manifest that describes the published
	// projection paths: the provider refuses to publish it otherwise.
	frozen, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).FrozenCandidateContext(t.Context(), record.State.InitialSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	manifest := frozen.ChangedPathManifest
	if digest, err := frozenManifestDigestForProjection(manifest, status.Projection.Paths); err != nil || digest != status.Frozen.ChangedPathManifestSHA256 {
		t.Fatalf("frozen manifest matching the projection must publish a digest, got %q, %v", digest, err)
	}
	if _, err := frozenManifestDigestForProjection(manifest, append([]string{"docs/extra.md"}, status.Projection.Paths...)); err == nil || !strings.Contains(err.Error(), "differ from the published projection paths") {
		t.Fatalf("frozen manifest that disagrees with the projection must be refused, got %v", err)
	}
	unbound := status
	unbound.Frozen = &ReviewTargetStatusFrozen{Tier: status.Frozen.Tier, OriginalChangedLines: status.Frozen.OriginalChangedLines, CorrectionBudget: status.Frozen.CorrectionBudget}
	if err := unbound.Validate(); err == nil || !strings.Contains(err.Error(), "without the frozen candidate manifest digest") {
		t.Fatalf("a manifest-less capture without the frozen digest must fail closed, got %v", err)
	}
	// Same paths as the frozen snapshot, different statuses: the digest and the
	// recomputed subject hash both change, so only the frozen binding refuses it.
	subject := *inputs[0].ArtifactSubject
	forgedFrozen := reviewtransaction.FrozenCandidateContext{BaseTree: subject.BaseTree, CandidateTree: subject.CandidateTree}
	for _, path := range status.Projection.Paths {
		forgedFrozen.ChangedPathManifest = append(forgedFrozen.ChangedPathManifest, reviewtransaction.ChangedPathManifestEntry{Path: path, Status: "D", OldMode: "100644", NewMode: "000000", Deleted: true})
	}
	forged, err := reviewtransaction.NewArtifactSubject(record.State, subject.AuthorityRevision, forgedFrozen, subject.Lens, subject.SelectedOrder, "")
	if err != nil {
		t.Fatal(err)
	}
	inputs[0].ArtifactSubject = &forged
	for index := range inputs[0].Arguments {
		if inputs[0].Arguments[index].Name == "subject-hash" {
			inputs[0].Arguments[index].Value = forged.SubjectHash
		}
	}
	if err := status.Validate(); err == nil || !strings.Contains(err.Error(), "differs from the frozen candidate manifest") {
		t.Fatalf("a forged subject digest must be refused by the frozen binding, got %v", err)
	}
}
