package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// TestResolveGoverningAuthorityFullMatrix is task 5.1's RED test: the full
// 2×2 over {new: related | unrelated | absent} × {legacy: present | absent}
// (design decision 4, Amendment C). ResolveGoverningAuthority performs no
// I/O, so every one of the six cells is exercised as a pure table-driven
// case.
func TestResolveGoverningAuthorityFullMatrix(t *testing.T) {
	cases := []struct {
		name          string
		found         bool
		relation      CandidateRelation
		legacyPresent bool
		want          GoverningAuthorityKind
	}{
		{"related, legacy present: new governs regardless", true, ShadowRelationExact, true, GoverningAuthorityKindNew},
		{"related, legacy absent: new governs", true, ShadowRelationExact, false, GoverningAuthorityKindNew},
		{"related (changed), legacy present: new still governs", true, ShadowRelationChanged, true, GoverningAuthorityKindNew},
		{"related (ambiguous has no live counterpart but is still 'related'): new governs", true, ShadowRelationAmbiguous, false, GoverningAuthorityKindNew},
		{"unrelated, legacy present: deny — never authorized by legacy", true, ShadowRelationUnrelated, true, GoverningAuthorityKindDeny},
		{"unrelated, legacy absent: defer to legacy, byte-identical", true, ShadowRelationUnrelated, false, GoverningAuthorityKindLegacy},
		{"absent, legacy present: legacy path, byte-identical (today's behavior)", false, "", true, GoverningAuthorityKindLegacy},
		{"absent, legacy absent: legacy's receipt_missing path, byte-identical", false, "", false, GoverningAuthorityKindLegacy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveGoverningAuthority(tc.found, tc.relation, tc.legacyPresent)
			if got != tc.want {
				t.Fatalf("ResolveGoverningAuthority(found=%v, relation=%q, legacyPresent=%v) = %q, want %q",
					tc.found, tc.relation, tc.legacyPresent, got, tc.want)
			}
		})
	}
}

// TestDiscoverNewLineageAbsentWithoutMarker proves the discovery rule's
// corrected construction: no explicit --lineage marker means "new absent"
// without ever resolving a store root or touching the filesystem.
func TestDiscoverNewLineageAbsentWithoutMarker(t *testing.T) {
	record, found, err := DiscoverNewLineage(context.Background(), "/does/not/exist", "")
	if err != nil || found || record.Authority.LineageID != "" {
		t.Fatalf("DiscoverNewLineage(no marker) = record=%#v found=%v err=%v, want absent", record, found, err)
	}
}

// TestDiscoverNewLineageAbsentWithMarkerButNoDirectory proves a marker that
// simply never wrote anything (the ordinary "new absent" case reachable
// through an explicit --lineage flag naming a lineage id that has no v3
// directory at all) is absent, not corrupted.
func TestDiscoverNewLineageAbsentWithMarkerButNoDirectory(t *testing.T) {
	repo := initSnapshotRepo(t)
	record, found, err := DiscoverNewLineage(context.Background(), repo, "never-started-lineage")
	if err != nil || found || record.Authority.LineageID != "" {
		t.Fatalf("DiscoverNewLineage(no directory) = record=%#v found=%v err=%v, want absent", record, found, err)
	}
}

// TestDiscoverNewLineageRelatedFindsTheRecord proves the ordinary present
// case: a real Mutate-written record is found and returned intact.
func TestDiscoverNewLineageRelatedFindsTheRecord(t *testing.T) {
	repo := initSnapshotRepo(t)
	store, err := NewLineageAuthorityStore(context.Background(), repo, "discoverable-lineage")
	if err != nil {
		t.Fatal(err)
	}
	authority := fixtureNewLineageAuthority("discoverable-lineage", NewLineageStateReviewing)
	if _, err := store.Mutate(context.Background(), "", func(next *NewLineageAuthority) error {
		*next = authority
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	record, found, err := DiscoverNewLineage(context.Background(), repo, "discoverable-lineage")
	if err != nil || !found {
		t.Fatalf("DiscoverNewLineage(present) = found=%v err=%v, want found", found, err)
	}
	if record.Authority.LineageID != "discoverable-lineage" {
		t.Fatalf("DiscoverNewLineage(present) authority = %#v", record.Authority)
	}
}

// TestDiscoverNewLineageMarkerPresentRecordRemovedDenies is task 5.2's
// MANDATORY RED test (design decision 4's non-matrix discovery-integrity
// clause): a new-lineage marker (the v3/<lineage> directory) is present —
// asserting this lineage was reviewed under the new-lineage model — but its
// record (review-state.json) was removed. This MUST report
// *NewLineageMarkerCorruptedError, distinct from — and never collapsing
// into — the ordinary "absent" case a caller could silently fall through to
// legacy from.
func TestDiscoverNewLineageMarkerPresentRecordRemovedDenies(t *testing.T) {
	repo := initSnapshotRepo(t)
	store, err := NewLineageAuthorityStore(context.Background(), repo, "corrupted-marker-lineage")
	if err != nil {
		t.Fatal(err)
	}
	authority := fixtureNewLineageAuthority("corrupted-marker-lineage", NewLineageStateReviewing)
	if _, err := store.Mutate(context.Background(), "", func(next *NewLineageAuthority) error {
		*next = authority
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// The marker (the directory) survives; only the record inside it is
	// removed — the exact shape task 5.2 names: "v3 record removed".
	if err := os.Remove(store.StatePath()); err != nil {
		t.Fatal(err)
	}
	if info, statErr := os.Stat(store.Dir); statErr != nil || !info.IsDir() {
		t.Fatalf("marker directory must still exist after removing only the record: stat=%v err=%v", info, statErr)
	}

	record, found, err := DiscoverNewLineage(context.Background(), repo, "corrupted-marker-lineage")
	if found || record.Authority.LineageID != "" {
		t.Fatalf("DiscoverNewLineage(marker present, record removed) reported found=%v record=%#v, want not-found", found, record)
	}
	var corrupted *NewLineageMarkerCorruptedError
	if !errors.As(err, &corrupted) {
		t.Fatalf("DiscoverNewLineage(marker present, record removed) error = %T %v, want *NewLineageMarkerCorruptedError", err, err)
	}
	if corrupted.LineageID != "corrupted-marker-lineage" {
		t.Fatalf("corrupted marker error lineage = %q", corrupted.LineageID)
	}
	if !strings.Contains(corrupted.Error(), "corrupted-marker-lineage") {
		t.Fatalf("corrupted marker error message = %q, must name the lineage", corrupted.Error())
	}
}
