package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const abandonAuthorizationActor = "maintainer@example.com"

func abandonQuarantineCount(t *testing.T, repo string) int {
	t.Helper()
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(base, "quarantine"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return len(entries)
}

// handBinding builds the binding from the documented format, independent of the
// production renderer, so a format change cannot pass unnoticed.
func handBinding(lineage, revision, snapshot string, extra ...string) string {
	lines := append([]string{
		CompactAbandonAuthorizationSchema,
		"lineage=" + lineage,
		"revision=" + revision,
		"snapshot_identity=" + snapshot,
		"reason=" + CompactAbandonReasonOperatorDisposition,
		"captured_lens_results=",
		"findings_present=false",
		"actor=" + abandonAuthorizationActor,
	}, extra...)
	return strings.Join(lines, "\n")
}

// TestAbandonRequiresTheMaintainerAuthorizationBinding: without or with a wrong
// binding or revision the abandonment refuses and nothing in the store moves;
// with the exact binding it abandons the target and its recovery cascade.
func TestAbandonRequiresTheMaintainerAuthorizationBinding(t *testing.T) {
	repo := initSnapshotRepo(t)
	chain := recoveryChainFixture(t, repo, "abandon-auth", 3)
	target, middle, leaf := chain[0], chain[1], chain[2]
	lineage := target.State.LineageID
	base := handBinding(lineage, target.Revision, target.State.InitialSnapshot.Identity)
	request := CompactAbandonRequest{
		LineageID: lineage, ExpectedRevision: target.Revision,
		Reason: CompactAbandonReasonOperatorDisposition, Actor: abandonAuthorizationActor,
	}
	untouched := func(label string) {
		t.Helper()
		for _, record := range chain {
			if abandonEntryAbsent(t, repo, record.State.LineageID) {
				t.Fatalf("%s: refused abandonment moved %s", label, record.State.LineageID)
			}
		}
		if got := abandonQuarantineCount(t, repo); got != 0 {
			t.Fatalf("%s: refused abandonment created %d quarantine entries", label, got)
		}
	}

	// The binding must name every cascaded successor, deepest first.
	cascadeLine := func(record CompactRecord) string {
		return "cascade=" + record.State.LineageID + "@" + record.Revision + " snapshot_identity=" + record.State.InitialSnapshot.Identity +
			" captured_lens_results= findings_present=false"
	}
	exact := handBinding(lineage, target.Revision, target.State.InitialSnapshot.Identity, cascadeLine(leaf), cascadeLine(middle))

	for name, mutate := range map[string]func(CompactAbandonRequest) CompactAbandonRequest{
		"no authorization": func(r CompactAbandonRequest) CompactAbandonRequest { return r },
		"eight-line binding that omits the cascade": func(r CompactAbandonRequest) CompactAbandonRequest {
			r.MaintainerAuthorization = base
			return r
		},
		"cascade in the wrong order": func(r CompactAbandonRequest) CompactAbandonRequest {
			r.MaintainerAuthorization = handBinding(lineage, target.Revision, target.State.InitialSnapshot.Identity, cascadeLine(middle), cascadeLine(leaf))
			return r
		},
		"binding with a trailing newline": func(r CompactAbandonRequest) CompactAbandonRequest {
			r.MaintainerAuthorization = exact + "\n"
			return r
		},
	} {
		result, err := AbandonCompactStore(context.Background(), repo, mutate(request))
		if err == nil || !strings.Contains(err.Error(), "requires an exact maintainer authorization binding") {
			t.Fatalf("%s: error = %v", name, err)
		}
		if result.QuarantinePath != "" || len(result.Cascaded) != 0 {
			t.Fatalf("%s: refusal reported a quarantine: %#v", name, result)
		}
		// The refusal prints the exact binding, naming every lineage that moves.
		for _, record := range []CompactRecord{middle, leaf} {
			if !strings.Contains(err.Error(), "cascade="+record.State.LineageID+"@"+record.Revision) {
				t.Fatalf("%s: refusal does not name cascaded %s: %v", name, record.State.LineageID, err)
			}
		}
		if !strings.Contains(err.Error(), exact) {
			t.Fatalf("%s: refusal does not print the exact binding:\n%v", name, err)
		}
		untouched(name)
	}

	stale := request
	stale.ExpectedRevision = "not-the-revision"
	stale.MaintainerAuthorization = exact
	if _, err := AbandonCompactStore(context.Background(), repo, stale); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("stale expected revision = %v", err)
	}
	untouched("stale revision")

	bad := request
	bad.Reason = "because"
	bad.MaintainerAuthorization = exact
	if _, err := AbandonCompactStore(context.Background(), repo, bad); err == nil || !strings.Contains(err.Error(), "requires reason") {
		t.Fatalf("out-of-vocabulary reason = %v", err)
	}
	untouched("bad reason")

	request.MaintainerAuthorization = exact
	committed, err := AbandonCompactStore(context.Background(), repo, request)
	if err != nil {
		t.Fatalf("abandon with the exact binding: %v", err)
	}
	if committed.Status != CompactReclaimCommitted || committed.LineageID != lineage || len(committed.Cascaded) != 2 ||
		committed.Cascaded[0].LineageID != leaf.State.LineageID || committed.Cascaded[1].LineageID != middle.State.LineageID {
		t.Fatalf("abandonment result = %#v", committed)
	}
	for _, record := range chain {
		if !abandonEntryAbsent(t, repo, record.State.LineageID) {
			t.Fatalf("%s survived an authorized abandonment", record.State.LineageID)
		}
	}
	if proof := committed.Abandonment; proof == nil || proof.AuthorizationBinding != exact {
		t.Fatalf("audit record does not keep the authorized binding: %#v", committed.Abandonment)
	}
	// Exact replay converges; a replay with a different binding does not.
	replayed, err := AbandonCompactStore(context.Background(), repo, request)
	if err != nil || replayed.QuarantinePath != committed.QuarantinePath {
		t.Fatalf("exact replay = %#v, %v", replayed, err)
	}
	request.MaintainerAuthorization = base
	if _, err := AbandonCompactStore(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "no committed abandonment record matches") {
		t.Fatalf("replay with a different binding = %v", err)
	}
}

