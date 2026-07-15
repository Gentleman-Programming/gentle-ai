package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLegacyCurrentChangesGateRejectsCallerProjectionMismatch(t *testing.T) {
	for _, gate := range []GateKind{GatePostApply, GatePreCommit} {
		t.Run(string(gate), func(t *testing.T) {
			repo := initSnapshotRepo(t)
			writeSnapshotFile(t, repo, "tracked.txt", "reviewed candidate\n")
			transaction, receipt, request := nativeGateFixture(t, repo, "legacy-projection-mismatch-"+string(gate))
			store, err := AuthoritativeStore(context.Background(), repo, transaction.LineageID)
			if err != nil {
				t.Fatal(err)
			}
			appendApprovedStoreChain(t, store, transaction)
			bindGateRequestToStore(t, &request, store)
			request.Gate = gate
			request.Target.Projection = ProjectionStaged

			if got := EvaluateNativeGate(context.Background(), repo, receipt, request); got.Result != GateInvalidated || !strings.Contains(got.Reason, "projection") {
				t.Fatalf("caller-selected projection = %#v", got)
			}
		})
	}
}

func TestCompactStagedPreCommitBindsIndexDespiteWorkspaceDivergence(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "staged candidate\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt")
	state := newCompactStartStateForTarget(t, repo, "compact-staged-precommit", Target{Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{}})
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
	}
	if err := state.CompleteReview(CompactReviewInput{LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-review", state); err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("tests pass\n"), true); err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(record.Revision, "review/complete-verification", state); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("unstaged workspace divergence\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePreCommit, LineageID: state.LineageID}); got.Result != GateAllow {
		t.Fatalf("matching staged index with divergent workspace = %#v", got)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("mutated index\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "--", "tracked.txt")
	if got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePreCommit, LineageID: state.LineageID}); got.Result != GateScopeChanged {
		t.Fatalf("mutated staged index = %#v, want scope changed", got)
	}
}

func TestCompactPreCommitGatePreservesExactStagedIntendedTransition(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "reviewed tracked change\n")
	intended := []string{"first.txt", "second.txt"}
	for _, path := range intended {
		writeSnapshotFile(t, repo, path, "reviewed "+path+"\n")
	}
	state, store, receipt := approvedCompactCurrentChangesFixture(t, repo, "compact-staged-intended", intended)
	if got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePostApply, LineageID: state.LineageID}); got.Result != GateAllow {
		t.Fatalf("unstaged post-apply target = %#v", got)
	}
	gitSnapshot(t, repo, "add", "--", "tracked.txt", "first.txt", "second.txt")
	if stagedTree := strings.TrimSpace(gitSnapshot(t, repo, "write-tree")); stagedTree != receipt.FinalCandidateTree {
		t.Fatalf("staged tree = %s, want approved %s", stagedTree, receipt.FinalCandidateTree)
	}
	indexPath := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "--git-path", "index"))
	beforeIndex, err := os.ReadFile(filepath.Join(repo, indexPath))
	if err != nil {
		t.Fatal(err)
	}
	beforeAuthority, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	beforeRecord, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	input := NativeGateRequestInput{Gate: GatePreCommit, LineageID: state.LineageID}
	first := EvaluateCompactGate(context.Background(), repo, receipt, input)
	second := EvaluateCompactGate(context.Background(), repo, receipt, input)
	if first.Result != GateAllow || !reflect.DeepEqual(first, second) || first.Context.CandidateTree != receipt.FinalCandidateTree || first.Context.PathsDigest != receipt.PathsDigest {
		t.Fatalf("deterministic staged transition = first %#v, second %#v", first, second)
	}
	afterIndex, _ := os.ReadFile(filepath.Join(repo, indexPath))
	afterAuthority, _ := os.ReadFile(store.StatePath())
	afterRecord, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeIndex, afterIndex) || !bytes.Equal(beforeAuthority, afterAuthority) || beforeRecord.Revision != afterRecord.Revision || beforeRecord.State.CorrectionBudget != afterRecord.State.CorrectionBudget {
		t.Fatal("pre-commit validation mutated the index, authority, lineage, or correction budget")
	}

	gitSnapshot(t, repo, "reset", "--", "tracked.txt", "first.txt", "second.txt")
	if got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePostApply, LineageID: state.LineageID}); got.Result != GateAllow {
		t.Fatalf("restored unstaged post-apply target = %#v", got)
	}
}

