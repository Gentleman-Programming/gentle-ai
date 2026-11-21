package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// legacyV1TreeDigest fingerprints every file under a v1 authority tree so a
// test can prove an operation never touched it.
func legacyV1TreeDigest(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(root, path)
		if entry.IsDir() {
			lines = append(lines, "d "+rel)
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(payload)
		lines = append(lines, "f "+rel+" "+hex.EncodeToString(sum[:]))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// legacyV1Fixture persists one structurally valid shipped v1 lineage and one
// damaged one, plus a v1 twin of an existing compact lineage.
func legacyV1Fixture(t *testing.T, repo string) string {
	t.Helper()
	for _, lineage := range []string{"v1-ignored-valid", "v1-ignored-compact"} {
		store, err := AuthoritativeStore(context.Background(), repo, lineage)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := NewTransaction(Start{
			LineageID: lineage, Mode: ModeOrdinary4R, Generation: 1,
			Snapshot: Snapshot{
				Kind: TargetCurrentChanges, BaseTree: tree("a"), CandidateTree: tree("b"),
				PathsDigest: hash("a"), IntendedUntracked: []string{},
				IntendedUntrackedProof: hash("b"), Paths: []string{"internal/example.go"}, Identity: hash("c"),
			},
			PolicyHash: hash("d"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.StartReview(); err != nil {
			t.Fatal(err)
		}
		writeStoreEvent(t, store, Record{Operation: "review/start", Transaction: *tx})
	}
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(base, "v1", "v1-ignored-broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "HEAD"), []byte("not-a-revision\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(base, "v1")
}

// A repository that still carries a v1 store behaves as if v1 were absent: the
// inventory lists only compact-v2 authority, reports no collision, lock,
// diagnostic or incompleteness for the v1 tree, and deletes nothing.
func TestInventoryIgnoresLegacyV1Store(t *testing.T) {
	repo := initSnapshotRepo(t)
	approvedCompactFixture(t, repo, "v1-ignored-compact")
	v1Root := legacyV1Fixture(t, repo)
	before := legacyV1TreeDigest(t, v1Root)

	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !report.Authoritative || report.Status != AuthorityStatusApproved {
		t.Fatalf("a v1 store changed the inventory verdict: %#v", report)
	}
	if len(report.Entries) != 1 || report.Entries[0].LineageID != "v1-ignored-compact" ||
		report.Entries[0].Version != AuthorityVersionCompact || report.Entries[0].Status != AuthorityStatusApproved {
		t.Fatalf("inventory must list only the compact lineage, untouched by its v1 twin: %#v", report.Entries)
	}
	for _, lock := range report.Locks {
		if lock.Version != AuthorityVersionCompact || lock.LineageID != "" {
			t.Fatalf("v1 tree leaked a lock: %#v", lock)
		}
	}
	if len(report.Diagnostics) != 0 {
		t.Fatalf("v1 tree leaked diagnostics: %#v", report.Diagnostics)
	}
	if after := legacyV1TreeDigest(t, v1Root); after != before {
		t.Fatalf("inventory modified the v1 tree:\nbefore=%s\nafter=%s", before, after)
	}
}

// A lineage that exists only in v1 is not authority: abandon sees nothing to
// abandon, inspection reports it ineligible, an exact-lineage occupancy check
// does not count it, and the v1 tree stays byte-identical.
func TestAbandonAndOccupancyIgnoreLegacyV1Store(t *testing.T) {
	repo := initSnapshotRepo(t)
	v1Root := legacyV1Fixture(t, repo)
	before := legacyV1TreeDigest(t, v1Root)

	eligibility, err := InspectCompactPristineAbandonment(context.Background(), repo, "v1-ignored-valid")
	if err != nil || eligibility.Eligible {
		t.Fatalf("v1-only lineage must not be abandonable: %#v %v", eligibility, err)
	}
	occupied, err := ExactReviewLineageOccupied(context.Background(), repo, "v1-ignored-valid")
	if err != nil || occupied {
		t.Fatalf("v1-only lineage must not occupy a lineage name: occupied=%v err=%v", occupied, err)
	}
	if after := legacyV1TreeDigest(t, v1Root); after != before {
		t.Fatalf("v1 tree changed:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestSameLineageLegacyV1EntryNeitherCollidesNorExtendsTheAbandonBinding(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	record, _ := pristineReviewingFixture(t, repo, "collide-ignored")
	storeLegacyReviewingStatus(t, repo, record.State.LineageID, record.State.InitialSnapshot)
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	v1Root := filepath.Join(base, "v1")
	before := legacyV1TreeDigest(t, v1Root)

	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil || !report.Complete {
		t.Fatalf("inventory = %#v, %v", report, err)
	}
	if len(report.Entries) != 1 || report.Entries[0].Version != AuthorityVersionCompact ||
		report.Entries[0].Status != AuthorityStatusActive || len(report.Entries[0].Problems) != 0 {
		t.Fatalf("same-lineage v1 entry leaked into inventory: %#v", report.Entries)
	}
	eligibility, err := InspectCompactPristineAbandonment(context.Background(), repo, record.State.LineageID)
	if err != nil || !eligibility.Eligible || len(eligibility.Cascade) != 0 {
		t.Fatalf("eligibility = %#v, %v", eligibility, err)
	}
	request := abandonFixtureRequest(record)
	got := eligibility.Authorization(request.LineageID, request.Actor, request.Reason)
	want := RenderCompactAbandonAuthorization(request.LineageID, eligibility.Revision, eligibility.SnapshotIdentity, request.Actor, request.Reason, eligibility.DiscardedWork)
	if got != want || strings.Count(got, "\n") != 7 || strings.Contains(got, "legacy_v1_entry") {
		t.Fatalf("binding is not the frozen eight lines:\n%s", got)
	}
	committed, err := abandonAuthorized(t, context.Background(), repo, request)
	if err != nil || committed.Status != CompactReclaimCommitted || len(committed.Cascaded) != 0 {
		t.Fatalf("abandon = %#v, %v", committed, err)
	}
	if after := legacyV1TreeDigest(t, v1Root); after != before {
		t.Fatalf("v1 tree changed:\nbefore=%s\nafter=%s", before, after)
	}
}
