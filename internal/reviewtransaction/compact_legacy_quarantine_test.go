package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// malformedLegacyFreezeFixture persists one shipped legacy-v1 lineage whose
// review/freeze-findings successor captured state the semantic replay
// considers unrelated to the findings freeze — the #1311 corruption class.
// The chain is structurally intact and content-addressed; only the semantic
// historical replay fails.
func malformedLegacyFreezeFixture(t *testing.T, repo, lineage string) (Store, string, string) {
	t.Helper()
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
	genesis := writeStoreEvent(t, store, Record{Operation: "review/start", Transaction: *tx})
	freeze := historicalFreezeTransition(t, *tx)
	// The persisted event also captured an evidence hash the freeze transition
	// never writes, so the semantic historical replay reports that the freeze
	// changed unrelated transaction state while the event stays structurally
	// valid, content-addressed, and decodable.
	freeze.EvidenceHash = hash("0")
	malformed := writeStoreEvent(t, store, Record{Operation: "review/freeze-findings", PreviousRevision: genesis, Transaction: freeze})
	if _, err := freeze.ClassifyEvidence([]FindingEvidence{}); err != nil {
		t.Fatal(err)
	}
	head := writeStoreEvent(t, store, Record{Operation: "review/classify-evidence", PreviousRevision: malformed, Transaction: freeze})
	return store, head, malformed
}

// TestMalformedLegacyFreezeEventStaysConfinedToItsOwnEntry is #1311 after the
// scoping change. One semantically invalid legacy freeze event still marks its
// own lineage invalid and still has no sanctioned exit of its own -- that half
// of #1311 is untouched and the refusals below are its proof. What it no
// longer does is force the WHOLE inventory to complete:false, which is how one
// dead-ended historical lineage used to take every unrelated lineage in the
// repository down with it.
func TestMalformedLegacyFreezeEventStaysConfinedToItsOwnEntry(t *testing.T) {
	repo := initSnapshotRepo(t)
	approvedCompactFixture(t, repo, "legacy-quarantine-unrelated-approved")
	store, _, _ := malformedLegacyFreezeFixture(t, repo, "legacy-freeze-broken")

	_, loadErr := store.LoadChain()
	if loadErr == nil || !errors.Is(loadErr, ErrInvalidSuccessor) ||
		!strings.Contains(loadErr.Error(), "historical findings freeze changed unrelated transaction state") {
		t.Fatalf("malformed freeze load error = %v", loadErr)
	}
	t.Logf("verbatim load failure: %v", loadErr)

	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !hasAuthorityInventoryStatus(report.Entries, "legacy-freeze-broken", AuthorityStatusInvalid) {
		t.Fatalf("the malformed freeze entry is not reported invalid: %#v", report)
	}
	if !report.Complete || !report.Authoritative {
		t.Fatalf("one dead-ended historical lineage still bricks the whole inventory: %#v", report)
	}
	if !hasAuthorityInventoryStatus(report.Entries, "legacy-quarantine-unrelated-approved", AuthorityStatusApproved) {
		t.Fatalf("the unrelated approved lineage lost its entry: %#v", report.Entries)
	}
	for _, entry := range report.Entries {
		if entry.LineageID != "legacy-freeze-broken" {
			continue
		}
		if len(entry.Problems) != 1 || !strings.Contains(entry.Problems[0], "historical findings freeze changed unrelated transaction state") {
			t.Fatalf("bricked entry problems = %#v", entry.Problems)
		}
		t.Logf("verbatim inventory classification: status=%s problems=%q", entry.Status, entry.Problems)
	}

	// Reclaim still refuses (it only quarantines entries that never held
	// authority), but `review abandon` quarantines a legacy-v1-only entry
	// whole, uninterpreted, and leaves every other lineage alone.
	if _, err := ReclaimIncompleteCompactStore(context.Background(), repo, CompactReclaimRequest{
		LineageID: "legacy-freeze-broken", Reason: "retire malformed legacy history", Actor: "maintainer@example.com",
	}); err == nil || !strings.Contains(err.Error(), "inspect reclaim target") {
		t.Fatalf("reclaim refusal = %v", err)
	}
	committed, err := AbandonCompactStore(context.Background(), repo, CompactAbandonRequest{
		LineageID: "legacy-freeze-broken", Reason: "retire malformed legacy history", Actor: "maintainer@example.com",
	})
	if err != nil || committed.Status != CompactReclaimCommitted || len(committed.Cascaded) != 0 {
		t.Fatalf("abandon of the legacy-v1-only entry = %#v, %v", committed, err)
	}
	if proof := committed.Abandonment; proof == nil || !proof.DiscardedWork.Unverifiable {
		t.Fatalf("legacy abandonment proof = %#v", committed.Abandonment)
	}
	if _, statErr := os.Stat(store.Dir); !os.IsNotExist(statErr) {
		t.Fatalf("legacy-v1 entry still present: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(committed.QuarantinePath, "residue")); statErr != nil {
		t.Fatalf("legacy-v1 residue not quarantined: %v", statErr)
	}
	after, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if hasAuthorityInventoryStatus(after.Entries, "legacy-freeze-broken", AuthorityStatusInvalid) ||
		!hasAuthorityInventoryStatus(after.Entries, "legacy-quarantine-unrelated-approved", AuthorityStatusApproved) {
		t.Fatalf("post-abandon inventory = %#v", after.Entries)
	}
}