func TestCompactGateAllowsCorrectedFinalSnapshotWithIntendedUntracked(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := correctedCompactTestStateWithIntended(t, repo, "corrected-final-intended", []string{"new.txt"})
	state.CorrectionAttempts = []CompactCorrectionAttempt{{
		Snapshot: state.CurrentSnapshot, ProposedLines: *state.ProposedCorrectionLines,
		ActualLines: *state.ActualCorrectionLines, FixDeltaHash: state.FixDeltaHash,
		OriginalCriteria: *state.OriginalCriteria, CorrectionRegression: *state.CorrectionRegression,
	}}
	state.CumulativeCorrectionLines = *state.ActualCorrectionLines
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	_, statePayload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), statePayload, 0o644); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if receipt.FinalCandidateTree != state.CurrentSnapshot.CandidateTree || receipt.PathsDigest != state.InitialSnapshot.PathsDigest {
		t.Fatalf("receipt identity = %#v, final snapshot = %#v", receipt, state.CurrentSnapshot)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	leaves, err := CompactAuthorityLeaves(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePostApply, LineageID: state.LineageID, IntendedUntracked: []string{"new.txt"}})
	if got.Result != GateAllow {
		t.Fatalf("corrected intended-untracked gate = %#v", got)
	}
	after, _ := os.ReadFile(store.StatePath())
	updatedLeaves, _ := CompactAuthorityLeaves(context.Background(), repo)
	if !bytes.Equal(before, after) || len(updatedLeaves) != len(leaves) {
		t.Fatal("gate validation mutated authority, lineage count, or budget")
	}
}

func TestCompactPostApplyPreservesExactCommittedApprovedTarget(t *testing.T) {
	tests := []struct {
		name  string
		start func(t *testing.T, repo, lineage string) CompactState
	}{
		{
			name: "current changes",
			start: func(t *testing.T, repo, lineage string) CompactState {
				writeSnapshotFile(t, repo, "tracked.txt", "approved current changes\n")
				return newCompactStartStateForTarget(t, repo, lineage, Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
			},
		},
		{
			name: "base diff",
			start: func(t *testing.T, repo, lineage string) CompactState {
				base := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
				writeSnapshotFile(t, repo, "tracked.txt", "approved committed base diff\n")
				gitSnapshot(t, repo, "add", "tracked.txt")
				gitSnapshot(t, repo, "commit", "-m", "reviewed candidate")
				return newCompactStartStateForTarget(t, repo, lineage, Target{Kind: TargetBaseDiff, BaseRef: base, IntendedUntracked: []string{}})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			state := tt.start(t, repo, "compact-committed-"+strings.ReplaceAll(tt.name, " ", "-"))
			state, receipt := persistApprovedCompactState(t, repo, state)
			if state.InitialSnapshot.Kind == TargetCurrentChanges {
				gitSnapshot(t, repo, "add", "tracked.txt")
				gitSnapshot(t, repo, "commit", "-m", "reviewed candidate")
			}

			got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePostApply, LineageID: state.LineageID})
			replayed := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePostApply, LineageID: state.LineageID})
			if got.Result != GateAllow || !reflect.DeepEqual(got, replayed) || got.Context.BaseTree != state.CurrentSnapshot.BaseTree || got.Context.CandidateTree != state.CurrentSnapshot.CandidateTree || got.Context.PathsDigest != state.CurrentSnapshot.PathsDigest {
				t.Fatalf("exact committed approved target = %#v", got)
			}

			writeSnapshotFile(t, repo, "tracked.txt", "base\n")
			if missing := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePostApply, LineageID: state.LineageID}); missing.Result == GateAllow {
				t.Fatalf("committed target with missing reviewed path = %#v", missing)
			}
			writeSnapshotFile(t, repo, "tracked.txt", "changed after approval\n")
			if changed := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePostApply, LineageID: state.LineageID}); changed.Result == GateAllow {
				t.Fatalf("changed committed tree = %#v", changed)
			}
		})
	}
}

func TestCompactPostApplyRejectsUnboundUntrackedPathAfterCommit(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "approved candidate\n")
	state := newCompactStartStateForTarget(t, repo, "compact-committed-extra-untracked", Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	state, receipt := persistApprovedCompactState(t, repo, state)
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "reviewed candidate")
	writeSnapshotFile(t, repo, "correction-evidence.json", "{}\n")

	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePostApply, LineageID: state.LineageID})
	if got.Result == GateAllow {
		t.Fatalf("unbound untracked path = %#v", got)
	}
	if err := os.Remove(filepath.Join(repo, "correction-evidence.json")); err != nil {
		t.Fatal(err)
	}
	verifyReport := filepath.Join(repo, "openspec", "changes", "thin", "verify-report.md")
	if err := os.MkdirAll(filepath.Dir(verifyReport), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(verifyReport, []byte("# Verification\n\nPASS\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if lifecycle := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePostApply, LineageID: state.LineageID}); lifecycle.Result != GateAllow {
		t.Fatalf("post-review verify report = %#v", lifecycle)
	}
}

func TestCompactFixDiffUsesAuthoritativeCorrectionBindingAcrossDelivery(t *testing.T) {
	t.Run("uncommitted pre-commit", func(t *testing.T) {
		repo, state, receipt, _ := approvedCompactFixDiffFixture(t, "compact-fix-pre-commit")
		gitSnapshot(t, repo, "add", "other.txt")
		got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePreCommit, LineageID: state.LineageID})
		replayed := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePreCommit, LineageID: state.LineageID})
		if got.Result != GateAllow || !reflect.DeepEqual(got, replayed) || got.Context.BaseTree != state.CurrentSnapshot.BaseTree || got.Context.CandidateTree != state.CurrentSnapshot.CandidateTree || got.Context.PathsDigest != state.CurrentSnapshot.PathsDigest {
			t.Fatalf("exact uncommitted correction = %#v", got)
		}
	})

	t.Run("committed pre-push", func(t *testing.T) {
		repo, state, receipt, baseRef := approvedCompactFixDiffFixture(t, "compact-fix-pre-push")
		gitSnapshot(t, repo, "add", "other.txt")
		gitSnapshot(t, repo, "commit", "-m", "approved correction")
		got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePush, LineageID: state.LineageID, BaseRef: baseRef})
		replayed := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePush, LineageID: state.LineageID, BaseRef: baseRef})
		if got.Result != GateAllow || !reflect.DeepEqual(got, replayed) || got.Context.BaseTree != state.CurrentSnapshot.BaseTree || got.Context.CandidateTree != state.CurrentSnapshot.CandidateTree || got.Context.PathsDigest != state.CurrentSnapshot.PathsDigest {
			t.Fatalf("exact committed correction = %#v", got)
		}
	})
}

