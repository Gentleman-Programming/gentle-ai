package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
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

func TestBaseWorkspaceOverlayPreCommitUsesExactStagedIndexWithoutMutation(t *testing.T) {
	repo := initSnapshotRepo(t)
	base := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	writeSnapshotFile(t, repo, "committed.txt", "committed\n")
	gitSnapshot(t, repo, "add", "committed.txt")
	gitSnapshot(t, repo, "commit", "-m", "branch")
	writeSnapshotFile(t, repo, "tracked.txt", "overlay\n")
	state := newCompactStartStateForTarget(t, repo, "overlay-precommit", Target{Kind: TargetBaseWorkspaceOverlay, BaseRef: base, IntendedUntracked: []string{}})
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
	if err := state.CompleteReview(CompactReviewInput{LensResults: results}); err != nil {
		t.Fatal(err)
	}
	if revision, err = store.Replace(revision, "review/complete-review", state); err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("verified\n"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-verification", state); err != nil {
		t.Fatal(err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "tracked.txt")
	beforeIndex := strings.TrimSpace(gitSnapshot(t, repo, "write-tree"))
	beforeStatus := gitSnapshot(t, repo, "status", "--porcelain=v1")
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePreCommit, LineageID: state.LineageID})
	if got.Result != GateAllow || got.Context.BaseTree != state.InitialSnapshot.BaseTree || got.Context.CandidateTree != state.InitialSnapshot.CandidateTree {
		t.Fatalf("overlay pre-commit gate = %#v", got)
	}
	if strings.TrimSpace(gitSnapshot(t, repo, "write-tree")) != beforeIndex || gitSnapshot(t, repo, "status", "--porcelain=v1") != beforeStatus {
		t.Fatal("overlay pre-commit mutated the real index or worktree")
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

func TestCompactPostApplyExemptsExactChangeLocalReceiptMirror(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "approved candidate\n")
	state := newCompactStartStateForTarget(t, repo, "compact-receipt-mirror-exact", Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	state, receipt := persistApprovedCompactState(t, repo, state)
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "reviewed candidate")

	mirror := filepath.Join(repo, "openspec", "changes", "thin", "reviews", "receipt.json")
	if err := os.MkdirAll(filepath.Dir(mirror), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(mirror, receipt); err != nil {
		t.Fatal(err)
	}

	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePostApply, LineageID: state.LineageID})
	if got.Result != GateAllow {
		t.Fatalf("exact change-local receipt mirror = %#v", got)
	}
}

