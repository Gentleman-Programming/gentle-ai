package reviewtransaction

import (
	"strings"
	"testing"
)

func artifactSubjectFixture(t *testing.T) (CompactState, string, FrozenCandidateContext) {
	t.Helper()
	diff, err := NewFrozenCandidateDiff([]byte("diff --git a/internal/a.go b/internal/a.go\n"))
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"internal/a.go", "internal/b.go"}
	state := CompactState{
		LineageID:       "review-artifact-subject",
		SelectedLenses:  []string{LensReliability, LensReadability},
		InitialSnapshot: Snapshot{Identity: "sha256:" + strings.Repeat("1", 64), Paths: paths},
	}
	context := FrozenCandidateContext{
		CandidateDiff: diff,
		ChangedPathManifest: []ChangedPathManifestEntry{
			{Path: paths[0], Status: CandidatePathModified, OldMode: "100644", NewMode: "100644"},
			{Path: paths[1], Status: CandidatePathAdded, OldMode: "000000", NewMode: "100644", IntendedUntracked: true},
		},
		repositoryPaths: []string{
			"Dockerfile", "Makefile", "docs/naïve guide.md", "docs/秘密 guide.md", "internal/a.go", "internal/b.go",
			"internal/secret.go", "main.go", "secret.go", "sha256", "status", "unrelated/old.go",
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
		subject.TargetIdentity != state.InitialSnapshot.Identity || subject.CandidateDiffSHA256 != context.CandidateDiff.SHA256 ||
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