func TestCompactFixDiffRejectsInexactCorrectionBinding(t *testing.T) {
	tests := []struct {
		name   string
		gate   GateKind
		mutate func(t *testing.T, repo string)
	}{
		{name: "pre-commit extra staged path", gate: GatePreCommit, mutate: func(t *testing.T, repo string) {
			writeSnapshotFile(t, repo, "extra.go", "package extra\n")
			gitSnapshot(t, repo, "add", "extra.go")
		}},
		{name: "pre-commit missing correction", gate: GatePreCommit, mutate: func(t *testing.T, repo string) {
			writeSnapshotFile(t, repo, "other.txt", "reviewed other\n")
		}},
		{name: "pre-commit untracked path", gate: GatePreCommit, mutate: func(t *testing.T, repo string) {
			writeSnapshotFile(t, repo, "extra.json", "{}\n")
		}},
		{name: "pre-push extra committed path", gate: GatePrePush, mutate: func(t *testing.T, repo string) {
			gitSnapshot(t, repo, "add", "other.txt")
			writeSnapshotFile(t, repo, "tracked.txt", "changed outside approved correction\n")
			gitSnapshot(t, repo, "add", "tracked.txt")
			gitSnapshot(t, repo, "commit", "-m", "inexact correction")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, state, receipt, baseRef := approvedCompactFixDiffFixture(t, "compact-inexact-"+strings.ReplaceAll(tt.name, " ", "-"))
			tt.mutate(t, repo)
			got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: tt.gate, LineageID: state.LineageID, BaseRef: baseRef})
			if got.Result == GateAllow {
				t.Fatalf("inexact correction binding = %#v", got)
			}
		})
	}
}

func TestCompactPreCommitGateRejectsInexactStagedIntendedTransitions(t *testing.T) {
	tests := []struct {
		name     string
		prepare  func(t *testing.T, repo string)
		mutate   func(t *testing.T, repo string)
		override []string
	}{
		{name: "changed content", mutate: func(t *testing.T, repo string) {
			writeSnapshotFile(t, repo, "first.txt", "changed after review\n")
			gitSnapshot(t, repo, "add", "--", "first.txt", "second.txt")
		}},
		{name: "changed mode", mutate: func(t *testing.T, repo string) {
			gitSnapshot(t, repo, "config", "core.filemode", "true")
			if err := os.Chmod(filepath.Join(repo, "first.txt"), 0o755); err != nil {
				t.Fatal(err)
			}
			gitSnapshot(t, repo, "add", "--", "first.txt", "second.txt")
		}},
		{name: "additional unreviewed staged path", mutate: func(t *testing.T, repo string) {
			writeSnapshotFile(t, repo, "extra.txt", "not reviewed\n")
			gitSnapshot(t, repo, "add", "--", "first.txt", "second.txt", "extra.txt")
		}},
		{name: "partial staging", mutate: func(t *testing.T, repo string) {
			gitSnapshot(t, repo, "add", "--", "first.txt")
		}},
		{name: "reviewed tracked path left unstaged", prepare: func(t *testing.T, repo string) {
			writeSnapshotFile(t, repo, "tracked.txt", "reviewed tracked change\n")
		}, mutate: func(t *testing.T, repo string) {
			gitSnapshot(t, repo, "add", "--", "first.txt", "second.txt")
		}},
		{name: "removed path", mutate: func(t *testing.T, repo string) {
			if err := os.Remove(filepath.Join(repo, "first.txt")); err != nil {
				t.Fatal(err)
			}
			gitSnapshot(t, repo, "add", "--", "second.txt")
		}},
		{name: "renamed path", mutate: func(t *testing.T, repo string) {
			if err := os.Rename(filepath.Join(repo, "first.txt"), filepath.Join(repo, "renamed.txt")); err != nil {
				t.Fatal(err)
			}
			gitSnapshot(t, repo, "add", "--", "second.txt", "renamed.txt")
		}},
		{name: "replaced path type", mutate: func(t *testing.T, repo string) {
			if err := os.Remove(filepath.Join(repo, "first.txt")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("second.txt", filepath.Join(repo, "first.txt")); err != nil {
				t.Fatal(err)
			}
			gitSnapshot(t, repo, "add", "--", "first.txt", "second.txt")
		}},
		{name: "caller drops frozen intended paths", mutate: func(t *testing.T, repo string) {
			gitSnapshot(t, repo, "add", "--", "first.txt", "second.txt")
		}, override: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			intended := []string{"first.txt", "second.txt"}
			for _, path := range intended {
				writeSnapshotFile(t, repo, path, "reviewed "+path+"\n")
			}
			if tt.prepare != nil {
				tt.prepare(t, repo)
			}
			state, _, receipt := approvedCompactCurrentChangesFixture(t, repo, "compact-inexact-"+strings.ReplaceAll(tt.name, " ", "-"), intended)
			tt.mutate(t, repo)
			input := NativeGateRequestInput{Gate: GatePreCommit, LineageID: state.LineageID}
			if tt.override != nil {
				input.IntendedUntracked = tt.override
			}
			if got := EvaluateCompactGate(context.Background(), repo, receipt, input); got.Result == GateAllow {
				t.Fatalf("inexact staged transition was allowed: %#v", got)
			}
		})
	}
}

