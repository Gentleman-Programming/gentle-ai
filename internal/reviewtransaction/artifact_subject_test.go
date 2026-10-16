package reviewtransaction

import (
	"strings"
	"testing"
)

func artifactSubjectFixture(t *testing.T) (CompactState, string, FrozenCandidateContext) {
	t.Helper()
	paths := []string{"internal/a.go", "internal/b.go"}
	baseTree, candidateTree := strings.Repeat("a", 40), strings.Repeat("b", 40)
	state := CompactState{
		LineageID:       "review-artifact-subject",
		SelectedLenses:  []string{LensReliability, LensReadability},
		InitialSnapshot: Snapshot{Identity: "sha256:" + strings.Repeat("1", 64), BaseTree: baseTree, CandidateTree: candidateTree, Paths: paths},
	}
	context := FrozenCandidateContext{
		BaseTree: baseTree, CandidateTree: candidateTree,
		ChangedPathManifest: []ChangedPathManifestEntry{
			{Path: paths[0], Status: CandidatePathModified, OldMode: "100644", NewMode: "100644"},
			{Path: paths[1], Status: CandidatePathAdded, OldMode: "000000", NewMode: "100644", IntendedUntracked: true},
		},
	}
	return state, "sha256:" + strings.Repeat("2", 64), context
}

func TestArtifactSubjectBindsFrozenCandidateAndSlot(t *testing.T) {
	state, revision, context := artifactSubjectFixture(t)
	subject, err := NewArtifactSubject(state, revision, context, LensReadability, 1, "")
	if err != nil {
		t.Fatalf("NewArtifactSubject() error = %v", err)
	}
	if subject.Schema != ArtifactSubjectSchema || subject.SubjectHash == "" ||
		subject.LineageID != state.LineageID || subject.AuthorityRevision != revision ||
		subject.TargetIdentity != state.InitialSnapshot.Identity || subject.BaseTree != context.BaseTree || subject.CandidateTree != context.CandidateTree ||
		subject.Lens != LensReadability || subject.SelectedOrder != 1 {
		t.Fatalf("subject = %#v", subject)
	}
	wantManifest, err := ChangedPathManifestDigest(context.ChangedPathManifest)
	if err != nil {
		t.Fatal(err)
	}
	if subject.ChangedPathManifestSHA256 != wantManifest {
		t.Fatalf("manifest digest = %q, want %q", subject.ChangedPathManifestSHA256, wantManifest)
	}
	if err := ValidateArtifactSubject(subject); err != nil {
		t.Fatalf("ValidateArtifactSubject() error = %v", err)
	}

	mutated := subject
	mutated.SelectedOrder = 0
	if err := ValidateArtifactSubject(mutated); err == nil {
		t.Fatal("slot-mutated subject validated")
	}
	mutated = subject
	mutated.ChangedPathManifestSHA256 = "sha256:" + strings.Repeat("3", 64)
	if err := ValidateArtifactSubject(mutated); err == nil {
		t.Fatal("manifest-mutated subject validated")
	}
}

func TestArtifactSubjectOptionalCorrectionIdentityIsBound(t *testing.T) {
	state, revision, context := artifactSubjectFixture(t)
	correction := "sha256:" + strings.Repeat("4", 64)
	subject, err := NewArtifactSubject(state, revision, context, LensReliability, 0, correction)
	if err != nil {
		t.Fatal(err)
	}
	if subject.CorrectionTargetIdentity != correction {
		t.Fatalf("correction target = %q", subject.CorrectionTargetIdentity)
	}
	without := subject
	without.CorrectionTargetIdentity = ""
	if err := ValidateArtifactSubject(without); err == nil {
		t.Fatal("subject hash ignored correction identity")
	}
}

func TestLegacyArtifactSubjectRetainsCandidateDiffBinding(t *testing.T) {
	state, revision, frozen := artifactSubjectFixture(t)
	diff, err := NewFrozenCandidateDiff([]byte("diff --git a/internal/a.go b/internal/a.go\n"))
	if err != nil {
		t.Fatal(err)
	}
	frozen.LegacyCandidateDiff = &diff
	legacy, err := NewLegacyArtifactSubject(state, revision, frozen, LensReliability, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Schema != ArtifactSubjectSchemaV1 || legacy.CandidateDiffSHA256 != diff.SHA256 || legacy.BaseTree != "" || legacy.CandidateTree != "" {
		t.Fatalf("legacy subject = %#v", legacy)
	}
	if err := ValidateArtifactSubject(legacy); err != nil {
		t.Fatal(err)
	}
	nativeGit, err := NewArtifactSubject(state, revision, frozen, LensReliability, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if nativeGit.SubjectHash == legacy.SubjectHash {
		t.Fatal("v1 candidate-diff and v2 native-Git subjects share an identity")
	}
}

// TestArtifactSubjectBindsFrozenRequestContext proves the lens slot identity
// commits to the request a review judges against: a changed request yields a
// different authority revision and subject, so an admitted lens result can
// never be replayed under a different request.
func TestArtifactSubjectBindsFrozenRequestContext(t *testing.T) {
	state, _, context := artifactSubjectFixture(t)
	subjectFor := func(request string) string {
		t.Helper()
		candidate := state
		if request == "" {
			phase, err := deriveCompactCapturePhaseRevision(candidate)
			if err != nil {
				t.Fatal(err)
			}
			candidate.CapturePhaseRevision = phase
		} else if err := candidate.FreezeRequestContext(request); err != nil {
			t.Fatal(err)
		}
		subject, err := NewArtifactSubject(candidate, candidate.CapturePhaseRevision, context, LensReliability, 0, "")
		if err != nil {
			t.Fatal(err)
		}
		return subject.SubjectHash
	}
	absent, first, second := subjectFor(""), subjectFor("S1 add --category.\n"), subjectFor("S1 add --amount.\n")
	if absent == first || first == second || absent == second {
		t.Fatalf("subject does not bind the request context: absent=%s first=%s second=%s", absent, first, second)
	}
}
