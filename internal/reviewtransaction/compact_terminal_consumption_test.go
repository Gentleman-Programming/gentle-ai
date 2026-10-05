package reviewtransaction

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcknowledgeTerminalConsumptionIsBoundAndNonAuthoritative(t *testing.T) {
	ctx := context.Background()
	repo, base, store, ack := approvedCompactAcknowledgementFixture(t, "terminal-consumption")
	if err := AcknowledgeApprovedCompactAuthority(ctx, repo, ack.LineageID, ack.TargetIdentity, ack.ExpectedRevision, ack.Token); err != nil {
		t.Fatal(err)
	}
	status, err := AssessTargetStatus(ctx, repo, TargetStatusRequest{Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}})
	if err != nil || status.Action != TargetStatusActionStop || status.Replayability != ReplayabilityNotReplayable || status.LineageID != "" || status.AuthorityVersion != "" {
		t.Fatalf("consumed target status = %#v, %v", status, err)
	}
	if stores, err := DiscoverCompactStores(ctx, repo); err != nil || len(stores) != 0 {
		t.Fatalf("terminal fact discovered as authority: %#v, %v", stores, err)
	}
	if _, err := store.Load(); !os.IsNotExist(err) {
		t.Fatalf("burned authority revived: %v", err)
	}
	_, root, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	path := compactTerminalConsumptionPath(base, root, ack.TargetIdentity)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{ack.Token, ack.ExpectedRevision} {
		if strings.Contains(string(payload), secret) {
			t.Fatal("terminal fact retained reusable acknowledgement material")
		}
	}
	if consumed, err := CompactTargetConsumed(ctx, repo, hash("f")); err != nil || consumed {
		t.Fatalf("different target consumed = %v, %v", consumed, err)
	}
	foreign := initSnapshotRepo(t)
	foreignBase, foreignRoot, err := reviewAuthorityRoot(ctx, foreign)
	if err != nil {
		t.Fatal(err)
	}
	foreignPath := compactTerminalConsumptionPath(foreignBase, foreignRoot, ack.TargetIdentity)
	if err := os.MkdirAll(filepath.Dir(foreignPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreignPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if consumed, err := CompactTargetConsumed(ctx, foreign, ack.TargetIdentity); err == nil || consumed {
		t.Fatalf("copied foreign evidence accepted: %v, %v", consumed, err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if consumed, err := CompactTargetConsumed(ctx, repo, ack.TargetIdentity); err == nil || consumed {
		t.Fatalf("malformed evidence accepted: %v, %v", consumed, err)
	}
}

func TestAcknowledgeTerminalConsumptionWriteFailureDoesNotBurn(t *testing.T) {
	repo, base, store, ack := approvedCompactAcknowledgementFixture(t, "terminal-write-failure")
	// A file at the directory boundary deterministically prevents publication.
	if err := os.WriteFile(filepath.Join(base, "terminal-consumption"), []byte("blocked"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AcknowledgeApprovedCompactAuthority(context.Background(), repo, ack.LineageID, ack.TargetIdentity, ack.ExpectedRevision, ack.Token); err == nil {
		t.Fatal("acknowledgement burned without durable consumption evidence")
	}
	record, err := store.Load()
	if err != nil || record.Revision != ack.ExpectedRevision || record.State.ApprovedAckToken != ack.Token {
		t.Fatalf("failed publication changed pending authority: %#v, %v", record, err)
	}
}