func TestCompactPostApplyRejectsMismatchedChangeLocalReceiptMirror(t *testing.T) {
	tests := []struct {
		name    string
		payload func(t *testing.T, receipt CompactReceipt) []byte
	}{
		{name: "divergent receipt", payload: func(t *testing.T, receipt CompactReceipt) []byte {
			tampered := receipt
			tampered.Generation++
			payload, err := json.MarshalIndent(tampered, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			return append(payload, '\n')
		}},
		{name: "malformed payload", payload: func(t *testing.T, receipt CompactReceipt) []byte {
			return []byte("{\"schema\":\"tampered\"}\n")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			writeSnapshotFile(t, repo, "tracked.txt", "approved candidate\n")
			state := newCompactStartStateForTarget(t, repo, "compact-receipt-mirror-"+strings.ReplaceAll(tt.name, " ", "-"), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
			state, receipt := persistApprovedCompactState(t, repo, state)
			gitSnapshot(t, repo, "add", "tracked.txt")
			gitSnapshot(t, repo, "commit", "-m", "reviewed candidate")

			mirror := filepath.Join(repo, "openspec", "changes", "thin", "reviews", "receipt.json")
			if err := os.MkdirAll(filepath.Dir(mirror), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(mirror, tt.payload(t, receipt), 0o644); err != nil {
				t.Fatal(err)
			}

			got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePostApply, LineageID: state.LineageID})
			if got.Result == GateAllow {
				t.Fatalf("mismatched change-local receipt mirror = %#v", got)
			}
		})
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

func TestCompactCorrectedBaseDiffPrePushAllowsOnlyExactSquashedFullDelivery(t *testing.T) {
	tests := []struct {
		name      string
		extraPath bool
		wrongBase bool
		wantAllow bool
	}{
		{name: "exact genesis delivery", wantAllow: true},
		{name: "extra path", extraPath: true},
		{name: "wrong base", wrongBase: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, state, receipt, baseRef := approvedCompactSquashedFixDiffFixture(t, "compact-squashed-"+strings.ReplaceAll(tt.name, " ", "-"), tt.extraPath, tt.wrongBase)
			input := NativeGateRequestInput{Gate: GatePrePush, LineageID: state.LineageID, BaseRef: baseRef}
			assessment, err := AssessCompactGateTarget(context.Background(), repo, state, input)
			if err != nil {
				t.Fatal(err)
			}
			got := EvaluateCompactGate(context.Background(), repo, receipt, input)
			if tt.wantAllow && (got.Result != GateAllow || !got.Context.BaseRelationshipValid || got.Context.BaseTree != state.InitialSnapshot.BaseTree || got.Context.CandidateTree != receipt.FinalCandidateTree) {
				t.Fatalf("exact squashed full delivery = %#v", got)
			}
			if tt.wantAllow && assessment.Applicability != CompactGateTargetExact {
				t.Fatalf("exact squashed full delivery assessment = %#v", assessment)
			}
			if !tt.wantAllow && (got.Result == GateAllow || assessment.Applicability == CompactGateTargetExact) {
				t.Fatalf("inexact squashed full delivery = %#v", got)
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
			if runtime.GOOS == "windows" && tt.name == "changed mode" {
				t.Skip("Git worktree executable-bit transitions are POSIX-only")
			}
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

func TestCompactCommittedNextSliceWorkspaceRoutesLiveCurrentChanges(t *testing.T) {
	// A committed approved receipt whose candidate tree equals HEAD plus new
	// dirty tracked work is the next-slice topology (#1401). Gate input
	// derivation must construct the live current-changes target so the new
	// work classifies as scope-changed or unrelated instead of failing with
	// "committed approved target has dirty tracked changes".
	tests := []struct {
		name   string
		mutate func(t *testing.T, repo string)
		want   CompactGateTargetApplicability
	}{
		{name: "unrelated dirty tracked path", mutate: func(t *testing.T, repo string) {
			writeSnapshotFile(t, repo, "deleted.txt", "next slice in progress\n")
		}, want: CompactGateTargetUnrelated},
		{name: "overlapping dirty tracked path", mutate: func(t *testing.T, repo string) {
			writeSnapshotFile(t, repo, "tracked.txt", "next slice on reviewed path\n")
		}, want: CompactGateTargetScopeChanged},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			writeSnapshotFile(t, repo, "tracked.txt", "reviewed candidate\n")
			state := newCompactStartStateForTarget(t, repo, "compact-next-slice-"+strings.ReplaceAll(tt.name, " ", "-"), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
			state, receipt := persistApprovedCompactState(t, repo, state)
			gitSnapshot(t, repo, "add", "tracked.txt")
			gitSnapshot(t, repo, "commit", "-m", "reviewed candidate")
			tt.mutate(t, repo)

			input := NativeGateRequestInput{Gate: GatePostApply, LineageID: state.LineageID}
			assessment, err := AssessCompactGateTarget(context.Background(), repo, state, input)
			if err != nil {
				t.Fatalf("next-slice workspace assessment failed input derivation: %v", err)
			}
			if assessment.Applicability != tt.want {
				t.Fatalf("next-slice applicability = %q, want %q", assessment.Applicability, tt.want)
			}

			got := EvaluateCompactGate(context.Background(), repo, receipt, input)
			if got.Result == GateAllow {
				t.Fatalf("committed target with dirty tracked changes was allowed: %#v", got)
			}
			if got.Result != GateScopeChanged || strings.Contains(got.Reason, "cannot be derived") {
				t.Fatalf("next-slice explicit-lineage evaluation = %#v", got)
			}
		})
	}
}

func TestCompactCommittedNextSliceIntendedFilterPropagatesGitInfraFailure(t *testing.T) {
	// A git infrastructure failure while checking whether frozen intended
	// paths remain untracked must fail gate input derivation like every other
	// git call, never silently reclassify the path as untracked.
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "reviewed candidate\n")
	writeSnapshotFile(t, repo, "intended.txt", "reviewed untracked\n")
	state := newCompactStartStateForTarget(t, repo, "compact-next-slice-infra", Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{"intended.txt"}})
	state, receipt := persistApprovedCompactState(t, repo, state)
	gitSnapshot(t, repo, "add", "tracked.txt", "intended.txt")
	gitSnapshot(t, repo, "commit", "-m", "reviewed candidate")
	writeSnapshotFile(t, repo, "deleted.txt", "next slice in progress\n")

	originalStarter := gitProcessTreeStarter
	t.Cleanup(func() { gitProcessTreeStarter = originalStarter })
	gitProcessTreeStarter = func(command *exec.Cmd) (func() error, error) {
		for _, arg := range command.Args {
			if arg == "--error-unmatch" {
				return nil, errors.New("job object creation rejected")
			}
		}
		return originalStarter(command)
	}

	input := NativeGateRequestInput{Gate: GatePostApply, LineageID: state.LineageID}
	_, err := AssessCompactGateTarget(context.Background(), repo, state, input)
	var control *GitProcessControlError
	if !errors.As(err, &control) {
		t.Fatalf("git infra failure during intended filtering was reclassified instead of failing the assessment: %v", err)
	}

	got := EvaluateCompactGate(context.Background(), repo, receipt, input)
	if got.Result != GateInvalidated || !strings.Contains(got.Reason, "cannot be derived") || !errors.As(got.Cause, &control) {
		t.Fatalf("git infra failure evaluation = %#v", got)
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

func TestValidateReviewedPublicationRangeAllowsRepeatedApprovedPath(t *testing.T) {
	repo := initSnapshotRepo(t)
	base := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	writeSnapshotFile(t, repo, "tracked.txt", "red\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "red")
	writeSnapshotFile(t, repo, "tracked.txt", "green\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "green")
	head := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))

	err := validateReviewedPublicationRange(context.Background(), repo, []string{"tracked.txt"}, &resolvedPrePRRefs{
		BaseCommit: base,
		HeadCommit: head,
	})
	if err != nil {
		t.Fatalf("repeated approved publication path: %v", err)
	}
}

func TestValidateReviewedPublicationRangeRejectsHiddenAddedAndRevertedPath(t *testing.T) {
	repo := initSnapshotRepo(t)
	base := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	writeSnapshotFile(t, repo, "secret.txt", "hidden\n")
	gitSnapshot(t, repo, "add", "secret.txt")
	gitSnapshot(t, repo, "commit", "-m", "add hidden path")
	if err := os.Remove(filepath.Join(repo, "secret.txt")); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "-A")
	gitSnapshot(t, repo, "commit", "-m", "revert hidden path")
	head := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))

	err := validateReviewedPublicationRange(context.Background(), repo, []string{"tracked.txt"}, &resolvedPrePRRefs{
		BaseCommit: base,
		HeadCommit: head,
	})
	if err == nil || !strings.Contains(err.Error(), `correction path "secret.txt" is outside immutable genesis scope`) {
		t.Fatalf("hidden added and reverted publication path error = %v", err)
	}
}

func TestValidateReviewedPublicationRangeAllowsRepeatedApprovedPathAcrossMergeHistory(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "approved.txt", "first\nsecond\nthird\nfourth\nfifth\n")
	gitSnapshot(t, repo, "add", "approved.txt")
	gitSnapshot(t, repo, "commit", "-m", "add approved path")
	base := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	branch := currentBranch(context.Background(), repo)

	gitSnapshot(t, repo, "checkout", "-qb", "merge-side")
	writeSnapshotFile(t, repo, "approved.txt", "first\nsecond\nthird\nfourth\nside\n")
	gitSnapshot(t, repo, "add", "approved.txt")
	gitSnapshot(t, repo, "commit", "-m", "side approved change")
	gitSnapshot(t, repo, "checkout", "-q", branch)
	writeSnapshotFile(t, repo, "approved.txt", "main\nsecond\nthird\nfourth\nfifth\n")
	gitSnapshot(t, repo, "add", "approved.txt")
	gitSnapshot(t, repo, "commit", "-m", "main approved change")
	gitSnapshot(t, repo, "merge", "--no-edit", "merge-side")
	head := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))

	err := validateReviewedPublicationRange(context.Background(), repo, []string{"approved.txt"}, &resolvedPrePRRefs{
		BaseCommit: base,
		HeadCommit: head,
	})
	if err != nil {
		t.Fatalf("repeated approved merge-history path: %v", err)
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

func TestCompactBaseWorkspaceOverlayPublicationRejectsIntermediateUnreviewedPath(t *testing.T) {
	for _, gate := range []GateKind{GatePrePush, GatePrePR} {
		t.Run(string(gate), func(t *testing.T) {
			repo := initSnapshotRepo(t)
			base := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
			branch := currentBranch(context.Background(), repo)
			configurePublicationRemote(t, repo, branch)
			gitSnapshot(t, repo, "config", "branch."+branch+".remote", "origin")
			gitSnapshot(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
			writeSnapshotFile(t, repo, "committed.txt", "committed\n")
			gitSnapshot(t, repo, "add", "committed.txt")
			gitSnapshot(t, repo, "commit", "-m", "branch change")
			writeSnapshotFile(t, repo, "tracked.txt", "overlay\n")
			state := newCompactStartStateForTarget(t, repo, "overlay-hidden-range-"+string(gate), Target{
				Kind: TargetBaseWorkspaceOverlay, BaseRef: base, IntendedUntracked: []string{},
			})
			state, receipt := persistApprovedCompactState(t, repo, state)
			gitSnapshot(t, repo, "add", "tracked.txt")
			gitSnapshot(t, repo, "commit", "-m", "overlay delivery")
			writeSnapshotFile(t, repo, "secret.txt", "must never be published\n")
			gitSnapshot(t, repo, "add", "secret.txt")
			gitSnapshot(t, repo, "commit", "-m", "intermediate secret")
			if err := os.Remove(filepath.Join(repo, "secret.txt")); err != nil {
				t.Fatal(err)
			}
			gitSnapshot(t, repo, "add", "-A")
			gitSnapshot(t, repo, "commit", "-m", "remove intermediate secret")

			got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{
				Gate: gate, LineageID: state.LineageID, BaseRef: "origin/" + branch,
			})
			if got.Result == GateAllow || !strings.Contains(got.Reason, "publication range exceeds immutable genesis scope") {
				t.Fatalf("%s publication range with hidden path = %#v", gate, got)
			}
		})
	}
}

func TestCompactBaseWorkspaceOverlayPublicationRejectsMergeResolutionPath(t *testing.T) {
	for _, gate := range []GateKind{GatePrePush, GatePrePR} {
		t.Run(string(gate), func(t *testing.T) {
			repo := initSnapshotRepo(t)
			base := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
			branch := currentBranch(context.Background(), repo)
			configurePublicationRemote(t, repo, branch)
			gitSnapshot(t, repo, "config", "branch."+branch+".remote", "origin")
			gitSnapshot(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
			writeSnapshotFile(t, repo, "tracked.txt", "overlay\n")
			state := newCompactStartStateForTarget(t, repo, "overlay-merge-resolution-"+string(gate), Target{
				Kind: TargetBaseWorkspaceOverlay, BaseRef: base, IntendedUntracked: []string{},
			})
			state, receipt := persistApprovedCompactState(t, repo, state)
			gitSnapshot(t, repo, "add", "tracked.txt")
			gitSnapshot(t, repo, "commit", "-m", "reviewed delivery")
			gitSnapshot(t, repo, "checkout", "-b", "merge-side", base)
			writeSnapshotFile(t, repo, "side.txt", "side\n")
			gitSnapshot(t, repo, "add", "side.txt")
			gitSnapshot(t, repo, "commit", "-m", "side change")
			gitSnapshot(t, repo, "checkout", branch)
			gitSnapshot(t, repo, "merge", "--no-commit", "merge-side")
			writeSnapshotFile(t, repo, "secret.txt", "merge resolution only\n")
			gitSnapshot(t, repo, "add", "secret.txt")
			gitSnapshot(t, repo, "commit", "-m", "merge with resolution path")
			if err := os.Remove(filepath.Join(repo, "secret.txt")); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(repo, "side.txt")); err != nil {
				t.Fatal(err)
			}
			gitSnapshot(t, repo, "add", "-A")
			gitSnapshot(t, repo, "commit", "-m", "final reviewed tree")

			got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{
				Gate: gate, LineageID: state.LineageID, BaseRef: "origin/" + branch,
			})
			if got.Result == GateAllow || !strings.Contains(got.Reason, "publication range exceeds immutable genesis scope") {
				t.Fatalf("%s merge-resolution publication path = %#v", gate, got)
			}
		})
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
	return approvedCompactFixDiffFixtureWithCorrection(t, lineage, "base other\n")
}

func approvedCompactFixDiffFixtureWithCorrection(t *testing.T, lineage, correction string) (string, CompactState, CompactReceipt, string) {
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
	writeSnapshotFile(t, repo, "other.txt", correction)
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

func approvedCompactSquashedFixDiffFixture(t *testing.T, lineage string, extraPath, wrongBase bool) (string, CompactState, CompactReceipt, string) {
	t.Helper()
	repo, state, receipt, baseRef := approvedCompactFixDiffFixtureWithCorrection(t, lineage, "corrected other\n")
	baseCommit := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD^"))
	if extraPath {
		writeSnapshotFile(t, repo, "extra.go", "package extra\n")
	}
	gitSnapshot(t, repo, "add", "-A")
	finalTree := strings.TrimSpace(gitSnapshot(t, repo, "write-tree"))
	publicationBase := baseCommit
	if wrongBase {
		publicationBase = strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", baseCommit+"^"))
	}
	finalCommit := strings.TrimSpace(gitSnapshot(t, repo, "commit-tree", finalTree, "-p", publicationBase, "-m", "squashed full delivery"))
	gitSnapshot(t, repo, "update-ref", "HEAD", finalCommit)
	remote := strings.TrimSpace(gitSnapshot(t, repo, "remote", "get-url", "origin"))
	gitSnapshot(t, repo, "--git-dir", remote, "update-ref", "refs/heads/"+strings.TrimPrefix(baseRef, "origin/"), publicationBase)
	return repo, state, receipt, baseRef
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

func emptyRemoteCompactBaseDiffFixture(t *testing.T, lineage string) (string, CompactState, CompactReceipt) {
	t.Helper()
	repo := t.TempDir()
	gitSnapshot(t, repo, "init")
	gitSnapshot(t, repo, "config", "user.email", "snapshot@example.com")
	gitSnapshot(t, repo, "config", "user.name", "Snapshot Test")
	writeSnapshotFile(t, repo, "tracked.txt", "base\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "base")
	base := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	branch := currentBranch(context.Background(), repo)
	remote := filepath.Join(t.TempDir(), "empty-remote.git")
	gitSnapshot(t, repo, "init", "--bare", remote)
	gitSnapshot(t, repo, "remote", "add", "origin", remote)
	gitSnapshot(t, repo, "config", "branch."+branch+".remote", "origin")
	gitSnapshot(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
	writeSnapshotFile(t, repo, "tracked.txt", "reviewed delivery\n")
	gitSnapshot(t, repo, "commit", "-am", "reviewed delivery")
	state := newCompactStartStateForTarget(t, repo, lineage, Target{Kind: TargetBaseDiff, BaseRef: base, IntendedUntracked: []string{}})
	state, receipt := persistApprovedCompactState(t, repo, state)
	return repo, state, receipt
}

func TestCompactBaseDiffPrePushAllowsFirstPublicationToEmptyRemote(t *testing.T) {
	repo, state, receipt := emptyRemoteCompactBaseDiffFixture(t, "compact-empty-remote-first-publication")
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePush, LineageID: state.LineageID})
	if got.Result != GateAllow || !got.Context.BaseRelationshipValid {
		t.Fatalf("compact first-publication pre-push = %#v", got)
	}
}

func TestCompactCurrentChangesFirstPublicationRejectsUndisclosedAncestorPath(t *testing.T) {
	repo := t.TempDir()
	gitSnapshot(t, repo, "init")
	gitSnapshot(t, repo, "config", "user.email", "snapshot@example.com")
	gitSnapshot(t, repo, "config", "user.name", "Snapshot Test")
	writeSnapshotFile(t, repo, "tracked.txt", "one\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "one")
	writeSnapshotFile(t, repo, "secret.txt", "must never be published\n")
	gitSnapshot(t, repo, "add", "secret.txt")
	gitSnapshot(t, repo, "commit", "-m", "pre-base secret")
	gitSnapshot(t, repo, "rm", "-q", "secret.txt")
	writeSnapshotFile(t, repo, "tracked.txt", "base\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "remove pre-base secret")
	branch := currentBranch(context.Background(), repo)
	remote := filepath.Join(t.TempDir(), "empty-remote.git")
	gitSnapshot(t, repo, "init", "--bare", remote)
	gitSnapshot(t, repo, "remote", "add", "origin", remote)
	gitSnapshot(t, repo, "config", "branch."+branch+".remote", "origin")
	gitSnapshot(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
	writeSnapshotFile(t, repo, "tracked.txt", "reviewed delivery\n")
	state := newCompactStartStateForTarget(t, repo, "compact-bootstrap-undisclosed-ancestor", Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	state, receipt := persistApprovedCompactState(t, repo, state)
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "reviewed delivery")
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePush, LineageID: state.LineageID})
	if got.Result == GateAllow || !strings.Contains(got.Reason, "not disclosed by the reviewed base tree") || !strings.Contains(got.Reason, "squash pre-publication history") {
		t.Fatalf("current-changes bootstrap with undisclosed ancestor path = %#v", got)
	}
}

func TestCompactCurrentChangesFirstPublicationRejectsOverwrittenSecretBlob(t *testing.T) {
	repo := t.TempDir()
	gitSnapshot(t, repo, "init")
	gitSnapshot(t, repo, "config", "user.email", "snapshot@example.com")
	gitSnapshot(t, repo, "config", "user.name", "Snapshot Test")
	writeSnapshotFile(t, repo, "tracked.txt", "SECRET_API_KEY=sk-abc123\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "pre-base secret revision")
	writeSnapshotFile(t, repo, "tracked.txt", "hello\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "overwrite secret before review")
	branch := currentBranch(context.Background(), repo)
	remote := filepath.Join(t.TempDir(), "empty-remote.git")
	gitSnapshot(t, repo, "init", "--bare", remote)
	gitSnapshot(t, repo, "remote", "add", "origin", remote)
	gitSnapshot(t, repo, "config", "branch."+branch+".remote", "origin")
	gitSnapshot(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
	writeSnapshotFile(t, repo, "tracked.txt", "reviewed delivery\n")
	state := newCompactStartStateForTarget(t, repo, "compact-bootstrap-overwritten-secret-blob", Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	state, receipt := persistApprovedCompactState(t, repo, state)
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "reviewed delivery")
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePush, LineageID: state.LineageID})
	if got.Result == GateAllow || !strings.Contains(got.Reason, "publishes pre-base history blob") || !strings.Contains(got.Reason, "tracked.txt") || !strings.Contains(got.Reason, "squash pre-publication history") {
		t.Fatalf("current-changes bootstrap with overwritten secret blob = %#v", got)
	}
}

func TestCompactBaseDiffFirstPublicationAllowsAncestorsDisclosedByBaseTree(t *testing.T) {
	repo := t.TempDir()
	gitSnapshot(t, repo, "init")
	gitSnapshot(t, repo, "config", "user.email", "snapshot@example.com")
	gitSnapshot(t, repo, "config", "user.name", "Snapshot Test")
	writeSnapshotFile(t, repo, "docs/README.md", "pre-existing documentation\n")
	gitSnapshot(t, repo, "add", "docs/README.md")
	gitSnapshot(t, repo, "commit", "-m", "docs")
	writeSnapshotFile(t, repo, "tracked.txt", "base\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "base")
	base := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	branch := currentBranch(context.Background(), repo)
	remote := filepath.Join(t.TempDir(), "empty-remote.git")
	gitSnapshot(t, repo, "init", "--bare", remote)
	gitSnapshot(t, repo, "remote", "add", "origin", remote)
	gitSnapshot(t, repo, "config", "branch."+branch+".remote", "origin")
	gitSnapshot(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
	writeSnapshotFile(t, repo, "tracked.txt", "reviewed delivery\n")
	gitSnapshot(t, repo, "commit", "-am", "reviewed delivery")
	state := newCompactStartStateForTarget(t, repo, "compact-bootstrap-disclosed-ancestors", Target{Kind: TargetBaseDiff, BaseRef: base, IntendedUntracked: []string{}})
	state, receipt := persistApprovedCompactState(t, repo, state)
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePush, LineageID: state.LineageID})
	if got.Result != GateAllow || !got.Context.BaseRelationshipValid {
		t.Fatalf("bootstrap with base tree containing pre-existing unreviewed files = %#v", got)
	}
}

func TestCompactBaseDiffFirstPublicationRejectsHistoryOutsideGenesis(t *testing.T) {
	repo, state, receipt := emptyRemoteCompactBaseDiffFixture(t, "compact-empty-remote-genesis-exceed")
	writeSnapshotFile(t, repo, "secret.txt", "must never be published\n")
	gitSnapshot(t, repo, "add", "secret.txt")
	gitSnapshot(t, repo, "commit", "-m", "intermediate secret")
	if err := os.Remove(filepath.Join(repo, "secret.txt")); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "-A")
	gitSnapshot(t, repo, "commit", "-m", "remove intermediate secret")
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePush, LineageID: state.LineageID})
	if got.Result == GateAllow || !strings.Contains(got.Reason, "publication range exceeds immutable genesis scope") {
		t.Fatalf("first-publication range outside genesis = %#v", got)
	}
}

func TestCompactBaseDiffFirstPublicationRejectsOverwrittenSecretBlob(t *testing.T) {
	repo := t.TempDir()
	gitSnapshot(t, repo, "init")
	gitSnapshot(t, repo, "config", "user.email", "snapshot@example.com")
	gitSnapshot(t, repo, "config", "user.name", "Snapshot Test")
	writeSnapshotFile(t, repo, "tracked.txt", "SECRET_API_KEY=sk-abc123\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "pre-base secret revision")
	writeSnapshotFile(t, repo, "tracked.txt", "hello\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "overwrite secret before review")
	base := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	branch := currentBranch(context.Background(), repo)
	remote := filepath.Join(t.TempDir(), "empty-remote.git")
	gitSnapshot(t, repo, "init", "--bare", remote)
	gitSnapshot(t, repo, "remote", "add", "origin", remote)
	gitSnapshot(t, repo, "config", "branch."+branch+".remote", "origin")
	gitSnapshot(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
	writeSnapshotFile(t, repo, "tracked.txt", "reviewed delivery\n")
	gitSnapshot(t, repo, "commit", "-am", "reviewed delivery")
	state := newCompactStartStateForTarget(t, repo, "compact-base-diff-overwritten-secret-blob", Target{Kind: TargetBaseDiff, BaseRef: base, IntendedUntracked: []string{}})
	state, receipt := persistApprovedCompactState(t, repo, state)
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePush, LineageID: state.LineageID})
	if got.Result == GateAllow || !strings.Contains(got.Reason, "publishes pre-base history blob") || !strings.Contains(got.Reason, "tracked.txt") || !strings.Contains(got.Reason, "squash pre-publication history") {
		t.Fatalf("base-diff bootstrap with overwritten secret blob = %#v", got)
	}
}

// TestCompactBootstrapDisclosureScopeExcludesCommitMetadata pins the same
// documented contract boundary as the native
// TestBootstrapDisclosureScopeExcludesCommitMetadata through the compact
// entry point: disclosure covers repository content — tracked paths and blob
// bytes — and never commit metadata, so a pre-base `--allow-empty` commit
// with a secret message passes both disclosure rules even though its commit
// object is transferred by the bootstrap push. Messages are not review-bound
// anywhere in the gate model (see validateBootstrapAncestryDisclosure);
// closing this boundary requires consciously flipping this test.
func TestCompactBootstrapDisclosureScopeExcludesCommitMetadata(t *testing.T) {
	repo := t.TempDir()
	gitSnapshot(t, repo, "init")
	gitSnapshot(t, repo, "config", "user.email", "snapshot@example.com")
	gitSnapshot(t, repo, "config", "user.name", "Snapshot Test")
	writeSnapshotFile(t, repo, "tracked.txt", "base\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "base")
	gitSnapshot(t, repo, "commit", "--allow-empty", "-m", "SECRET-COMMIT-MESSAGE-MARKER token=hunter2")
	writeSnapshotFile(t, repo, "reviewed-base.txt", "reviewed base\n")
	gitSnapshot(t, repo, "add", "reviewed-base.txt")
	gitSnapshot(t, repo, "commit", "-m", "reviewed base")
	base := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	branch := currentBranch(context.Background(), repo)
	remote := filepath.Join(t.TempDir(), "empty-remote.git")
	gitSnapshot(t, repo, "init", "--bare", remote)
	gitSnapshot(t, repo, "remote", "add", "origin", remote)
	gitSnapshot(t, repo, "config", "branch."+branch+".remote", "origin")
	gitSnapshot(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
	writeSnapshotFile(t, repo, "tracked.txt", "reviewed delivery\n")
	gitSnapshot(t, repo, "commit", "-am", "reviewed delivery")
	state := newCompactStartStateForTarget(t, repo, "compact-bootstrap-commit-metadata-scope", Target{Kind: TargetBaseDiff, BaseRef: base, IntendedUntracked: []string{}})
	state, receipt := persistApprovedCompactState(t, repo, state)
	got := EvaluateCompactGate(context.Background(), repo, receipt, NativeGateRequestInput{Gate: GatePrePush, LineageID: state.LineageID})
	if got.Result != GateAllow || !got.Context.BaseRelationshipValid {
		t.Fatalf("compact bootstrap with secret pre-base commit message = %#v", got)
	}
}