func TestCompactPreCommitGateRechecksStagedIntendedTarget(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "new.txt", "reviewed\n")
	state, _, receipt := approvedCompactCurrentChangesFixture(t, repo, "compact-staged-recheck", []string{"new.txt"})
	gitSnapshot(t, repo, "add", "--", "new.txt")
	originalHook := finalGateAuthorizationHook
	finalGateAuthorizationHook = func() {
		writeSnapshotFile(t, repo, "new.txt", "changed during gate\n")
		gitSnapshot(t, repo, "add", "--", "new.txt")
	}
	t.Cleanup(func() { finalGateAuthorizationHook = originalHook })

	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePreCommit, LineageID: state.LineageID})
	if got.Result != GateInvalidated || !strings.Contains(got.Reason, "changed during final authorization") {
		t.Fatalf("staged intended TOCTOU evaluation = %#v", got)
	}
}

func TestCompactGateFinalRecheckRejectsConcurrentUntrackedPath(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "reviewed candidate\n")
	state, _, receipt := approvedCompactCurrentChangesFixture(t, repo, "compact-untracked-recheck", []string{})
	originalHook := finalGateAuthorizationHook
	finalGateAuthorizationHook = func() {
		writeSnapshotFile(t, repo, "late-evidence.json", "{}\n")
	}
	t.Cleanup(func() { finalGateAuthorizationHook = originalHook })

	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePostApply, LineageID: state.LineageID})
	if got.Result != GateInvalidated || !strings.Contains(got.Reason, "changed during final authorization") {
		t.Fatalf("concurrent untracked path = %#v", got)
	}
}

func TestCompactCommittedGateRechecksConcurrentDirtyTrackedTarget(t *testing.T) {
	for _, gate := range []GateKind{GatePostApply, GatePreCommit} {
		t.Run(string(gate), func(t *testing.T) {
			repo := initSnapshotRepo(t)
			writeSnapshotFile(t, repo, "tracked.txt", "reviewed candidate\n")
			state := newCompactStartStateForTarget(t, repo, "compact-dirty-recheck-"+string(gate), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
			state, receipt := persistApprovedCompactState(t, repo, state)
			gitSnapshot(t, repo, "add", "tracked.txt")
			gitSnapshot(t, repo, "commit", "-m", "reviewed candidate")
			if control := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: gate, LineageID: state.LineageID}); control.Result != GateAllow {
				t.Fatalf("unchanged committed target = %#v", control)
			}
			originalHook := finalGateAuthorizationHook
			finalGateAuthorizationHook = func() { writeSnapshotFile(t, repo, "tracked.txt", "concurrent mutation\n") }
			t.Cleanup(func() { finalGateAuthorizationHook = originalHook })
			if got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: gate, LineageID: state.LineageID}); got.Result != GateInvalidated {
				t.Fatalf("concurrent dirty tracked target = %#v", got)
			}
		})
	}
}

