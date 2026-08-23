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

func canonicalLensContextEmissionForTest(emission LensContextEmission) ([]byte, error) {
	payload, err := json.Marshal(emission)
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}
