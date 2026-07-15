package cli

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

func TestReviewInvalidateFailsClosedForCompetingAuthorities(t *testing.T) {
	for _, corrupt := range []bool{true, false} {
		t.Run(map[bool]string{true: "corrupt compact", false: "dual valid"}[corrupt], func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			started := startFacadeReview(t, repo)
			compact, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
			record, _ := compact.Load()
			legacy := addPristineLegacyAuthority(t, repo, started.LineageID)
			legacyChain, _ := legacy.LoadChain()
			if corrupt && os.WriteFile(compact.StatePath(), []byte("corrupt"), 0o644) != nil {
				t.Fatal("corrupt compact authority")
			}
			expected := record.Revision
			if corrupt {
				expected = legacyChain.HeadRevision
			}
			err := RunReview([]string{"invalidate", "--cwd", repo, "--lineage", started.LineageID, "--expected-revision", expected, "--reason", "operator abandoned"}, &bytes.Buffer{})
			if err == nil {
				t.Fatal("competing authority was mutated")
			}
			chain, loadErr := legacy.LoadChain()
			if loadErr != nil || chain.HeadRevision == "" || (!corrupt && record.Revision != compactRevision(t, compact)) {
				t.Fatalf("authority changed: %v", loadErr)
			}
		})
	}
}

func TestReviewInvalidateHelpLeavesDormantRemediationUnreachable(t *testing.T) {
	var output bytes.Buffer
	if err := RunReview([]string{"invalidate", "--help"}, &output); err != nil {
		t.Fatal(err)
	}
	if output.String() == "" || bytes.Contains(output.Bytes(), []byte("remediation")) || bytes.Contains(output.Bytes(), []byte("archive-sdd")) {
		t.Fatalf("invalidation help activated dormant lifecycle behavior: %q", output.String())
	}
}

func addPristineLegacyAuthority(t *testing.T, repo, lineage string) reviewtransaction.Store {
	t.Helper()
	snapshot, _ := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{}})
	tx, _ := reviewtransaction.NewTransaction(reviewtransaction.Start{LineageID: lineage, Mode: reviewtransaction.ModeOrdinary4R, Generation: 1, Snapshot: snapshot, PolicyHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	_ = tx.StartReview()
	store, _ := reviewtransaction.AuthoritativeStore(context.Background(), repo, lineage)
	if _, err := store.Append("", reviewtransaction.Record{Operation: "review/start", Transaction: *tx}); err != nil {
		t.Fatal(err)
	}
	return store
}

func compactRevision(t *testing.T, store reviewtransaction.CompactStore) string {
	t.Helper()
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return record.Revision
}