func TestCompactReleaseGateUsesIndependentCompleteCurrentEvidence(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "candidate")
	state, store, receipt := approvedCompactRevisionFixture(t, repo, "compact-release")
	dir := t.TempDir()
	paths := map[string]string{}
	for name, content := range map[string]string{
		"configuration": "release configuration\n", "generated": "generated manifest\n",
		"provenance": "release provenance\n", "boundary": "sealed publication boundary\n",
		"freshness": "current release evidence\n",
	} {
		paths[name] = filepath.Join(dir, name)
		if err := os.WriteFile(paths[name], []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	input := NativeGateRequestInput{
		Gate: GateRelease, LineageID: state.LineageID,
		ReleaseConfiguration: paths["configuration"], ReleaseGenerated: paths["generated"],
		ReleaseProvenance: paths["provenance"], ReleasePublicationBoundary: paths["boundary"],
		ReleaseEvidenceFreshness: paths["freshness"],
	}
	if got := EvaluateCompactGate(context.Background(), repo, receipt, input); got.Result != GateAllow || got.Context.Release == nil {
		t.Fatalf("independent compact release evidence = %#v", got)
	}
	if _, err := store.Load(); err != nil {
		t.Fatal(err)
	}

	missing := input
	missing.ReleaseProvenance = ""
	if got := EvaluateCompactGate(context.Background(), repo, receipt, missing); got.Result != GateInvalidated {
		t.Fatalf("missing compact release evidence = %#v", got)
	}
	originalHook := finalGateAuthorizationHook
	finalGateAuthorizationHook = func() {
		finalGateAuthorizationHook = originalHook
		if err := os.WriteFile(paths["freshness"], []byte("tampered after derivation\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { finalGateAuthorizationHook = originalHook })
	if got := EvaluateCompactGate(context.Background(), repo, receipt, input); got.Result != GateInvalidated || !strings.Contains(got.Reason, "release evidence changed") {
		t.Fatalf("tampered compact release evidence = %#v", got)
	}
}

func TestCompactGateRejectsCallerLineageMismatch(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "candidate")
	state, _, receipt := approvedCompactRevisionFixture(t, repo, "compact-lineage-match")
	result := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{
		Gate: GatePrePush, LineageID: "different-lineage",
	})
	if result.Result != GateInvalidated || !strings.Contains(result.Reason, "lineage") {
		t.Fatalf("mismatched compact lineage = %#v for %s", result, state.LineageID)
	}
}

func TestCompactGateFinalRecheckRejectsConcurrentAuthorityAndGitChanges(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(t *testing.T, repo string, store CompactStore, state CompactState, revision string)
	}{
		{name: "Git target", mutate: func(t *testing.T, repo string, _ CompactStore, _ CompactState, _ string) {
			writeSnapshotFile(t, repo, "tracked.txt", "changed during gate\n")
			gitSnapshot(t, repo, "add", "--", "tracked.txt")
		}},
		{name: "authority", mutate: func(t *testing.T, _ string, store CompactStore, state CompactState, revision string) {
			payload, err := os.ReadFile(store.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			var record map[string]any
			if err := json.Unmarshal(payload, &record); err != nil {
				t.Fatal(err)
			}
			record["revision"] = hash("f")
			payload, _ = json.Marshal(record)
			if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
			state := newCompactTestState(t, repo, "compact-final-recheck")
			results := make([]LensResult, len(state.SelectedLenses))
			for index, lens := range state.SelectedLenses {
				results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"review completed"}}
			}
			store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
			revision, _ := store.Replace("", "review/start", state)
			_ = state.CompleteReview(CompactReviewInput{LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{}})
			revision, _ = store.Replace(revision, "review/complete-review", state)
			_ = state.CompleteVerification([]byte("tests pass\n"), true)
			revision, _ = store.Replace(revision, "review/complete-verification", state)
			receipt, _ := state.Receipt()
			_ = WriteCompactReceiptAtomic(store.ReceiptPath(), receipt)
			gitSnapshot(t, repo, "add", "--", "tracked.txt")
			originalHook := finalGateAuthorizationHook
			finalGateAuthorizationHook = func() {
				finalGateAuthorizationHook = originalHook
				tt.mutate(t, repo, store, state, revision)
			}
			t.Cleanup(func() { finalGateAuthorizationHook = originalHook })
			got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePreCommit, LineageID: state.LineageID})
			if got.Result != GateInvalidated || !strings.Contains(got.Reason, "changed during final authorization") {
				t.Fatalf("compact final recheck = %#v", got)
			}
		})
	}
}

func TestCompactPrePRGatePreservesBoundaryContextForExactAndUnavailableSelectors(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "candidate")
	state, _, receipt := approvedCompactRevisionFixture(t, repo, "compact-pre-pr-boundary")
	base := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD^"))
	branch := currentBranch(context.Background(), repo)
	remote := configurePublicationRemote(t, repo, branch)
	gitSnapshot(t, repo, "--git-dir", remote, "branch", "reviewed-base", base)

	exact := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePR, LineageID: state.LineageID, BaseRef: "origin/reviewed-base"})
	if exact.Result != GateAllow || exact.Context.PrePRBoundary == nil || exact.Context.PrePRBoundary.Commit != base {
		t.Fatalf("exact compact pre-PR boundary = %#v", exact)
	}

	unavailable := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePR, LineageID: state.LineageID, BaseRef: "missing-reviewed-base"})
	if unavailable.Result != GateInvalidated || unavailable.Context.LineageID != state.LineageID || unavailable.Context.PrePRBoundary == nil || unavailable.Context.PrePRBoundary.Selector != "missing-reviewed-base" || unavailable.Context.Denial == nil || unavailable.Context.Denial.Code != "unavailable" {
		t.Fatalf("unavailable compact pre-PR boundary = %#v", unavailable)
	}
	payload, err := json.Marshal(unavailable.Context)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseGateContext(payload)
	if err != nil || !reflect.DeepEqual(parsed, unavailable.Context) {
		t.Fatalf("unavailable compact pre-PR context round trip = %#v, %v", parsed, err)
	}

	mismatched := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePR, LineageID: state.LineageID, BaseRef: "origin/" + branch})
	if mismatched.Result == GateAllow || mismatched.Context.PrePRBoundary == nil || mismatched.Context.Denial == nil || mismatched.Context.Denial.Stage != "receipt-binding" {
		t.Fatalf("mismatched compact pre-PR boundary = %#v", mismatched)
	}
}

