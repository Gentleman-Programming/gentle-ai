package reviewtransaction

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func approvedCompactBurnFixture(t *testing.T, lineage string) (string, string, CompactStore, CompactRecord) {
	t.Helper()
	repo := initSnapshotRepo(t)
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	record, store := approvedCompactFixture(t, repo, lineage)
	return repo, base, store, record
}

func approvedCompactAcknowledgementFixture(t *testing.T, lineage string) (string, string, CompactStore, ApprovedCompactAcknowledgement) {
	t.Helper()
	repo := initSnapshotRepo(t)
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	state, store := startReviewingCompactAuthority(t, repo, newCompactTestState(t, repo, lineage))
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
	}
	state, record := captureAndCompleteCompactReview(t, store, state, CompactReviewInput{
		LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{},
	})
	if err := state.CloseCleanReviewOnLastEvent(); err != nil {
		t.Fatal(err)
	}
	acknowledgement, err := CommitApprovedCompactAcknowledgement(context.Background(), store, record.Revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	return repo, base, store, acknowledgement
}

func TestAcknowledgeApprovedCompactAuthorityRefusesInvalidBindingsWithoutMutation(t *testing.T) {
	const lineage = "pending-acknowledgement-refusal"
	repo, _, store, acknowledgement := approvedCompactAcknowledgementFixture(t, lineage)
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "stale live revision",
			call: func() error {
				return AcknowledgeApprovedCompactAuthority(context.Background(), repo, lineage, acknowledgement.TargetIdentity, hash("0"), acknowledgement.Token)
			},
		},
		{
			name: "wrong target",
			call: func() error {
				return AcknowledgeApprovedCompactAuthority(context.Background(), repo, lineage, hash("f"), acknowledgement.ExpectedRevision, acknowledgement.Token)
			},
		},
		{
			name: "wrong token",
			call: func() error {
				return AcknowledgeApprovedCompactAuthority(context.Background(), repo, lineage, acknowledgement.TargetIdentity, acknowledgement.ExpectedRevision, strings.Repeat("0", 64))
			},
		},
		{
			name: "malformed token",
			call: func() error {
				return AcknowledgeApprovedCompactAuthority(context.Background(), repo, lineage, acknowledgement.TargetIdentity, acknowledgement.ExpectedRevision, "not-a-token")
			},
		},
		{
			name: "wrong lineage",
			call: func() error {
				return AcknowledgeApprovedCompactAuthority(context.Background(), repo, "other-lineage", acknowledgement.TargetIdentity, acknowledgement.ExpectedRevision, acknowledgement.Token)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("invalid acknowledgement succeeded")
			}
			after, err := os.ReadFile(store.StatePath())
			if err != nil || string(after) != string(before) {
				t.Fatalf("invalid acknowledgement mutated authority: %v", err)
			}
		})
	}
}

func TestAcknowledgeApprovedCompactAuthorityBurnsOnceUnderConcurrentReplay(t *testing.T) {
	const lineage = "pending-acknowledgement-concurrent"
	repo, _, store, acknowledgement := approvedCompactAcknowledgementFixture(t, lineage)

	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			results <- AcknowledgeApprovedCompactAuthority(context.Background(), repo, lineage, acknowledgement.TargetIdentity, acknowledgement.ExpectedRevision, acknowledgement.Token)
		}()
	}
	close(start)
	group.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent acknowledgement successes = %d, want 1", successes)
	}
	if _, err := os.Stat(store.StatePath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("successful acknowledgement left authority: %v", err)
	}
}

func TestAcknowledgeApprovedCompactAuthorityFailureKeepsPendingAuthority(t *testing.T) {
	const lineage = "pending-acknowledgement-burn-failure"
	repo, _, store, acknowledgement := approvedCompactAcknowledgementFixture(t, lineage)
	stubStoreResetRemoveTree(t, func(path string) error {
		if path == store.Dir {
			return errors.New("injected authority delete failure")
		}
		return os.RemoveAll(path)
	})

	err := AcknowledgeApprovedCompactAuthority(context.Background(), repo, lineage, acknowledgement.TargetIdentity, acknowledgement.ExpectedRevision, acknowledgement.Token)
	var incomplete *ReviewAuthorityBurnIncompleteError
	if !errors.As(err, &incomplete) {
		t.Fatalf("acknowledgement deletion failure = %v, want incomplete burn", err)
	}
	after, err := store.Load()
	if err != nil || after.Revision != acknowledgement.ExpectedRevision || after.State.ApprovedAckToken != acknowledgement.Token {
		t.Fatalf("failed acknowledgement did not leave the exact pending authority replayable: %#v, %v", after, err)
	}
}

