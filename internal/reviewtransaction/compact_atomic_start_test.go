package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func compactAtomicStartFixture(t *testing.T, repo, lineage string) CompactAtomicStartRequest {
	t.Helper()
	state := newCompactTestState(t, repo, lineage)
	lease, err := OpenRepositoryIdentityLease(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := state.InitialSnapshot
	selector := Target{
		Kind:              snapshot.Kind,
		Projection:        snapshot.Projection,
		IntendedUntracked: append([]string(nil), snapshot.IntendedUntracked...),
		LedgerIDs:         append([]string(nil), snapshot.LedgerIDs...),
	}
	switch snapshot.Kind {
	case TargetBaseDiff, TargetBaseWorkspaceOverlay, TargetFixDiff:
		selector.BaseRef = snapshot.BaseTree
	}
	return CompactAtomicStartRequest{
		State: state,
		Binding: CompactAtomicStartBinding{
			LineageID: lineage, WorktreeIdentity: lease.Identity().RepositoryRef,
			TargetIdentity: snapshot.Identity, Selector: selector, PolicyHash: state.PolicyHash,
			Tier: state.RiskLevel, SelectedLenses: append([]string(nil), state.SelectedLenses...),
			OriginalChangedLines: state.OriginalChangedLines, CorrectionBudget: state.CorrectionBudget,
			CorrectionBudgetPolicy: state.CorrectionBudgetPolicy,
		},
	}
}

func compactAtomicRequestForState(t *testing.T, ctx context.Context, repo string, state CompactState) CompactAtomicStartRequest {
	t.Helper()
	lease, err := OpenRepositoryIdentityLease(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := state.InitialSnapshot
	selector := Target{
		Kind:              snapshot.Kind,
		Projection:        snapshot.Projection,
		IntendedUntracked: append([]string(nil), snapshot.IntendedUntracked...),
		LedgerIDs:         append([]string(nil), snapshot.LedgerIDs...),
	}
	switch snapshot.Kind {
	case TargetBaseDiff, TargetBaseWorkspaceOverlay, TargetFixDiff:
		selector.BaseRef = snapshot.BaseTree
	}
	return CompactAtomicStartRequest{
		State: state,
		Binding: CompactAtomicStartBinding{
			LineageID: state.LineageID, WorktreeIdentity: lease.Identity().RepositoryRef,
			TargetIdentity: snapshot.Identity, Selector: selector, PolicyHash: state.PolicyHash,
			Tier: state.RiskLevel, SelectedLenses: append([]string(nil), state.SelectedLenses...),
			OriginalChangedLines: state.OriginalChangedLines, CorrectionBudget: state.CorrectionBudget,
			CorrectionBudgetPolicy: state.CorrectionBudgetPolicy,
		},
	}
}

func createAtomicCompactAuthority(t *testing.T, ctx context.Context, repo string, state CompactState) (CompactAtomicStartResult, error) {
	t.Helper()
	store, err := CompactAuthoritativeStore(ctx, repo, state.LineageID)
	if err != nil {
		return CompactAtomicStartResult{}, err
	}
	return store.CreateOrReplayAtomicStart(ctx, compactAtomicRequestForState(t, ctx, repo, state))
}

func compactAtomicStartStore(t *testing.T, repo, lineage string) CompactStore {
	t.Helper()
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func compactAtomicStartHash(seed string) string {
	return "sha256:" + strings.Repeat(seed, 64)
}

func TestCompactStoreCreateOrReplayAtomicStartReplaysExactActiveBindingWithoutMutation(t *testing.T) {
	const lineage = "compact-atomic-start-exact-replay"
	repo := initSnapshotRepo(t)
	store := compactAtomicStartStore(t, repo, lineage)
	request := compactAtomicStartFixture(t, repo, lineage)

	created, err := store.CreateOrReplayAtomicStart(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateOrReplayAtomicStart(create): %v", err)
	}
	if created.Replayed {
		t.Fatal("absent authority reported replay")
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	replayed, err := store.CreateOrReplayAtomicStart(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateOrReplayAtomicStart(replay): %v", err)
	}
	if !replayed.Replayed || replayed.Record.Revision != created.Record.Revision {
		t.Fatalf("exact replay = %#v, want record revision %q and replay", replayed, created.Record.Revision)
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("exact atomic START replay mutated review-state.json")
	}
}

func TestCompactStoreCreateOrReplayAtomicStartRefusesEveryBindingMismatchWithoutMutation(t *testing.T) {
	tests := []struct {
		name   string
		field  string
		mutate func(*CompactAtomicStartBinding)
	}{
		{"lineage identity", "lineage_id", func(binding *CompactAtomicStartBinding) { binding.LineageID = "compact-atomic-start-other-lineage" }},
		{"frozen target identity", "target_identity", func(binding *CompactAtomicStartBinding) { binding.TargetIdentity = compactAtomicStartHash("f") }},
		{"selector projection", "selector", func(binding *CompactAtomicStartBinding) { binding.Selector.Projection = ProjectionStaged }},
		{"policy hash", "policy_hash", func(binding *CompactAtomicStartBinding) { binding.PolicyHash = compactAtomicStartHash("f") }},
		{"tier", "tier", func(binding *CompactAtomicStartBinding) { binding.Tier = RiskHigh }},
		{"selected lenses", "selected_lenses", func(binding *CompactAtomicStartBinding) { binding.SelectedLenses = []string{LensRisk} }},
		{"original changed lines", "original_changed_lines", func(binding *CompactAtomicStartBinding) { binding.OriginalChangedLines++ }},
		{"correction budget", "correction_budget", func(binding *CompactAtomicStartBinding) { binding.CorrectionBudget++ }},
		{"correction budget policy", "correction_budget_policy", func(binding *CompactAtomicStartBinding) { binding.CorrectionBudgetPolicy = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const lineage = "compact-atomic-start-binding-conflict"
			repo := initSnapshotRepo(t)
			store := compactAtomicStartStore(t, repo, lineage)
			request := compactAtomicStartFixture(t, repo, lineage)
			if _, err := store.CreateOrReplayAtomicStart(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(store.StatePath())
			if err != nil {
				t.Fatal(err)
			}

			conflicting := request
			conflicting.Binding = cloneCompactAtomicStartBinding(request.Binding)
			tt.mutate(&conflicting.Binding)
			_, err = store.CreateOrReplayAtomicStart(context.Background(), conflicting)
			var conflict *CompactAtomicStartConflictError
			if !errors.As(err, &conflict) || conflict.Field != tt.field {
				t.Fatalf("CreateOrReplayAtomicStart(conflicting %s) error = %T %v, want field %q", tt.name, err, err, tt.field)
			}
			after, err := os.ReadFile(store.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatalf("%s mismatch mutated review-state.json", tt.name)
			}
		})
	}
}

func TestCompactStoreCreateOrReplayAtomicStartRefusesOtherWorktreeWithoutMutation(t *testing.T) {
	const lineage = "compact-atomic-start-worktree-conflict"
	primary := initSnapshotRepo(t)
	linked := filepath.Join(t.TempDir(), "linked-worktree")
	gitSnapshot(t, primary, "worktree", "add", "--detach", linked)
	t.Cleanup(func() { _ = runSnapshotGit(primary, "worktree", "remove", "--force", linked) })

	primaryStore := compactAtomicStartStore(t, primary, lineage)
	request := compactAtomicStartFixture(t, primary, lineage)
	if _, err := primaryStore.CreateOrReplayAtomicStart(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(primaryStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	linkedStore := compactAtomicStartStore(t, linked, lineage)
	linkedRequest := request
	linkedRequest.Binding = compactAtomicStartFixture(t, linked, lineage).Binding
	_, err = linkedStore.CreateOrReplayAtomicStart(context.Background(), linkedRequest)
	var conflict *CompactAtomicStartConflictError
	if !errors.As(err, &conflict) || conflict.Field != "worktree_identity" {
		t.Fatalf("CreateOrReplayAtomicStart(other worktree) error = %T %v, want worktree conflict", err, err)
	}
	after, err := os.ReadFile(primaryStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("other-worktree atomic START mismatch mutated review-state.json")
	}
}

func TestCompactStoreCreateOrReplayAtomicStartFailsClosedForCorruptOrHistoricalAuthority(t *testing.T) {
	tests := []struct {
		name      string
		write     func(*testing.T, CompactStore, CompactAtomicStartRequest)
		assertErr func(error) bool
	}{
		{
			name: "corrupt",
			write: func(t *testing.T, store CompactStore, _ CompactAtomicStartRequest) {
				t.Helper()
				if err := os.MkdirAll(store.Dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(store.StatePath(), []byte("{corrupt"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			assertErr: func(err error) bool {
				var corrupt *CompactAtomicStartCorruptionError
				return errors.As(err, &corrupt)
			},
		},
		{
			name: "historical missing binding",
			write: func(t *testing.T, store CompactStore, request CompactAtomicStartRequest) {
				t.Helper()
				writeCompactFixtureRecord(t, store, request.State)
			},
			assertErr: func(err error) bool {
				var conflict *CompactAtomicStartConflictError
				return errors.As(err, &conflict) && conflict.Field == "initial_atomic_start"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lineage := "compact-atomic-start-" + strings.ReplaceAll(tt.name, " ", "-")
			repo := initSnapshotRepo(t)
			store := compactAtomicStartStore(t, repo, lineage)
			request := compactAtomicStartFixture(t, repo, lineage)
			tt.write(t, store, request)
			before, err := os.ReadFile(store.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			if err := func() error { _, err := store.CreateOrReplayAtomicStart(context.Background(), request); return err }(); !tt.assertErr(err) {
				t.Fatalf("CreateOrReplayAtomicStart error = %T %v", err, err)
			}
			after, err := os.ReadFile(store.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("non-replay atomic START rewrote existing authority")
			}
		})
	}
}

func TestCompactStoreCreateOrReplayAtomicStartLeavesUnrelatedStoresByteIdentical(t *testing.T) {
	repo := initSnapshotRepo(t)
	compactStore := compactAtomicStartStore(t, repo, "compact-atomic-start-unrelated-v2")
	compactRequest := compactAtomicStartFixture(t, repo, "compact-atomic-start-unrelated-v2")
	if _, err := compactStore.CreateOrReplayAtomicStart(context.Background(), compactRequest); err != nil {
		t.Fatal(err)
	}
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	v3Path := filepath.Join(base, "v3", "compact-atomic-start-unrelated-v3", compactStateFileName)
	legacyPath := filepath.Join(base, "v1", "compact-atomic-start-unrelated-legacy", compactStateFileName)
	for path, content := range map[string][]byte{
		v3Path:     []byte("v3 authority bytes\n"),
		legacyPath: []byte("legacy authority bytes\n"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	beforeCompact, err := os.ReadFile(compactStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	beforeV3, err := os.ReadFile(v3Path)
	if err != nil {
		t.Fatal(err)
	}
	beforeLegacy, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}

	store := compactAtomicStartStore(t, repo, "compact-atomic-start-requested")
	request := compactAtomicStartFixture(t, repo, "compact-atomic-start-requested")
	if _, err := store.CreateOrReplayAtomicStart(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	for _, assertion := range []struct {
		path string
		want []byte
	}{
		{compactStore.StatePath(), beforeCompact},
		{v3Path, beforeV3},
		{legacyPath, beforeLegacy},
	} {
		got, err := os.ReadFile(assertion.path)
		if err != nil || !bytes.Equal(got, assertion.want) {
			t.Fatalf("unrelated authority %s changed: %v", assertion.path, err)
		}
	}
}

func TestCompactStoreCreateOrReplayAtomicStartKeepsBindingImmutableAcrossTransition(t *testing.T) {
	const lineage = "compact-atomic-start-immutable-binding"
	repo := initSnapshotRepo(t)
	store := compactAtomicStartStore(t, repo, lineage)
	request := compactAtomicStartFixture(t, repo, lineage)
	created, err := store.CreateOrReplayAtomicStart(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	next := created.Record.State
	binding := cloneCompactAtomicStartBinding(*next.InitialAtomicStart)
	binding.WorktreeIdentity = compactAtomicStartHash("f")
	next.InitialAtomicStart = &binding
	if err := next.CompleteReview(CompactReviewInput{}); err != nil {
		t.Fatal(err)
	}
	_, err = store.Replace(created.Record.Revision, "review/complete-review", next)
	if !errors.Is(err, ErrInvalidSuccessor) {
		t.Fatalf("Replace(changed initial atomic START binding) error = %v, want ErrInvalidSuccessor", err)
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("transition changed immutable initial atomic START binding")
	}
}

func TestCompactStoreCreateOrReplayAtomicStartConvergesConcurrentExactRequests(t *testing.T) {
	const lineage = "compact-atomic-start-concurrent-exact"
	repo := initSnapshotRepo(t)
	store := compactAtomicStartStore(t, repo, lineage)
	request := compactAtomicStartFixture(t, repo, lineage)
	start := make(chan struct{})
	results := make(chan CompactAtomicStartResult, 2)
	errs := make(chan error, 2)
	var callers sync.WaitGroup
	for range 2 {
		callers.Add(1)
		go func() {
			defer callers.Done()
			<-start
			result, err := store.CreateOrReplayAtomicStart(context.Background(), request)
			if err != nil {
				errs <- err
				return
			}
			results <- result
		}()
	}
	close(start)
	callers.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent exact START: %v", err)
	}
	created, replayed := 0, 0
	var revision string
	for result := range results {
		if result.Replayed {
			replayed++
		} else {
			created++
		}
		if revision == "" {
			revision = result.Record.Revision
		} else if result.Record.Revision != revision {
			t.Fatalf("concurrent exact START revisions diverged: %q and %q", revision, result.Record.Revision)
		}
	}
	if created != 1 || replayed != 1 {
		t.Fatalf("concurrent exact START outcomes = created:%d replayed:%d, want created:1 replayed:1", created, replayed)
	}
}

func TestCompactStoreCreateOrReplayAtomicStartRecreatesAfterApprovedAuthorityBurn(t *testing.T) {
	const lineage = "compact-atomic-start-recreate-after-burn"
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	store := compactAtomicStartStore(t, repo, lineage)
	request := compactAtomicStartFixture(t, repo, lineage)
	created, err := store.CreateOrReplayAtomicStart(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	approved := created.Record.State
	for order := range approved.SelectedLenses {
		captureCompactLens(t, store, approved, order)
	}
	captured, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	approved = captured.State
	view, err := approved.CompactReviewView()
	if err != nil {
		t.Fatal(err)
	}
	if err := approved.CompleteReview(CompactReviewInput{LensResults: view.LensResults, RefuterOutcomes: view.RefuterOutcomes}); err != nil {
		t.Fatal(err)
	}
	if err := approved.CloseCleanReviewOnLastEvent(); err != nil {
		t.Fatal(err)
	}
	acknowledgement, err := CommitApprovedCompactAcknowledgement(context.Background(), store, captured.Revision, "review/complete-review", approved)
	if err != nil {
		t.Fatal(err)
	}
	approvedRevision := acknowledgement.ExpectedRevision
	beforeBurn, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateOrReplayAtomicStart(context.Background(), request)
	var conflict *CompactAtomicStartConflictError
	if !errors.As(err, &conflict) || conflict.Field != "state" {
		t.Fatalf("CreateOrReplayAtomicStart(terminal authority) error = %T %v, want state conflict", err, err)
	}
	afterTerminalReplay, err := os.ReadFile(store.StatePath())
	if err != nil || !bytes.Equal(beforeBurn, afterTerminalReplay) {
		t.Fatalf("terminal atomic START replay changed authority: %v", err)
	}
	if err := AcknowledgeApprovedCompactAuthority(context.Background(), repo, lineage, acknowledgement.TargetIdentity, approvedRevision, acknowledgement.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.StatePath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("burn left exact compact authority state: %v", err)
	}

	recreated, err := store.CreateOrReplayAtomicStart(context.Background(), request)
	if err != nil || recreated.Replayed {
		t.Fatalf("CreateOrReplayAtomicStart(after burn) = %#v, %v", recreated, err)
	}
}