func TestCompactPrePRGateAllowsFinalSubsetOfGenesisPaths(t *testing.T) {
	repo, state, receipt, baseRef := approvedCompactSubsetDeliveryFixture(t, "compact-subset-pre-pr")
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePR, LineageID: state.LineageID, BaseRef: baseRef})
	if got.Result != GateAllow {
		t.Fatalf("subset pre-PR gate = %#v", got)
	}
}

func TestCompactPrePushAllowsCurrentChangesWithoutTransientCorrectionBaseCommit(t *testing.T) {
	repo, state, receipt, baseRef := approvedCompactSubsetDeliveryFixture(t, "compact-subset-pre-push")
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePush, LineageID: state.LineageID, BaseRef: baseRef})
	if got.Result != GateAllow || !got.Context.BaseRelationshipValid {
		t.Fatalf("pre-push final delivery binding = %#v", got)
	}
}

func TestCompactCorrectedBaseDiffPrePushRejectsIntermediateUnreviewedPath(t *testing.T) {
	repo, state, receipt, baseRef := approvedCompactFixDiffFixture(t, "compact-hidden-range-path")
	writeSnapshotFile(t, repo, "secret.txt", "must never be published\n")
	gitSnapshot(t, repo, "add", "other.txt", "secret.txt")
	gitSnapshot(t, repo, "commit", "-m", "intermediate secret")
	if err := os.Remove(filepath.Join(repo, "secret.txt")); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "-A")
	gitSnapshot(t, repo, "commit", "-m", "remove intermediate secret")
	if got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePush, LineageID: state.LineageID, BaseRef: baseRef}); got.Result == GateAllow {
		t.Fatalf("publication range with hidden path = %#v", got)
	}
}
func TestCompactCorrectedPreCommitBindsStagedIndexAndIgnoresWorkspace(t *testing.T) {
	repo := initSnapshotRepo(t)
	state := correctedCompactTestStateWithIntended(t, repo, "compact-corrected-staged-index", []string{"new.txt"})
	receipt := persistCorrectedCompactFixture(t, repo, state)
	gitSnapshot(t, repo, "add", "tracked.txt", "new.txt")
	writeSnapshotFile(t, repo, "tracked.txt", "unstaged workspace divergence\n")
	writeSnapshotFile(t, repo, "excluded.txt", "outside staged projection\n")
	input := NativeGateRequestInput{Gate: GatePreCommit, LineageID: state.LineageID}
	if got := EvaluateCompactGate(context.Background(), repo, receipt, input); got.Result != GateAllow {
		t.Fatalf("exact staged corrected target = %#v", got)
	}
	gitSnapshot(t, repo, "add", "tracked.txt")
	if got := EvaluateCompactGate(context.Background(), repo, receipt, input); got.Result == GateAllow {
		t.Fatalf("mutated staged correction = %#v", got)
	}
}

func TestCompactCorrectedCurrentChangesPrePushUsesFinalDeliveryBinding(t *testing.T) {
	repo := initSnapshotRepo(t)
	branch := currentBranch(context.Background(), repo)
	configurePublicationRemote(t, repo, branch)
	gitSnapshot(t, repo, "config", "branch."+branch+".remote", "origin")
	gitSnapshot(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
	state := correctedCompactTestStateWithIntended(t, repo, "compact-corrected-current-delivery", []string{})
	receipt := persistCorrectedCompactFixture(t, repo, state)
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "corrected delivery")
	input := NativeGateRequestInput{Gate: GatePrePush, LineageID: state.LineageID, BaseRef: "origin/" + branch}
	if got := EvaluateCompactGate(context.Background(), repo, receipt, input); got.Result != GateAllow {
		t.Fatalf("one-commit corrected delivery = %#v", got)
	}
	gitSnapshot(t, repo, "commit", "--allow-empty", "-m", "unreviewed extra commit")
	if got := EvaluateCompactGate(context.Background(), repo, receipt, input); got.Result == GateAllow {
		t.Fatalf("multi-commit current-changes delivery = %#v", got)
	}
}

func TestCompactDeliveryGateRejectsNonGenesisPath(t *testing.T) {
	repo, state, receipt, baseRef := approvedCompactSubsetDeliveryFixture(t, "compact-new-delivery-path")
	writeSnapshotFile(t, repo, "c.go", "package c\n")
	gitSnapshot(t, repo, "add", "c.go")
	gitSnapshot(t, repo, "commit", "-m", "new path")
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePR, LineageID: state.LineageID, BaseRef: baseRef})
	if got.Result != GateScopeChanged {
		t.Fatalf("non-genesis delivery path = %#v", got)
	}
}