// TestAbandonBindingNamesLegacyV1AndUnverifiableEntries: the binding carries a
// line for a same-lineage legacy-v1 entry and for an unreadable record, so the
// human approves what the discarded work could not be shown to be.
func TestAbandonBindingNamesLegacyV1AndUnverifiableEntries(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\nbinding change\n")
	_, store := pristineReviewingFixture(t, repo, "abandon-bind-v1")
	truncateCompactStateFile(t, store)
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	v1Dir := filepath.Join(base, "v1", "abandon-bind-v1")
	if err := os.MkdirAll(v1Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	exact := handBinding("abandon-bind-v1", compactAbandonUnavailable, compactAbandonUnavailable, "discarded_work=unverifiable", "legacy_v1_entry=abandon-bind-v1")
	request := CompactAbandonRequest{
		LineageID: "abandon-bind-v1", ExpectedRevision: compactAbandonUnavailable,
		Reason: CompactAbandonReasonOperatorDisposition, Actor: abandonAuthorizationActor,
	}
	if _, err := AbandonCompactStore(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), exact) {
		t.Fatalf("refusal does not print the binding naming the v1 and unverifiable entries: %v", err)
	}
	if abandonEntryAbsent(t, repo, "abandon-bind-v1") || abandonQuarantineCount(t, repo) != 0 {
		t.Fatal("refused abandonment changed the store")
	}
	request.MaintainerAuthorization = exact
	committed, err := AbandonCompactStore(context.Background(), repo, request)
	if err != nil || len(committed.Cascaded) != 1 {
		t.Fatalf("authorized abandonment = %#v, %v", committed, err)
	}
	if _, statErr := os.Stat(v1Dir); !os.IsNotExist(statErr) || !abandonEntryAbsent(t, repo, "abandon-bind-v1") {
		t.Fatal("entries survived the authorized abandonment")
	}
}

// TestAbandonBindingStaysEightLinesWhenNothingElseMoves pins the format
// gentle-shell renders: a readable lineage with no successors and no v1 entry
// is authorized by exactly the eight frozen lines.
func TestAbandonBindingStaysEightLinesWhenNothingElseMoves(t *testing.T) {
	repo := initSnapshotRepo(t)
	chain := recoveryChainFixture(t, repo, "abandon-eight", 1)
	record := chain[0]
	eligibility, err := InspectCompactPristineAbandonment(context.Background(), repo, record.State.LineageID)
	if err != nil || !eligibility.Eligible {
		t.Fatalf("eligibility = %#v, %v", eligibility, err)
	}
	want := handBinding(record.State.LineageID, record.Revision, record.State.InitialSnapshot.Identity)
	if got := eligibility.Authorization(record.State.LineageID, abandonAuthorizationActor, CompactAbandonReasonOperatorDisposition); got != want || strings.Count(got, "\n") != 7 {
		t.Fatalf("binding = %q, want %q", got, want)
	}
	if _, err := AbandonCompactStore(context.Background(), repo, CompactAbandonRequest{
		LineageID: record.State.LineageID, ExpectedRevision: record.Revision,
		Reason: CompactAbandonReasonOperatorDisposition, Actor: abandonAuthorizationActor, MaintainerAuthorization: want,
	}); err != nil {
		t.Fatalf("eight-line binding: %v", err)
	}
}
