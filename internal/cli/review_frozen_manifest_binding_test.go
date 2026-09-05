package cli

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// A capture input without an inlined manifest is bound to the frozen candidate
// through its subject digest: the real STATUS facade publishes the digest it
// re-derived from the repository in `frozen`, every validation path refuses a
// self-consistent subject whose digest differs from it, and an envelope that
// offers a manifest-less capture without that digest is refused outright.
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