func TestCompactPrePRGateAllowsOnlyAttestedCompatibleSelectedBaseAdvance(t *testing.T) {
	fixture := newCompatiblePrePRFixture(t, "delivery.txt", "base-only.txt")
	state, receipt := approvedCompactPrePRFixture(t, fixture)
	input := NativeGateRequestInput{
		Gate: GatePrePR, LineageID: state.LineageID, BaseRef: "main",
		PolicyArtifact: fixture.request.PolicyArtifact, PrePRCIAttestation: fixture.attestationPath,
	}
	allowed := EvaluateCompactGate(context.Background(), fixture.repo, receipt, input)
	if allowed.Result != GateAllow || allowed.Context.BaseAdvance == nil || !allowed.Context.BaseAdvance.Compatible || allowed.Context.PrePRBoundary == nil || allowed.Context.PrePRBoundary.Commit == fixture.originalBaseCommit {
		t.Fatalf("attested compact compatible advance = %#v", allowed)
	}
	input.PrePRCIAttestation = ""
	if denied := EvaluateCompactGate(context.Background(), fixture.repo, receipt, input); denied.Result == GateAllow {
		t.Fatalf("unattested compact compatible advance = %#v", denied)
	}
}

func TestCompactPrePRGateInvalidatesSelectedBaseAndHeadRaces(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(t *testing.T, fixture *compatiblePrePRFixture)
	}{
		{name: "selected base moves", mutate: func(t *testing.T, fixture *compatiblePrePRFixture) {
			gitSnapshot(t, fixture.repo, "--git-dir", fixture.remote, "update-ref", "refs/heads/main", fixture.originalBaseCommit)
		}},
		{name: "head moves", mutate: func(t *testing.T, fixture *compatiblePrePRFixture) {
			gitSnapshot(t, fixture.repo, "commit", "--allow-empty", "-m", "move head")
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCompatiblePrePRFixture(t, "delivery.txt", "base-only.txt")
			state, receipt := approvedCompactPrePRFixture(t, fixture)
			originalHook := finalGateAuthorizationHook
			finalGateAuthorizationHook = func() {
				finalGateAuthorizationHook = originalHook
				tt.mutate(t, fixture)
			}
			t.Cleanup(func() { finalGateAuthorizationHook = originalHook })
			got := EvaluateCompactGate(context.Background(), fixture.repo, receipt, NativeGateRequestInput{
				Gate: GatePrePR, LineageID: state.LineageID, BaseRef: "main", PolicyArtifact: fixture.request.PolicyArtifact, PrePRCIAttestation: fixture.attestationPath,
			})
			if got.Result != GateInvalidated {
				t.Fatalf("compact %s = %#v", tt.name, got)
			}
		})
	}
}

