package reviewtransaction

// Wave 5 (Gate Cutover) Slice 6: the surviving read-only coverage for
// candidate_decline.go, superseding candidate_decline_test.go (deleted --
// both its tests drove RecordCandidateDecline/ResolveCandidateDeclineForGate
// directly, neither of which exists anymore). parseCandidateDeclineAuthorization
// itself has no production caller today (see candidate_decline.go's own doc
// comment), so this file constructs a decline record's canonical bytes
// directly rather than through the now-deleted writer, proving the parser
// still round-trips a structurally valid record and still fails closed on a
// tampered one.

import (
	"context"
	"testing"
)

func TestParseCandidateDeclineAuthorizationRoundTripsCanonicalBytes(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "declined candidate\n")
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	authorization := CandidateDeclineAuthorization{Schema: CandidateDeclineAuthorizationSchema, Snapshot: snapshot}
	ref, err := candidateDeclineDigest(authorization)
	if err != nil {
		t.Fatal(err)
	}
	authorization.AuthorizationRef = ref
	payload, err := canonicalCandidateDeclinePayload(authorization)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := parseCandidateDeclineAuthorization(payload)
	if err != nil {
		t.Fatalf("parse canonical candidate decline authorization: %v", err)
	}
	if parsed.AuthorizationRef != authorization.AuthorizationRef || parsed.Snapshot.Identity != snapshot.Identity {
		t.Fatalf("round-tripped authorization = %#v, want %#v", parsed, authorization)
	}
}

func TestParseCandidateDeclineAuthorizationRejectsNonCanonicalBytes(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "declined candidate\n")
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	authorization := CandidateDeclineAuthorization{Schema: CandidateDeclineAuthorizationSchema, Snapshot: snapshot}
	ref, err := candidateDeclineDigest(authorization)
	if err != nil {
		t.Fatal(err)
	}
	authorization.AuthorizationRef = ref

	if _, err := parseCandidateDeclineAuthorization([]byte("{\n")); err == nil {
		t.Fatal("parseCandidateDeclineAuthorization accepted malformed JSON")
	}

	payload, err := canonicalCandidateDeclinePayload(authorization)
	if err != nil {
		t.Fatal(err)
	}
	// Corrupt the canonical bytes' trailing newline without re-canonicalizing
	// (canonicalCandidateDeclinePayload would refuse mismatched content
	// itself -- this proves the parser's OWN canonical-bytes comparison,
	// not just validate()'s structural check).
	corrupted := append([]byte(nil), payload...)
	corrupted[len(corrupted)-1] = ' '
	if _, err := parseCandidateDeclineAuthorization(corrupted); err == nil {
		t.Fatal("parseCandidateDeclineAuthorization accepted non-canonical bytes")
	}
}
