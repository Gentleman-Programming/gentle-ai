package reviewtransaction

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func lensEmissionForTest(revision string, level ReviewerContextLevel) LensContextEmission {
	return LensContextEmission{
		Schema: LensContextEmissionSchema, LineageID: "review-0123456789abcdef",
		TargetIdentity:    "sha256:" + strings.Repeat("a", 64),
		AuthorityRevision: revision,
		Lens:              LensReliability, SelectedOrder: 0,
		SubjectHash: "sha256:" + strings.Repeat("b", 64), Level: level,
	}
}

// TestLensContextEmissionSlotIsScopedToItsAuthorityRevision pins issue #2850:
// a maintainer-authorized reopen-results moves the same lineage back to
// reviewing under a NEW authority revision, and negotiated status then demands
// a fresh capture for that slot. The emission path used to be keyed by lens and
// order alone, so the new revision's record collided with the old one forever —
// while ReadLensContextEmission already treated that older record as absent
// because it describes a different revision. The writer's key now matches the
// identity the reader already enforces.
func TestLensContextEmissionSlotIsScopedToItsAuthorityRevision(t *testing.T) {
	dir := t.TempDir()
	first := lensEmissionForTest("sha256:"+strings.Repeat("1", 64), ReviewerContextLevelProviderCommand)
	if err := PublishLensContextEmission(dir, first); err != nil {
		t.Fatalf("first emission: %v", err)
	}
	second := lensEmissionForTest("sha256:"+strings.Repeat("2", 64), ReviewerContextLevelProviderCommand)
	if err := PublishLensContextEmission(dir, second); err != nil {
		t.Fatalf("reopened slot at a new revision must record its own emission: %v", err)
	}

	// Neither record was rewritten: both remain readable against their own
	// revision, which is what "audit history is never rewritten" has to mean.
	for _, emission := range []LensContextEmission{first, second} {
		got, found := ReadLensContextEmission(dir, emission.LineageID, emission.TargetIdentity,
			emission.AuthorityRevision, emission.Lens, emission.SelectedOrder, emission.SubjectHash)
		if !found || got != emission {
			t.Fatalf("emission for revision %s = %#v, found=%v", emission.AuthorityRevision, got, found)
		}
	}
}

// TestLensContextEmissionStillReadsLegacyFlatRecords keeps stores written
// before the key was corrected readable: the record moves, the history does
// not disappear.
func TestLensContextEmissionStillReadsLegacyFlatRecords(t *testing.T) {
	dir := t.TempDir()
	emission := lensEmissionForTest("sha256:"+strings.Repeat("3", 64), ReviewerContextLevelRuntimeInterception)
	legacyDir := filepath.Join(dir, LensContextEmissionDir)
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload, err := canonicalLensContextEmissionForTest(emission)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "00-"+emission.Lens+".json"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	got, found := ReadLensContextEmission(dir, emission.LineageID, emission.TargetIdentity,
		emission.AuthorityRevision, emission.Lens, emission.SelectedOrder, emission.SubjectHash)
	if !found || got != emission {
		t.Fatalf("legacy emission = %#v, found=%v", got, found)
	}
}

// TestLensContextEmissionStillRefusesAConflictingMechanism keeps the deliberate
// design intact: within ONE frozen slot at ONE revision, two delivery
// mechanisms remain a conflict rather than an overwrite. Only the slot's
// identity was wrong, not this rule.
func TestLensContextEmissionStillRefusesAConflictingMechanism(t *testing.T) {
	dir := t.TempDir()
	revision := "sha256:" + strings.Repeat("4", 64)
	if err := PublishLensContextEmission(dir, lensEmissionForTest(revision, ReviewerContextLevelProviderCommand)); err != nil {
		t.Fatal(err)
	}
	err := PublishLensContextEmission(dir, lensEmissionForTest(revision, ReviewerContextLevelRuntimeInterception))
	if err == nil {
		t.Fatal("a second mechanism for the same slot and revision must still conflict")
	}
}

// TestCapturedVerificationEvidenceIsScopedToItsAuthorityRevision pins issue
// #2623: the evidence directory was keyed by target identity alone, so
// correction-phase evidence for an unchanged candidate collided with the
// pre-correction record permanently — while ReadCapturedVerificationEvidence
// already rejected that older record through ValidateBinding.
func TestCapturedVerificationEvidenceIsScopedToItsAuthorityRevision(t *testing.T) {
	dir := t.TempDir()
	target := Snapshot{
		Kind:     TargetCurrentChanges,
		BaseTree: strings.Repeat("1", 64), CandidateTree: strings.Repeat("2", 64),
		Paths:                  []string{"a.go"},
		IntendedUntracked:      []string{},
		PathsDigest:            "sha256:" + strings.Repeat("d", 64),
		IntendedUntrackedProof: "sha256:" + strings.Repeat("e", 64),
		Identity:               "sha256:" + strings.Repeat("c", 64),
	}
	first := CaptureVerificationEvidenceRequest{
		StoreDir: dir, LineageID: "review-0123456789abcdef",
		AuthorityRevision: "sha256:" + strings.Repeat("5", 64),
		Target:            target, Payload: []byte("pre-correction suite passed\n"),
		Outcome: VerificationOutcomePassed,
	}
	if _, err := PublishCapturedVerificationEvidence(first); err != nil {
		t.Fatalf("first evidence: %v", err)
	}
	second := first
	second.AuthorityRevision = "sha256:" + strings.Repeat("6", 64)
	second.Payload = []byte("correction-phase suite passed\n")
	if _, err := PublishCapturedVerificationEvidence(second); err != nil {
		t.Fatalf("correction-phase evidence for the same target must record its own directory: %v", err)
	}
	for _, request := range []CaptureVerificationEvidenceRequest{first, second} {
		captured, err := ReadCapturedVerificationEvidence(dir, request.LineageID, request.AuthorityRevision, request.Target)
		if err != nil {
			t.Fatalf("evidence for revision %s: %v", request.AuthorityRevision, err)
		}
		if string(captured.Payload) != string(request.Payload) {
			t.Fatalf("evidence payload = %q, want %q", captured.Payload, request.Payload)
		}
	}
}

func canonicalLensContextEmissionForTest(emission LensContextEmission) ([]byte, error) {
	payload, err := json.Marshal(emission)
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}