func approvedCompactRevisionFixture(t *testing.T, repo, lineage string) (CompactState, CompactStore, CompactReceipt) {
	t.Helper()
	state := newCompactRevisionState(t, repo, lineage)
	store, _ := CompactAuthoritativeStore(context.Background(), repo, lineage)
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"review completed"}}
	}
	if err := state.CompleteReview(CompactReviewInput{LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Replace(revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("independent verification passed\n"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-verification", state); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	return state, store, receipt
}

func persistApprovedCompactState(t *testing.T, repo string, state CompactState) (CompactState, CompactReceipt) {
	t.Helper()
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"review completed"}}
	}
	if err := state.CompleteReview(CompactReviewInput{LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Replace(revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("independent verification passed\n"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-verification", state); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	return state, receipt
}

func persistCorrectedCompactFixture(t *testing.T, repo string, state CompactState) CompactReceipt {
	t.Helper()
	state.CorrectionAttempts = []CompactCorrectionAttempt{{
		Snapshot: state.CurrentSnapshot, ProposedLines: *state.ProposedCorrectionLines, ActualLines: *state.ActualCorrectionLines,
		FixDeltaHash: state.FixDeltaHash, OriginalCriteria: *state.OriginalCriteria, CorrectionRegression: *state.CorrectionRegression,
	}}
	state.CumulativeCorrectionLines = *state.ActualCorrectionLines
	store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	_, payload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	return receipt
}

func approvedCompactFixDiffFixture(t *testing.T, lineage string) (string, CompactState, CompactReceipt, string) {
	t.Helper()
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "other.txt", "base other\n")
	gitSnapshot(t, repo, "add", "other.txt")
	gitSnapshot(t, repo, "commit", "-m", "add correction base path")
	base := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	writeSnapshotFile(t, repo, "tracked.txt", "reviewed tracked\n")
	writeSnapshotFile(t, repo, "other.txt", "reviewed other\n")
	gitSnapshot(t, repo, "add", "tracked.txt", "other.txt")
	gitSnapshot(t, repo, "commit", "-m", "reviewed candidate")
	state := newCompactStartStateForTarget(t, repo, lineage, Target{Kind: TargetBaseDiff, BaseRef: base, IntendedUntracked: []string{}})
	finding := Finding{ID: "R3-001", Lens: "reliability", Location: "other.txt:1", Severity: "CRITICAL", Claim: "candidate returns the wrong value", ProofRefs: []string{"differential test fails only on candidate"}}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
		if lens == LensReliability {
			results[index].Findings = []Finding{finding}
		}
	}
	if err := state.CompleteReview(CompactReviewInput{
		LensResults:     results,
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk causes the failure"}},
		RefuterOutcomes: []EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	branch := currentBranch(context.Background(), repo)
	configurePublicationRemote(t, repo, branch)
	gitSnapshot(t, repo, "config", "branch."+branch+".remote", "origin")
	gitSnapshot(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
	writeSnapshotFile(t, repo, "other.txt", "base other\n")
	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetFixDiff, BaseRef: state.InitialSnapshot.CandidateTree,
		IntendedUntracked: []string{}, LedgerIDs: state.FixFindingIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := ScopedValidationResult{
		LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria:     ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: true},
		CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true},
	}
	if err := state.CompleteCorrection(fix, 1, validation); err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("independent correction verification passed\n"), true); err != nil {
		t.Fatal(err)
	}
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	_, payload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	return repo, state, receipt, "origin/" + branch
}

func approvedCompactSubsetDeliveryFixture(t *testing.T, lineage string) (string, CompactState, CompactReceipt, string) {
	t.Helper()
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "b.txt", "base\n")
	gitSnapshot(t, repo, "add", "b.txt")
	gitSnapshot(t, repo, "commit", "-m", "add second base path")
	branch := currentBranch(context.Background(), repo)
	configurePublicationRemote(t, repo, branch)
	gitSnapshot(t, repo, "config", "branch."+branch+".remote", "origin")
	gitSnapshot(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	writeSnapshotFile(t, repo, "b.txt", "candidate\n")
	state := newCompactTestState(t, repo, lineage)
	finding := Finding{ID: "R3-001", Lens: "reliability", Location: "b.txt:1", Severity: "CRITICAL", Claim: "candidate returns the wrong terminal value", ProofRefs: []string{"differential test fails only on candidate"}}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
		if lens == LensReliability {
			results[index].Findings = []Finding{finding}
		}
	}
	if err := state.CompleteReview(CompactReviewInput{
		LensResults:     results,
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk causes the failure"}}, RefuterOutcomes: []EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.BeginCorrection(1); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "b.txt", "base\n")
	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetFixDiff, BaseRef: state.InitialSnapshot.CandidateTree, IntendedUntracked: []string{}, LedgerIDs: state.FixFindingIDs})
	if err != nil {
		t.Fatal(err)
	}
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := ScopedValidationResult{LedgerIDs: state.FixFindingIDs, FixCausedFindings: []Finding{}, FollowUps: []FollowUp{}, OriginalCriteria: ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: true}, CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true}}
	if err := state.CompleteCorrection(fix, 1, validation); err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("tests pass\n"), true); err != nil {
		t.Fatal(err)
	}
	store, _ := CompactAuthoritativeStore(context.Background(), repo, lineage)
	_, payload, _ := makeCompactRecord(state)
	if err := os.MkdirAll(store.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "-A")
	gitSnapshot(t, repo, "commit", "-m", "subset delivery")
	return repo, state, receipt, "origin/" + branch
}

func approvedCompactCurrentChangesFixture(t *testing.T, repo, lineage string, intended []string) (CompactState, CompactStore, CompactReceipt) {
	t.Helper()
	state := newCompactTestStateWithIntended(t, repo, lineage, intended)
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"review completed"}}
	}
	if err := state.CompleteReview(CompactReviewInput{LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Replace(revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("independent verification passed\n"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-verification", state); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	return state, store, receipt
}

func approvedCompactPrePRFixture(t *testing.T, fixture *compatiblePrePRFixture) (CompactState, CompactReceipt) {
	t.Helper()
	snapshot, err := (SnapshotBuilder{Repo: fixture.repo}).Build(context.Background(), Target{Kind: TargetBaseDiff, BaseRef: fixture.originalBaseCommit})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := (SnapshotBuilder{Repo: fixture.repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses := []string{}
	if risk == RiskMedium {
		lenses = []string{LensReliability}
	} else if risk == RiskHigh {
		lenses = append([]string(nil), supportedLenses...)
	}
	policyHash, err := HashArtifact(fixture.request.PolicyArtifact)
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewCompactState(Start{LineageID: "compact-compatible-base", Mode: ModeOrdinaryBounded, Generation: 1, Snapshot: snapshot, PolicyHash: policyHash, RiskLevel: risk, SelectedLenses: lenses, OriginalChangedLines: &lines})
	if err != nil {
		t.Fatal(err)
	}
	store, err := CompactAuthoritativeStore(context.Background(), fixture.repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"review complete"}}
	}
	if err := state.CompleteReview(CompactReviewInput{LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	revision, err = store.Replace(revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("independent verification passed\n"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-verification", state); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	return state, receipt
}
