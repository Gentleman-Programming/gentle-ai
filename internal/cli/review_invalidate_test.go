package cli

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/reviewtransaction"
)

// START is deliberately absent: compact atomic START coexists with retained
// v1/v3 siblings, as TestAtomicStartIgnoresSiblingAuthorityAcrossVersions proves.
func TestLegacyOrdinaryMutationRoutesShareTypedReadOnlyErrorWithoutMutation(t *testing.T) {
	reviewEnabledHome(t)
	tests := []struct {
		name               string
		wantOperation      string
		wantCompactAbsence bool
		run                func(t *testing.T, repo, lineage, revision string, chain reviewtransaction.ValidatedChain) error
	}{
		{
			name:          "direct store append",
			wantOperation: "review/complete-final-verification",
			run: func(t *testing.T, repo, lineage, revision string, chain reviewtransaction.ValidatedChain) error {
				fresh, err := reviewtransaction.AuthoritativeStore(context.Background(), repo, lineage)
				if err != nil {
					t.Fatal(err)
				}
				_, err = fresh.Append(revision, reviewtransaction.Record{Operation: "review/complete-final-verification", Transaction: chain.Records[len(chain.Records)-1].Transaction})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			lineage := "legacy-route-" + strings.ReplaceAll(tt.name, " ", "-")
			store := addPristineLegacyAuthority(t, repo, lineage)
			chain, err := store.LoadChain()
			if err != nil {
				t.Fatal(err)
			}
			before := readLegacyAuthorityTree(t, store.Dir)
			for attempt := 0; attempt < 2; attempt++ {
				err := tt.run(t, repo, lineage, chain.HeadRevision, chain)
				var typed *reviewtransaction.LegacyReadOnlyError
				if !errors.Is(err, reviewtransaction.ErrLegacyReadOnly) || !errors.As(err, &typed) ||
					typed.Code() != reviewtransaction.LegacyReadOnlyErrorCode || typed.Operation != tt.wantOperation || typed.LineageID != lineage {
					t.Fatalf("attempt %d legacy error = %#v", attempt+1, err)
				}
				if after := readLegacyAuthorityTree(t, store.Dir); !reflect.DeepEqual(after, before) {
					t.Fatalf("attempt %d changed legacy authority bytes", attempt+1)
				}
			}
		})
	}
}

// TestReviewInvalidateRefusalIsIdempotentForApprovedLineage supersedes
// TestReviewInvalidateReplaysCompletedApprovedInvalidation: the refusal is a
// pure function of the request (no write, no derivation), so calling it
// twice in a row for the identical approved lineage still produces
// byte-identical output -- the replay-stability claim survives even though
// there is no longer a completed write to replay against.
func TestReviewInvalidateRefusalIsIdempotentForApprovedLineage(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "approved.md", "approved\n", 0o644)
	historical := seedHistoricalCompatibilityApprovedCompactReceipt(t, repo, "invalidate-approved-replay", reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	args := []string{
		"invalidate", "--cwd", repo, "--lineage", historical.Record.State.LineageID,
		"--expected-revision", historical.Record.Revision, "--gate", string(reviewtransaction.GatePostApply),
	}

	var first bytes.Buffer
	firstErr := RunReview(args, &first)
	var second bytes.Buffer
	secondErr := RunReview(args, &second)
	if firstErr == nil || secondErr == nil || firstErr.Error() != secondErr.Error() {
		t.Fatalf("CLI refusal replay changed: first=%v second=%v", firstErr, secondErr)
	}
	if first.String() != second.String() {
		t.Fatalf("CLI replay output changed:\nfirst: %s\nsecond: %s", first.String(), second.String())
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

func readLegacyAuthorityTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[relative] = payload
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return files
}
