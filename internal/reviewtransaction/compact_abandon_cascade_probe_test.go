package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// TestAbandonMidCascadeCommitFailureRerunCompletes: a commit failure on the
// deepest successor surfaces an error, and rerunning the same request finishes.
func TestAbandonMidCascadeCommitFailureRerunCompletes(t *testing.T) {
	repo := initSnapshotRepo(t)
	chain := recoveryChainFixture(t, repo, "abandon-midcascade", 3)
	request := abandonFixtureRequest(chain[1])

	original := persistReclaimRecord
	persistReclaimRecord = func(record CompactReclaimRecord) error {
		if record.Status == CompactReclaimCommitted && record.LineageID == chain[2].State.LineageID {
			return errors.New("simulated mid-cascade commit failure")
		}
		return original(record)
	}
	t.Cleanup(func() { persistReclaimRecord = original })
	if _, err := AbandonCompactStore(context.Background(), repo, request); err == nil {
		t.Fatal("mid-cascade commit failure reported success")
	}
	persistReclaimRecord = original

	result, err := AbandonCompactStore(context.Background(), repo, request)
	if err != nil {
		t.Fatalf("rerun after mid-cascade failure: %v", err)
	}
	if result.Status != CompactReclaimCommitted || result.LineageID != chain[1].State.LineageID {
		t.Fatalf("rerun record = %#v", result.CompactReclaimRecord)
	}
	if !abandonEntryAbsent(t, repo, chain[1].State.LineageID) || !abandonEntryAbsent(t, repo, chain[2].State.LineageID) {
		t.Fatal("rerun left an abandoned entry in the v2 store")
	}
	if abandonEntryAbsent(t, repo, chain[0].State.LineageID) {
		t.Fatal("rerun quarantined the ancestor")
	}
}

// TestAbandonLeafLeavesAncestorsByteIdentical: abandoning the leaf touches no
// ancestor's persisted bytes.
func TestAbandonLeafLeavesAncestorsByteIdentical(t *testing.T) {
	repo := initSnapshotRepo(t)
	chain := recoveryChainFixture(t, repo, "abandon-leaf", 3)
	before := [][]byte{}
	for _, record := range chain[:2] {
		store, err := CompactAuthoritativeStore(context.Background(), repo, record.State.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		payload, err := os.ReadFile(store.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		before = append(before, payload)
	}
	result, err := AbandonCompactStore(context.Background(), repo, abandonFixtureRequest(chain[2]))
	if err != nil || len(result.Cascaded) != 0 || result.LineageID != chain[2].State.LineageID {
		t.Fatalf("leaf abandonment = %#v, %v", result, err)
	}
	for index, record := range chain[:2] {
		store, err := CompactAuthoritativeStore(context.Background(), repo, record.State.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		after, err := os.ReadFile(store.StatePath())
		if err != nil || !bytes.Equal(before[index], after) {
			t.Fatalf("ancestor %s changed by leaf abandonment: %v", record.State.LineageID, err)
		}
	}
}

// TestInventoryIncompleteEntryNamesRunnableAbandonCommand: the printed command
// carries --cwd so it runs from any directory.
func TestInventoryIncompleteEntryNamesRunnableAbandonCommand(t *testing.T) {
	repo := initSnapshotRepo(t)
	root, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root+"/v2/lost-entry", 0o755); err != nil {
		t.Fatal(err)
	}
	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range report.Entries {
		if entry.LineageID == "lost-entry" {
			if len(entry.Problems) != 1 || !strings.Contains(entry.Problems[0], "gentle-ai review abandon --cwd ") || !strings.Contains(entry.Problems[0], " --lineage lost-entry") {
				t.Fatalf("problems = %v", entry.Problems)
			}
			return
		}
	}
	t.Fatalf("entry missing: %#v", report.Entries)
}