func TestCommitApprovedCompactAcknowledgementFailureBeforeCommitLeavesNoPendingState(t *testing.T) {
	const lineage = "pending-acknowledgement-random-failure"
	_, _, store, record := approvedCompactBurnFixture(t, lineage)
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	original := compactAcknowledgementRandomReader
	compactAcknowledgementRandomReader = strings.NewReader("short")
	t.Cleanup(func() { compactAcknowledgementRandomReader = original })

	if _, err := CommitApprovedCompactAcknowledgement(context.Background(), store, record.Revision, "review/complete-review", record.State); err == nil {
		t.Fatal("randomness failure committed an acknowledgement")
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil || string(after) != string(before) {
		t.Fatalf("pre-commit failure changed authority: %v", err)
	}
}

func TestApprovedCompactAcknowledgementTokenDoesNotLeakOutsideAuthorityOrReturnValue(t *testing.T) {
	const lineage = "pending-acknowledgement-no-leak"
	repo, base, store, acknowledgement := approvedCompactAcknowledgementFixture(t, lineage)
	if err := AcknowledgeApprovedCompactAuthority(context.Background(), repo, lineage, acknowledgement.TargetIdentity, acknowledgement.ExpectedRevision, strings.Repeat("0", 64)); err == nil || strings.Contains(err.Error(), acknowledgement.Token) {
		t.Fatalf("wrong-token error leaked the token: %v", err)
	}
	var tokenPaths []string
	err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(payload), acknowledgement.Token) {
			tokenPaths = append(tokenPaths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tokenPaths) != 1 || tokenPaths[0] != store.StatePath() {
		t.Fatalf("raw acknowledgement token persisted outside compact authority: %v", tokenPaths)
	}
}

// TestAcknowledgeApprovedCompactAuthorityReplayRefusesWithoutLeakingAPath pins
// the refusal shape every other check on this surface already uses. A replayed
// acknowledgement is an ordinary, expected outcome: the authority it names was
// burned by the caller's own previous call. Surfacing the raw *os.PathError
// from the missing state file both leaks the repository layout to whoever reads
// the error and describes the condition as a filesystem problem rather than as
// the already-burned authority it is.
func TestAcknowledgeApprovedCompactAuthorityReplayRefusesWithoutLeakingAPath(t *testing.T) {
	const lineage = "pending-acknowledgement-replay-refusal"
	repo, base, store, acknowledgement := approvedCompactAcknowledgementFixture(t, lineage)

	if err := AcknowledgeApprovedCompactAuthority(context.Background(), repo, lineage,
		acknowledgement.TargetIdentity, acknowledgement.ExpectedRevision, acknowledgement.Token); err != nil {
		t.Fatalf("first acknowledgement: %v", err)
	}
	if _, err := os.Stat(store.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("acknowledgement left authority behind: %v", err)
	}

	err := AcknowledgeApprovedCompactAuthority(context.Background(), repo, lineage,
		acknowledgement.TargetIdentity, acknowledgement.ExpectedRevision, acknowledgement.Token)
	if err == nil {
		t.Fatal("replayed acknowledgement succeeded against burned authority")
	}
	for _, secret := range []string{repo, base, store.Dir, store.StatePath()} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("replayed acknowledgement refusal leaked %q: %v", secret, err)
		}
	}
	if strings.Contains(err.Error(), "no such file or directory") {
		t.Fatalf("replayed acknowledgement refusal surfaced a raw filesystem error: %v", err)
	}
	if !errors.Is(err, ErrApprovedAcknowledgementAuthorityAbsent) {
		t.Fatalf("replayed acknowledgement refusal = %v, want the typed absent-authority refusal", err)
	}
}
