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

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

func TestLegacyOrdinaryMutationRoutesShareTypedReadOnlyErrorWithoutMutation(t *testing.T) {
	tests := []struct {
		name          string
		wantOperation string
		run           func(t *testing.T, repo, lineage, revision string, chain reviewtransaction.ValidatedChain) error
	}{
		{
			name:          "start collision",
			wantOperation: "review/start",
			run: func(t *testing.T, repo, lineage, _ string, _ reviewtransaction.ValidatedChain) error {
				return RunReview([]string{"start", "--cwd", repo, "--lineage", lineage}, &bytes.Buffer{})
			},
		},
		{
			name:          "finalize",
			wantOperation: "review/finalize",
			run: func(t *testing.T, repo, lineage, _ string, _ reviewtransaction.ValidatedChain) error {
				return RunReview([]string{"finalize", "--cwd", repo, "--lineage", lineage}, &bytes.Buffer{})
			},
		},
		{
			name:          "invalidate",
			wantOperation: "review/invalidate",
			run: func(t *testing.T, repo, lineage, revision string, _ reviewtransaction.ValidatedChain) error {
				return RunReview([]string{"invalidate", "--cwd", repo, "--lineage", lineage, "--expected-revision", revision, "--reason", "operator abandoned"}, &bytes.Buffer{})
			},
		},
		{
			name:          "review step",
			wantOperation: "review/freeze-findings",
			run: func(t *testing.T, repo, lineage, _ string, _ reviewtransaction.ValidatedChain) error {
				input := filepath.Join(t.TempDir(), "input.json")
				if err := os.WriteFile(input, []byte("{}\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return RunReviewStep([]string{"--cwd", repo, "--lineage", lineage, "--operation", "freeze-findings", "--input", input}, &bytes.Buffer{})
			},
		},
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

func TestReviewInvalidateDeniesLegacyWithTypedReadOnlyErrorWithoutMutation(t *testing.T) {
	repo := initReviewCLIRepo(t)
	store := addPristineLegacyAuthority(t, repo, "legacy-invalidate-read-only")
	chain, err := store.LoadChain()
	if err != nil {
		t.Fatal(err)
	}
	headPath := filepath.Join(store.Dir, "HEAD")
	eventPath := filepath.Join(store.Dir, "events", chain.HeadRevision[len("sha256:"):]+".json")
	beforeHead, _ := os.ReadFile(headPath)
	beforeEvent, _ := os.ReadFile(eventPath)

	for attempt := 0; attempt < 2; attempt++ {
		var output bytes.Buffer
		err := RunReview([]string{
			"invalidate", "--cwd", repo, "--lineage", "legacy-invalidate-read-only",
			"--expected-revision", chain.HeadRevision, "--reason", "operator abandoned",
		}, &output)
		var typed *reviewtransaction.LegacyReadOnlyError
		if !errors.Is(err, reviewtransaction.ErrLegacyReadOnly) || !errors.As(err, &typed) || typed.Code() != reviewtransaction.LegacyReadOnlyErrorCode || typed.Operation != "review/invalidate" || typed.LineageID != "legacy-invalidate-read-only" {
			t.Fatalf("legacy invalidate error = %#v", err)
		}
		if output.Len() != 0 {
			t.Fatalf("legacy invalidate emitted success output: %s", output.String())
		}
		afterHead, _ := os.ReadFile(headPath)
		afterEvent, _ := os.ReadFile(eventPath)
		if !bytes.Equal(afterHead, beforeHead) || !bytes.Equal(afterEvent, beforeEvent) {
			t.Fatal("legacy invalidate changed authority bytes")
		}
	}
}

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
