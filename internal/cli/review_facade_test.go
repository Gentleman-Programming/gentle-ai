package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// TestFacadeLensBindingsPublishFindingIDPrefix proves START output carries the
// lens-bound finding-ID prefix admission enforces, in the canonical high-risk
// selection order where lens order and prefix number diverge.
func TestFacadeLensBindingsPublishFindingIDPrefix(t *testing.T) {
	lenses := []string{
		reviewtransaction.LensRisk,
		reviewtransaction.LensResilience,
		reviewtransaction.LensReadability,
		reviewtransaction.LensReliability,
	}
	bindings := facadeLensBindings(lenses)
	if len(bindings) != len(lenses) {
		t.Fatalf("facadeLensBindings() = %#v, want one binding per lens %v", bindings, lenses)
	}
	for index, binding := range bindings {
		want := reviewtransaction.FindingIDPrefixForLens(lenses[index])
		if want == "" || binding.FindingIDPrefix != want {
			t.Fatalf("binding[%d] finding ID prefix = %q, want %q for lens %q", index, binding.FindingIDPrefix, want, lenses[index])
		}
	}
	encoded, err := json.Marshal(bindings[1])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"finding_id_prefix":"R4-"`) {
		t.Fatalf("binding JSON = %s, want published finding_id_prefix R4- for %q", encoded, reviewtransaction.LensResilience)
	}
}

func TestReviewFacadeStartStagedProjectionFreezesOnlyIndex(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "--", "tracked.txt")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("unstaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	indexTree := strings.TrimSpace(runReviewCLIGit(t, repo, "write-tree"))

	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--projection", "staged"}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.Projection != reviewtransaction.ProjectionStaged {
		t.Fatalf("start projection = %q", started.Projection)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.State.InitialSnapshot.Projection != reviewtransaction.ProjectionStaged || record.State.InitialSnapshot.CandidateTree != indexTree {
		t.Fatalf("staged authority = %#v, want index tree %s", record.State.InitialSnapshot, indexTree)
	}
	if record.State.InitialAtomicStart == nil || record.State.InitialAtomicStart.WorktreeIdentity == "" {
		t.Fatalf("staged START omitted compact atomic binding: %#v", record.State.InitialAtomicStart)
	}
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--projection", "future"}, io.Discard); err == nil || !strings.Contains(err.Error(), "unsupported review projection") {
		t.Fatalf("invalid projection error = %v", err)
	}
	runReviewCLIGit(t, repo, "commit", "-qm", "staged candidate")
	// A direct base-diff START that selects a lens remains intentionally
	// uncompletable: only negotiated START publishes repository_context for a
	// reviewer to capture against. The current route must refuse and name that
	// exact negotiated continuation rather than creating legacy authority.
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--projection", "staged", "--base-ref", base, "--committed-only"}, io.Discard); err == nil ||
		!strings.Contains(err.Error(), "require repository_context") || !strings.Contains(err.Error(), "--contract "+ReviewIntegrationContractV2) {
		t.Fatalf("staged projection + base-ref direct START error = %v, want the negotiated repository-context continuation", err)
	}
}

// TestReviewStatusActionEligibilityWithoutContractNamesContractValue proves
// the --contract refusal names the exact escape value rather than only
// describing the flag, for both call sites that emit it (review status and
// review finalize).
func TestReviewStatusActionEligibilityWithoutContractNamesContractValue(t *testing.T) {
	repo := initReviewCLIRepo(t)
	wantContract := "--contract " + ReviewIntegrationContractV1

	statusErr := RunReviewStatus([]string{"--cwd", repo, "--next-transition"}, io.Discard)
	if statusErr == nil || !strings.Contains(statusErr.Error(), wantContract) {
		t.Fatalf("review status --next-transition without --contract error = %v, want it to name %q verbatim", statusErr, wantContract)
	}

	finalizeErr := RunReviewFacadeFinalize([]string{"--cwd", repo, "--action-eligibility"}, io.Discard)
	if finalizeErr == nil || !strings.Contains(finalizeErr.Error(), wantContract) {
		t.Fatalf("review finalize --action-eligibility without --contract error = %v, want it to name %q verbatim", finalizeErr, wantContract)
	}
}

// TestReviewFacadeStagedProjectionBaseRefRefusal proves 1812: combining
// --projection staged with --base-ref (without --workspace-overlay) is
// ambiguous about intent, so it is refused rather than silently freezing the
// index. The refusal cannot guess which of the two real escapes the caller
// meant, so it names both verbatim: the plain staged-projection escape for a
// real staged-index review, and the --committed-only base-diff escape for a
// base-diff review.
func TestReviewFacadeStagedProjectionBaseRefRefusal(t *testing.T) {
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))

	leavesBefore, err := reviewtransaction.CompactAuthorityLeaves(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	callErr := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--projection", "staged", "--base-ref", base, "--committed-only"}, io.Discard)
	wantStagedEscape := "gentle-ai review start --projection staged"
	wantBaseDiffEscape := fmt.Sprintf("gentle-ai review start --base-ref %s --committed-only", base)
	if callErr == nil || !strings.Contains(callErr.Error(), wantStagedEscape) || !strings.Contains(callErr.Error(), wantBaseDiffEscape) {
		t.Fatalf("staged projection + base-ref start error = %v, want it to name both %q and %q verbatim", callErr, wantStagedEscape, wantBaseDiffEscape)
	}
	leavesAfter, err := reviewtransaction.CompactAuthorityLeaves(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(leavesAfter) != len(leavesBefore) {
		t.Fatalf("refused staged projection + base-ref start persisted authority: before=%v after=%v", leavesBefore, leavesAfter)
	}
}

func TestReviewFacadeStartEmitsCaptureBindingInputs(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\ncandidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if started.Action != "created" || started.TargetIdentity != record.State.InitialSnapshot.Identity {
		t.Fatalf("start target identity = %q, want frozen %q (action %q)", started.TargetIdentity, record.State.InitialSnapshot.Identity, started.Action)
	}
	if len(started.SelectedLenses) == 0 || len(started.LensBindings) != len(started.SelectedLenses) {
		t.Fatalf("start lens bindings = %#v, want one per selected lens %v", started.LensBindings, started.SelectedLenses)
	}
	for index, binding := range started.LensBindings {
		if binding.Lens != started.SelectedLenses[index] || binding.Order != index {
			t.Fatalf("lens binding[%d] = %#v, want {%q %d}", index, binding, started.SelectedLenses[index], index)
		}
	}
	replayed := startFacadeReview(t, repo)
	if replayed.Action != "replayed" || replayed.TargetIdentity != started.TargetIdentity ||
		!reflect.DeepEqual(replayed.LensBindings, started.LensBindings) {
		t.Fatalf("replayed binding inputs = %#v, want %#v", replayed, started)
	}
}

// TestReviewFacadeStartStagedProjectionBaseRefContinuationRefused documents a
// 1812 behavior change: combining --projection staged with an explicit
// --base-ref used to continue a prior staged-current-changes authority as a
// base-diff. That combination is now refused as ambiguous intent (see
// TestReviewFacadeStagedProjectionBaseRefRefusal), so the refused staged
// authority stays untouched at its prior revision.
func TestReviewFacadeStartStagedProjectionBaseRefContinuationRefused(t *testing.T) {
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("staged candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "--", "tracked.txt")
	var output bytes.Buffer
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--projection", "staged"}, &output); err != nil {
		t.Fatal(err)
	}
	var staged ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &staged); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, staged.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "commit", "-qm", "staged candidate")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("unstaged divergence\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	wantStagedEscape := "gentle-ai review start --projection staged"
	wantBaseDiffEscape := fmt.Sprintf("gentle-ai review start --base-ref %s --committed-only", base)
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--projection", "staged", "--base-ref", base, "--committed-only", "--lineage", "staged-base-request"}, io.Discard); err == nil ||
		!strings.Contains(err.Error(), wantStagedEscape) || !strings.Contains(err.Error(), wantBaseDiffEscape) {
		t.Fatalf("staged projection + base-ref continuation error = %v, want typed refusal naming both %q and %q (1812)", err, wantStagedEscape, wantBaseDiffEscape)
	}
	after, err := store.Load()
	if err != nil || after.Revision != before.Revision {
		t.Fatalf("refused staged base-ref continuation changed authority = %#v, %v", after, err)
	}
}

func TestReviewFacadeCleanFlowReplacesOneCompactStateAndUsesOnlyReceipt(t *testing.T) {
	// Not parallel: opting in writes the user's global mode through t.Setenv,
	// which Go forbids in a test that also calls t.Parallel.
	reviewEnabledHome(t)

	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate behavior\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	reviewing, err := store.Load()
	if err != nil || reviewing.State.State != reviewtransaction.StateReviewing {
		t.Fatalf("reviewing compact authority = %#v, %v", reviewing, err)
	}
	assertCompactLineageFiles(t, store, []string{"review-state.json"})
	if _, err := os.Stat(filepath.Join(store.Dir, "events")); !os.IsNotExist(err) {
		t.Fatalf("compact start created event history: %v", err)
	}
	legacy, _ := reviewtransaction.AuthoritativeStore(context.Background(), repo, started.LineageID)
	if _, err := legacy.LoadChain(); !os.IsNotExist(err) {
		t.Fatalf("facade start wrote legacy v1 authority: %v", err)
	}

	resultPath := filepath.Join(t.TempDir(), "review.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{Findings: []facadeFinding{}, Evidence: []string{"focused review completed"}})
	var output bytes.Buffer
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--result", resultPath}, &output); err != nil {
		t.Fatal(err)
	}
	validating := decodeFacadeFinalize(t, output.Bytes())
	if validating.State != reviewtransaction.StateValidating || validating.StoreRevision == reviewing.Revision {
		t.Fatalf("validating result = %#v", validating)
	}
	loadedValidating, err := store.Load()
	if err != nil || loadedValidating.State.State != reviewtransaction.StateValidating {
		t.Fatalf("restart validating authority = %#v, %v", loadedValidating, err)
	}
	assertCompactLineageFiles(t, store, []string{"finalize-attempt-journal.json", "review-state.json", "reviewer-results"})

	evidencePath := filepath.Join(t.TempDir(), "tests.txt")
	if err := os.WriteFile(evidencePath, []byte("go test ./...: pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--evidence", evidencePath}, &output); err != nil {
		t.Fatal(err)
	}
	approved := assertApprovedBurnedCompactFacadeFinalize(t, output.Bytes())
	if approved.StoreRevision == reviewing.Revision {
		t.Fatalf("approved burned result did not advance the reviewing authority revision: %#v", approved)
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID}, io.Discard); err == nil {
		t.Fatal("burned compact authority accepted a second FINALIZE")
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
}

func TestReviewFacadeStartRequiresCommittedOnlyAndReusesEquivalentAuthority(t *testing.T) {
	// Not parallel: opting in writes the user's global mode through t.Setenv,
	// which Go forbids in a test that also calls t.Parallel.
	reviewEnabledHome(t)

	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("committed candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "candidate")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("dirty change outside committed target\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--base-ref", base}, io.Discard); err == nil || !strings.Contains(err.Error(), "--committed-only") {
		t.Fatalf("dirty base-ref start error = %v, want committed-only acknowledgement", err)
	}
	if stores, err := reviewtransaction.CompactAuthorityLeaves(context.Background(), repo); err != nil || len(stores) != 0 {
		t.Fatalf("rejected start persisted authority = %v, %v", stores, err)
	}

	// The candidate selects a lens (LensesRequired is asserted true below), and
	// a direct start over --base-ref for a lens-selecting candidate now hits
	// issue #2447's up-front refusal in production (see runReviewFacadeStart).
	// This helper is SETUP for the resume/reuse authority behavior under test
	// (the underlying reviewtransaction.StartCompactAuthority state machine),
	// not the refusal itself, so it constructs legacy authority directly via
	// runLegacyFacadeStartForTest, bypassing the CLI dispatch the refusal lives
	// in entirely.
	start := func(args ...string) ReviewFacadeStartResult {
		t.Helper()
		var output bytes.Buffer
		if err := runLegacyFacadeStartForTest(t, append([]string{"--cwd", repo, "--base-ref", base, "--committed-only"}, args...), &output); err != nil {
			t.Fatal(err)
		}
		var result ReviewFacadeStartResult
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	created := start()
	if created.Action != "created" || !created.LensesRequired {
		t.Fatalf("created start = %#v", created)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, created.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	independent := start("--lineage", "requested-different-lineage")
	if independent.Action != "created" || !independent.LensesRequired || independent.LineageID != "requested-different-lineage" {
		t.Fatalf("different lineage did not create independent authority: %#v", independent)
	}
	if after, err := store.Load(); err != nil || after.Revision != before.Revision || after.State.CorrectionBudget != before.State.CorrectionBudget {
		t.Fatalf("different lineage changed original authority = %#v, %v", after, err)
	}
	replayed := start("--lineage", created.LineageID)
	if replayed.Action != "replayed" || !replayed.LensesRequired || replayed.LineageID != created.LineageID {
		t.Fatalf("exact lineage/binding did not replay authority: %#v", replayed)
	}
	after, err := store.Load()
	if err != nil || after.Revision != before.Revision || after.State.CorrectionBudget != before.State.CorrectionBudget {
		t.Fatalf("exact replay changed compact authority = %#v, %v", after, err)
	}

	policy := filepath.Join(t.TempDir(), "different-policy.md")
	if err := os.WriteFile(policy, []byte("different policy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// An explicit distinct lineage binds the different policy as independent
	// authority; it must not repurpose the original frozen review.
	var independentOutput bytes.Buffer
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--base-ref", base, "--committed-only", "--policy", policy, "--lineage", "policy-different-lineage"}, &independentOutput); err != nil {
		t.Fatal(err)
	}
	var independentPolicy ReviewFacadeStartResult
	if err := json.Unmarshal(independentOutput.Bytes(), &independentPolicy); err != nil {
		t.Fatal(err)
	}
	if independentPolicy.Action != "created" || !independentPolicy.LensesRequired || independentPolicy.LineageID != "policy-different-lineage" {
		t.Fatalf("different policy lineage did not create independent authority = %#v", independentPolicy)
	}
	if unchanged, err := store.Load(); err != nil || unchanged.Revision != before.Revision {
		t.Fatalf("independent policy start changed original authority = %#v, %v", unchanged, err)
	}

	resultPath := filepath.Join(t.TempDir(), "review.json")
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{Findings: []facadeFinding{}, Evidence: []string{"committed target reviewed"}})
	if err := os.WriteFile(evidencePath, []byte("focused tests pass\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var finalizedOutput bytes.Buffer
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--lineage", created.LineageID, "--result", resultPath, "--evidence", evidencePath}, &finalizedOutput); err != nil {
		t.Fatal(err)
	}
	assertApprovedBurnedCompactFacadeFinalize(t, finalizedOutput.Bytes())
	assertApprovedCompactAuthorityBurned(t, store, created.LineageID)

	// Lineage IDs are target-derived, so an equivalent target can retain its
	// canonical key after its prior authority was burned. `created` plus the new
	// reviewing record prove this is a fresh freeze rather than receipt reuse.
	fresh := start()
	if fresh.Action != "created" || !fresh.LensesRequired {
		t.Fatalf("approved equivalent start = %#v, want fresh frozen authority", fresh)
	}
	freshStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, fresh.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	freshRecord, err := freshStore.Load()
	if err != nil || freshRecord.State.State != reviewtransaction.StateReviewing {
		t.Fatalf("fresh equivalent authority = %#v, %v", freshRecord, err)
	}
}

func TestReviewFacadeStartServiceTokenSelectsCanonicalHighRiskLenses(t *testing.T) {
	repo := initReviewCLIRepo(t)
	neutral := filepath.Join(repo, "neutral")
	writeReviewStartCandidate(t, repo, "neutral/service-token.ts", "export const token = 'candidate'\n", 0o644)
	hostile := initReviewCLIRepo(t)
	for name, value := range map[string]string{
		"GIT_DIR": filepath.Join(hostile, ".git"), "GIT_WORK_TREE": hostile,
		"GIT_COMMON_DIR": filepath.Join(hostile, ".git"), "GIT_INDEX_FILE": filepath.Join(hostile, ".git", "index"),
		"GIT_REPLACE_REF_BASE": filepath.Join(hostile, "replace"),
	} {
		t.Setenv(name, value)
	}
	t.Chdir(repo)
	want := []string{reviewtransaction.LensRisk, reviewtransaction.LensResilience, reviewtransaction.LensReadability, reviewtransaction.LensReliability}
	for index, cwd := range []string{repo, neutral, "."} {
		var output bytes.Buffer
		if err := runLegacyFacadeStartForTest(t, []string{"--cwd", cwd, "--lineage", fmt.Sprintf("service-token-%d", index)}, &output); err != nil {
			t.Fatalf("facade start from %q: %v", cwd, err)
		}
		var started ReviewFacadeStartResult
		if err := json.Unmarshal(output.Bytes(), &started); err != nil {
			t.Fatal(err)
		}
		store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		record, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		if record.State.RiskLevel != reviewtransaction.RiskHigh || !reflect.DeepEqual(record.State.SelectedLenses, want) {
			t.Fatalf("facade service-token state from %q = risk %q lenses %v, want high %v", cwd, record.State.RiskLevel, record.State.SelectedLenses, want)
		}
	}
}

func TestReviewFacadeStartProvableShellAndModeRiskSelectsCanonical4R(t *testing.T) {
	t.Parallel()

	want := []string{reviewtransaction.LensRisk, reviewtransaction.LensResilience, reviewtransaction.LensReadability, reviewtransaction.LensReliability}
	tests := []struct {
		name  string
		setup func(t *testing.T, repo string)
	}{
		{
			name: "shell source",
			setup: func(t *testing.T, repo string) {
				writeReviewStartCandidate(t, repo, "run.sh", "printf '%s\\n' safe\n", 0o644)
			},
		},
		{
			name: "GitHub workflow",
			setup: func(t *testing.T, repo string) {
				if err := os.MkdirAll(filepath.Join(repo, ".github", "workflows"), 0o755); err != nil {
					t.Fatal(err)
				}
				writeReviewStartCandidate(t, repo, ".github/workflows/ci.yml", "jobs: {}\n", 0o644)
			},
		},
		{
			name: "mode only",
			setup: func(t *testing.T, repo string) {
				runReviewCLIGit(t, repo, "config", "core.filemode", "true")
				if err := os.Chmod(filepath.Join(repo, "tracked.txt"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if runtime.GOOS == "windows" && tt.name == "mode only" {
				t.Skip("Git worktree executable-bit transitions are POSIX-only")
			}
			repo := initReviewCLIRepo(t)
			tt.setup(t, repo)
			var output bytes.Buffer
			if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo}, &output); err != nil {
				t.Fatal(err)
			}
			var started ReviewFacadeStartResult
			if err := json.Unmarshal(output.Bytes(), &started); err != nil {
				t.Fatal(err)
			}
			if started.RiskLevel != reviewtransaction.RiskHigh || !reflect.DeepEqual(started.SelectedLenses, want) {
				t.Fatalf("start risk/lenses = %q/%v, want high/%v", started.RiskLevel, started.SelectedLenses, want)
			}
		})
	}
}

// TestReviewFacadeStartUnnegotiatedJSONFieldSetRemainsCompatible pins the
// actual pre-change unnegotiated field set. Negotiated-only transport fields
// and the internal compact budget policy must remain absent, while the
// established hint remains part of this exact legacy shape.
func TestReviewFacadeStartUnnegotiatedJSONFieldSetRemainsCompatible(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo}, &output); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	want := []string{"action", "changed_files", "changed_lines", "correction_budget", "hint", "lens_bindings", "lenses_required", "lineage_id", "operation", "projection", "risk_evidence", "risk_level", "selected_lenses", "state", "target_identity"}
	got := make([]string, 0, len(fields))
	for field := range fields {
		got = append(got, field)
	}
	slices.Sort(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unnegotiated start fields = %v, want %v", got, want)
	}
	for _, negotiatedOnly := range []string{"changed_path_manifest", "artifact_subjects", "correction_budget_policy", "schema", "contract"} {
		if _, ok := fields[negotiatedOnly]; ok {
			t.Fatalf("unnegotiated start leaked negotiated-only field %q: %s", negotiatedOnly, output.String())
		}
	}
}

func TestReviewFacadeFinalizeReceiptPublicationFailureIsExactlyReplayable(t *testing.T) {
	reviewEnabledHome(t)
	fixture := prepareFacadeReceiptPending(t)
	if fixture.diagnostic.MutationOutcome != "committed" || fixture.diagnostic.Replayability != "exact_replay_safe" || fixture.diagnostic.LineageID != fixture.started.LineageID || !strings.HasPrefix(fixture.diagnostic.RequestDigest, "sha256:") {
		t.Fatalf("receipt publication diagnostic = %#v", fixture.diagnostic)
	}
	receipt, err := fixture.pending.State.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	pendingAttempt, err := fixture.store.PendingFinalizeAttempt()
	if err != nil || pendingAttempt == nil {
		t.Fatalf("pending finalize attempt = %#v, %v", pendingAttempt, err)
	}
	wantDigest := pendingAttempt.Request.RequestDigest
	if fixture.diagnostic.RequestDigest != wantDigest {
		t.Fatalf("request digest = %q, want %q", fixture.diagnostic.RequestDigest, wantDigest)
	}
	if _, err := os.Stat(fixture.store.ReceiptPath()); !os.IsNotExist(err) {
		t.Fatalf("failed publication created receipt: %v", err)
	}

	original := writeCompactFacadeReceipt
	writes := 0
	writeCompactFacadeReceipt = func(_ context.Context, store reviewtransaction.CompactStore, got reviewtransaction.CompactReceipt) error {
		path := store.ReceiptPath()
		writes++
		if !reflect.DeepEqual(got, receipt) {
			t.Fatalf("replay receipt = %#v, want %#v", got, receipt)
		}
		return reviewtransaction.WriteCompactReceiptAtomic(path, got)
	}
	defer func() { writeCompactFacadeReceipt = original }()
	writeReviewStartCandidate(t, fixture.repo, "tracked.txt", "later worktree drift\n", 0o644)
	var output bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", fixture.repo, "--lineage", fixture.started.LineageID}, &output); err != nil {
		t.Fatalf("exact receipt replay: %v", err)
	}
	if writes != 1 {
		t.Fatalf("receipt replay writes = %d, want 1", writes)
	}
	result := decodeFacadeFinalize(t, output.Bytes())
	if result.LineageID != fixture.started.LineageID || result.StoreRevision != fixture.pending.Revision ||
		result.State != reviewtransaction.StateApproved || result.ReceiptPath != "" {
		t.Fatalf("receipt replay result = %#v, want approved terminal result without receipt path", result)
	}
	assertApprovedCompactAuthorityBurned(t, fixture.store, fixture.started.LineageID)
}

func TestReviewFacadeFinalizeCompletesPendingJournalAfterReceiptPublication(t *testing.T) {
	reviewEnabledHome(t)
	fixture := prepareFacadeReceiptPending(t)
	receipt, err := fixture.pending.State.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.WriteCompactReceiptAtomic(fixture.store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", fixture.repo, "--lineage", fixture.started.LineageID}, &output); err != nil {
		t.Fatalf("lineage-only replay after receipt publication: %v", err)
	}
	result := decodeFacadeFinalize(t, output.Bytes())
	if result.State != reviewtransaction.StateApproved || result.StoreRevision != fixture.pending.Revision || result.ReceiptPath != "" {
		t.Fatalf("receipt replay result = %#v, want approved terminal result without receipt path", result)
	}
	assertApprovedCompactAuthorityBurned(t, fixture.store, fixture.started.LineageID)
}

func TestReviewFacadeFinalizeResyncsCompletedJournalBeforeTerminalReplay(t *testing.T) {
	reviewEnabledHome(t)
	fixture := prepareFacadeReceiptPending(t)
	receipt, err := fixture.pending.State.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.WriteCompactReceiptAtomic(fixture.store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	pending, err := fixture.store.PendingFinalizeAttempt()
	if err != nil || pending == nil {
		t.Fatalf("pending finalize attempt = %#v, %v", pending, err)
	}
	if err := fixture.store.MarkFinalizeAttemptReceiptPublished(pending.Request.RequestDigest); err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.CompleteFinalizeAttempt(pending.Request.RequestDigest); err != nil {
		t.Fatal(err)
	}
	original := reviewFacadeSyncDirectory
	syncs := 0
	reviewFacadeSyncDirectory = func(string) error {
		syncs++
		return nil
	}
	t.Cleanup(func() { reviewFacadeSyncDirectory = original })
	var output bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", fixture.repo, "--lineage", fixture.started.LineageID}, &output); err != nil {
		t.Fatal(err)
	}
	if syncs != 1 {
		t.Fatalf("completed journal directory syncs = %d, want 1", syncs)
	}
	result := decodeFacadeFinalize(t, output.Bytes())
	if result.State != reviewtransaction.StateApproved || result.ReceiptPath != "" {
		t.Fatalf("completed journal terminal result = %#v, want approved without receipt path", result)
	}
	assertApprovedCompactAuthorityBurned(t, fixture.store, fixture.started.LineageID)
}

func TestReviewFacadeFinalizePlannedTransitionInterruptionResumesWithoutDuplicateCommit(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	args := append([]string{"--cwd", repo, "--lineage", started.LineageID}, facadeReviewerResultArgs(t, repo, started)...)
	interrupted := errors.New("interrupt after durable transition plan")
	original := reviewFacadePlannedTransitionHook
	reviewFacadePlannedTransitionHook = func(_ context.Context, _ string, operation, _ string) error {
		if operation == "review/complete-review" {
			return interrupted
		}
		return nil
	}
	t.Cleanup(func() { reviewFacadePlannedTransitionHook = original })
	if err := RunReviewFacadeFinalize(args, io.Discard); !errors.Is(err, interrupted) {
		t.Fatalf("planned transition interruption = %v", err)
	}
	planned, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if planned.Revision != before.Revision || planned.State.State != reviewtransaction.StateReviewing {
		t.Fatalf("planned interruption committed authority: before %#v after %#v", before, planned)
	}
	pending, err := store.PendingFinalizeAttempt()
	if err != nil || pending == nil || len(pending.Transitions) == 0 || pending.Transitions[0].Operation != "review/complete-review" {
		t.Fatalf("planned interruption journal = %#v, %v", pending, err)
	}
	reviewFacadePlannedTransitionHook = original
	writeReviewStartCandidate(t, repo, "tracked.txt", "later worktree drift\n", 0o644)
	if err := RunReviewFacadeFinalize(args, io.Discard); err != nil {
		t.Fatalf("exact replay after planned interruption: %v", err)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.State.State != reviewtransaction.StateValidating || after.Revision == before.Revision {
		t.Fatalf("resumed authority = %#v", after)
	}
	pending, err = store.PendingFinalizeAttempt()
	if err != nil || pending != nil {
		t.Fatalf("successful replay left pending attempt: %#v, %v", pending, err)
	}
}

func TestReviewFacadeFinalizeReceiptReplayRejectsNonExactAndUnsafeRequests(t *testing.T) {
	reviewEnabledHome(t)
	t.Run("explicit lineage is required", func(t *testing.T) {
		fixture := prepareFacadeReceiptPending(t)
		assertFacadeReceiptReplayRejected(t, fixture, []string{"--cwd", fixture.repo})
	})
	t.Run("mutation inputs are rejected", func(t *testing.T) {
		fixture := prepareFacadeReceiptPending(t)
		assertFacadeReceiptReplayRejected(t, fixture, []string{"--cwd", fixture.repo, "--lineage", fixture.started.LineageID, "--evidence", fixture.evidencePath})
	})
	t.Run("malformed existing receipt is not overwritten", func(t *testing.T) {
		fixture := prepareFacadeReceiptPending(t)
		malformed := []byte("malformed receipt\n")
		if err := os.WriteFile(fixture.store.ReceiptPath(), malformed, 0o600); err != nil {
			t.Fatal(err)
		}
		assertFacadeReceiptReplayRejected(t, fixture, []string{"--cwd", fixture.repo, "--lineage", fixture.started.LineageID})
		got, err := os.ReadFile(fixture.store.ReceiptPath())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, malformed) {
			t.Fatalf("unsafe replay overwrote existing receipt: %q", got)
		}
	})
}

func TestReviewFacadeFinalizeTerminalReplaysRejectCapturedEvidence(t *testing.T) {
	reviewEnabledHome(t)
	for _, tt := range []struct {
		name             string
		publishReceipt   bool
		completeTerminal bool
	}{
		{name: "terminal finalize replay", publishReceipt: true},
		{name: "receipt publication replay"},
		{name: "completed terminal replay", publishReceipt: true, completeTerminal: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixture := prepareFacadeReceiptPending(t)
			if tt.publishReceipt {
				receipt, err := fixture.pending.State.Receipt()
				if err != nil {
					t.Fatal(err)
				}
				if err := reviewtransaction.WriteCompactReceiptAtomic(fixture.store.ReceiptPath(), receipt); err != nil {
					t.Fatal(err)
				}
			}
			if tt.completeTerminal {
				pending, err := fixture.store.PendingFinalizeAttempt()
				if err != nil || pending == nil {
					t.Fatalf("pending finalize attempt = %#v, %v", pending, err)
				}
				if err := fixture.store.MarkFinalizeAttemptReceiptPublished(pending.Request.RequestDigest); err != nil {
					t.Fatal(err)
				}
				if err := fixture.store.CompleteFinalizeAttempt(pending.Request.RequestDigest); err != nil {
					t.Fatal(err)
				}
			}
			assertFacadeReceiptReplayRejected(t, fixture, []string{"--cwd", fixture.repo, "--lineage", fixture.started.LineageID, "--captured-evidence"})
		})
	}
}

func TestReviewFacadeStartRejectsInvalidBaseRefWithoutPersistingLineage(t *testing.T) {
	tests := []struct {
		name string
		ref  string
	}{
		{name: "range", ref: "HEAD~1..HEAD"},
		{name: "missing ref", ref: "refs/heads/does-not-exist"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			lineage := "invalid-base-ref"
			err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--lineage", lineage, "--base-ref", tt.ref}, io.Discard)
			if err == nil {
				t.Fatalf("base ref %q was accepted", tt.ref)
			}
			store, storeErr := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
			if storeErr != nil {
				t.Fatal(storeErr)
			}
			if _, statErr := os.Stat(store.Dir); !os.IsNotExist(statErr) {
				t.Fatalf("invalid base ref persisted lineage: %v", statErr)
			}
		})
	}
}

func TestReadFacadeReviewerResultsRejectsNonNativeFields(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "summary replaces claim", payload: `{"findings":[{"location":"x.go:1","severity":"CRITICAL","summary":"incorrect behavior","evidence_class":"deterministic","causal_disposition":"introduced","proof_refs":["test"]}],"evidence":["inspected candidate"]}`},
		{name: "top-level skill resolution", payload: `{"findings":[],"evidence":["inspected candidate"],"skill_resolution":"paths-injected"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "review.json")
			if err := os.WriteFile(path, []byte(tt.payload), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := readFacadeReviewerResults([]string{path}); err == nil || !strings.Contains(err.Error(), "unknown field") {
				t.Fatalf("readFacadeReviewerResults() error = %v, want unknown field", err)
			}
		})
	}
}

func TestReviewFacadeCorrectionFlowResumesFromEachCompactIntermediateState(t *testing.T) {
	// Not parallel: opting in writes the user's global mode through t.Setenv,
	// which Go forbids in a test that also calls t.Parallel.
	reviewEnabledHome(t)

	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	resultPath := filepath.Join(t.TempDir(), "review.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
		Findings: []facadeFinding{{
			Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "candidate returns the wrong terminal value",
			ProofRefs:     []string{"differential test passes on base and fails on candidate"},
			EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
		}},
		Evidence: []string{"focused differential test failed on candidate"},
	})
	var output bytes.Buffer
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--result", resultPath}, &output); err != nil {
		t.Fatal(err)
	}
	if got := decodeFacadeFinalize(t, output.Bytes()); got.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("correction-required result = %#v", got)
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	beforeForecast, _ := store.Load()
	// Re-submitting the reviewer result once the lineage has left reviewing is
	// refused at admission, which is where reviewer results now enter at all.
	// Replaying the already-admitted results through --captured-results is a
	// different operation -- an idempotent no-progress replay -- so it is
	// asserted separately below rather than folded into this refusal.
	for attempt := 0; attempt < 2; attempt++ {
		if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--result", resultPath}, io.Discard); err == nil {
			t.Fatalf("replayed reviewer result attempt %d was accepted after reviewing ended", attempt+1)
		}
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--captured-results=true"}, io.Discard); err != nil {
		t.Fatalf("idempotent captured-result replay = %v", err)
	}
	afterRejectedReplay, _ := store.Load()
	if afterRejectedReplay.Revision != beforeForecast.Revision || !reflect.DeepEqual(afterRejectedReplay.State, beforeForecast.State) {
		t.Fatal("rejected reviewer result replay changed authority")
	}
	classification := beforeForecast.State.Classifications["R3-001"]
	if classification.Causality != reviewtransaction.CausalIntroduced || beforeForecast.State.Outcomes["R3-001"] != reviewtransaction.OutcomeCorroborated || !reflect.DeepEqual(beforeForecast.State.FixFindingIDs, []string{"R3-001"}) {
		t.Fatalf("compact causal admission = %#v", beforeForecast.State)
	}
	ledgerFromState, err := reviewtransaction.CanonicalLedger(beforeForecast.State.Findings)
	if err != nil {
		t.Fatal(err)
	}
	ledgerFromLens, err := reviewtransaction.CanonicalLedger(beforeForecast.State.LensResults[0].Findings)
	if err != nil || !bytes.Equal(ledgerFromState, ledgerFromLens) {
		t.Fatalf("native compact ledger derivation differs: %v", err)
	}

	output.Reset()
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--correction-lines", "2"}, &output); err != nil {
		t.Fatal(err)
	}
	forecasted, _ := store.Load()
	if forecasted.Revision == beforeForecast.Revision || forecasted.State.ProposedCorrectionLines == nil || *forecasted.State.ProposedCorrectionLines != 2 {
		t.Fatalf("forecasted compact authority = %#v", forecasted)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var statusOutput bytes.Buffer
	if err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo, "--lineage", started.LineageID}, &statusOutput); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	if status.Authority == nil || status.Authority.LineageID != started.LineageID || status.Authority.State != reviewtransaction.StateCorrectionRequired ||
		status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect || status.NextTransition.ReasonCode != "correction_repository_verification_required" {
		t.Fatalf("correction status did not continue the explicit active lineage: %#v", status)
	}
	correction, _ := store.Load()
	if correction.State.CorrectionBudget != 2 || correction.State.ProposedCorrectionLines == nil || *correction.State.ProposedCorrectionLines != 2 {
		t.Fatalf("resumed correction budget = %#v", correction.State)
	}
	fix, err := facadeVerificationEvidenceTarget(context.Background(), repo, correction.State, correction.Revision)
	if err != nil {
		t.Fatal(err)
	}
	request, err := reviewtransaction.BuildTargetedValidationRequest(context.Background(), repo, correction.State, correction.Revision)
	if err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidencePath, []byte("focused and full tests: pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureEvidence([]string{"--cwd", repo, "--lineage", started.LineageID,
		"--target", fix.Identity, "--expected-revision", correction.Revision,
		"--outcome", string(reviewtransaction.VerificationOutcomePassed), "--input", evidencePath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	validationPath := filepath.Join(t.TempDir(), "validation.json")
	writeReviewCLIJSON(t, validationPath, facadeValidationResult{
		TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: request.CorrectionTargetIdentity,
		OriginalCriteria:     facadeValidationCheck{Passed: true, Evidence: []string{"original acceptance test passed"}},
		CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"targeted regression test passed"}},
		FollowUps:            []reviewtransaction.FollowUp{},
	})
	output.Reset()
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--validation", validationPath, "--captured-evidence"}, &output); err != nil {
		t.Fatal(err)
	}
	finalized := decodeFacadeFinalize(t, output.Bytes())
	if finalized.State != reviewtransaction.StateApproved || finalized.Action != reviewApprovedCompactBurnedFinalizeAction || finalized.ReceiptPath != "" {
		t.Fatalf("corrected approved result = %#v", finalized)
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
}

// TestReviewFacadeRefusesFalseIntroducedFindingOutsideGenesis covers a claim
// the product used to accept and then downgrade at finalize: a CRITICAL finding
// asserted as candidate-introduced while sitting on an unchanged production
// path.
//
// It is now refused at admission instead, by the same repository-derived
// changed-line evidence finalize used to consult, and the refusal never
// consumes the lens slot or moves authority. Refusing an unprovable causal
// claim before it enters authority is strictly stronger than recording it and
// escalating afterwards, so this test asserts the refusal and the untouched
// authority rather than the escalated state that route no longer reaches.
func TestReviewFacadeRefusesFalseIntroducedFindingOutsideGenesis(t *testing.T) {
	t.Parallel()

	repo := initReviewCLIRepo(t)
	legacyDir := filepath.Join(repo, "internal", "legacy")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "unsafe.go"), []byte("package legacy\n\nfunc ParseLimit(value int) int {\n\tif value < 0 { panic(\"negative limit\") }\n\treturn value\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "internal/legacy/unsafe.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "add unchanged production path")
	// The candidate touches the legacy file so its path is inside the frozen
	// manifest; line 4 is left exactly as committed, so the causal claim below is
	// about a line the candidate never changed.
	if err := os.WriteFile(filepath.Join(legacyDir, "unsafe.go"), []byte("package legacy\n\nfunc ParseLimit(value int) int {\n\tif value < 0 { panic(\"negative limit\") }\n\treturn value\n}\n\nfunc Version() string { return \"v2\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeReviewStartCandidate(t, repo, "internal/candidate/feature.go", "package candidate\n\nfunc Enabled() bool { return true }\n", 0o644)
	started := startFacadeReview(t, repo)
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	reviewer := filepath.Join(t.TempDir(), "reviewer.json")
	writeReviewCLIJSON(t, reviewer, facadeReviewerResult{Findings: []facadeFinding{{
		Location: "internal/legacy/unsafe.go:4", Severity: "CRITICAL", Claim: "negative input panics in the unchanged parser",
		ProofRefs: []string{"the frozen candidate deterministically reproduces the panic"}, EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, Evidence: []string{"reproduced the defect without candidate-causality evidence"}})
	var output bytes.Buffer
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--result", reviewer}, &output); err == nil {
		t.Fatal("unprovable candidate-causal claim was admitted")
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || after.State.State != reviewtransaction.StateReviewing {
		t.Fatalf("refused causal claim moved authority: before=%s after=%#v", before.Revision, after.State)
	}
	if len(after.State.Classifications) != 0 || len(after.State.Outcomes) != 0 {
		t.Fatalf("refused causal claim entered authority: %#v", after.State)
	}
	// Both candidate paths are genesis; the finding cited an unchanged LINE
	// inside one of them, which is what made the introduced claim unprovable.
	if !reflect.DeepEqual(after.State.GenesisPaths, []string{"internal/candidate/feature.go", "internal/legacy/unsafe.go"}) {
		t.Fatalf("genesis paths = %#v", after.State.GenesisPaths)
	}
}

func TestReviewFacadeStartCannotResetActiveCorrectionBudget(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	write := func(value string) {
		t.Helper()
		content := fmt.Sprintf("base\none\ntwo\nthree\n%s\n", value)
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("wrong")
	started := runNegotiatedReviewStart(t, repo, "review-correction-conflict")
	reviewer := filepath.Join(t.TempDir(), "reviewer.json")
	writeReviewCLIJSON(t, reviewer, facadeReviewerResult{Findings: []facadeFinding{{
		Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "wrong value", ProofRefs: []string{"candidate-only failure"},
		EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, Evidence: []string{"focused differential failure"}})
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--lineage", started.LineageID, "--result", reviewer}, io.Discard); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	beforeState, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if before.State.State != reviewtransaction.StateCorrectionRequired || before.State.InitialAtomicStart == nil ||
		len(before.State.CorrectionAttempts) != 0 || before.State.CumulativeCorrectionLines != 0 {
		t.Fatalf("correction fixture = %#v", before.State)
	}
	write("second-byte-only-edit")

	var conflictOutput bytes.Buffer
	err = RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--lineage", started.LineageID,
	}), &conflictOutput)
	if err == nil {
		t.Fatalf("exact correction-lineage START reset the active budget:\n%s", conflictOutput.String())
	}
	failure := decodeReviewIntegrationFailure(t, conflictOutput.Bytes())
	failureSchema := compileWholePublishedReviewSchema(t, "v2", "failure.schema.json")
	validatePublishedReviewSchema(t, failureSchema, conflictOutput.Bytes())
	if failure.Code != "atomic_start_conflict" || failure.Phase != "pre_native" ||
		failure.MutationOutcome != ReviewMutationNotStarted || failure.AuthorityApplicability != "current_target" ||
		!failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityNotReplayable ||
		failure.NextAction != "correct_request" || failure.LineageID != started.LineageID {
		t.Fatalf("exact correction-lineage START conflict = %#v", failure)
	}
	afterConflict, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	afterConflictState, err := os.ReadFile(store.StatePath())
	if err != nil || !bytes.Equal(beforeState, afterConflictState) || !reflect.DeepEqual(before, afterConflict) ||
		afterConflict.State.CorrectionBudget != before.State.CorrectionBudget ||
		afterConflict.State.CumulativeCorrectionLines != 0 || len(afterConflict.State.CorrectionAttempts) != 0 ||
		!reflect.DeepEqual(afterConflict.State.InitialAtomicStart, before.State.InitialAtomicStart) {
		t.Fatalf("atomic conflict mutated correction authority: before=%#v after=%#v err=%v", before, afterConflict, err)
	}

	created := atomicStartV2(t, repo, "")
	if created.Action != "created" || created.LineageID == started.LineageID ||
		created.State != reviewtransaction.StateReviewing {
		t.Fatalf("unqualified START did not create an independent lineage: %#v", created)
	}
	createdStore, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, created.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	createdRecord, err := createdStore.Load()
	if err != nil || createdRecord.State.InitialAtomicStart == nil ||
		createdRecord.State.InitialAtomicStart.WorktreeIdentity != before.State.InitialAtomicStart.WorktreeIdentity ||
		!reflect.DeepEqual(createdRecord.State.InitialAtomicStart.Selector, before.State.InitialAtomicStart.Selector) {
		t.Fatalf("unqualified compact authority = %#v, %v", createdRecord, err)
	}
	stores, discoverErr := reviewtransaction.DiscoverCompactStores(t.Context(), repo)
	if discoverErr != nil || len(stores) != 2 {
		t.Fatalf("independent compact lineages = %d, err=%v", len(stores), discoverErr)
	}
}

// TestReviewFacadeFinalizeIgnoresCorrectionCreatedUntrackedPath replaces the
// test that pinned the exact opposite. Before #2394 an untracked file created
// while correcting was rejected outright, because the sweep would have made it
// review scope and the correction would have smuggled unreviewed bytes into
// the candidate. Scope is declared now, so an undeclared file cannot enter the
// corrected candidate -- and therefore must not block the correction either,
// which is the false blocker that guard would have become.
func TestReviewFacadeFinalizeIgnoresCorrectionCreatedUntrackedPath(t *testing.T) {
	// Not parallel: opting in writes the user's global mode through t.Setenv,
	// which Go forbids in a test that also calls t.Parallel.
	reviewEnabledHome(t)

	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	resultPath := filepath.Join(t.TempDir(), "review.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
		Findings: []facadeFinding{{
			Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "candidate returns the wrong terminal value",
			ProofRefs:     []string{"differential test passes on base and fails on candidate"},
			EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
		}},
		Evidence: []string{"focused differential test failed on candidate"},
	})
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--result", resultPath, "--correction-lines", "2"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Undeclared scratch output of the correction work, left on disk for the
	// whole acceptance.
	if err := os.WriteFile(filepath.Join(repo, "correction-evidence.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, storeErr := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	record, loadErr := store.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	fix, fixErr := facadeVerificationEvidenceTarget(context.Background(), repo, record.State, record.Revision)
	if fixErr != nil {
		t.Fatal(fixErr)
	}
	for _, path := range fix.Paths {
		if path == "correction-evidence.json" {
			t.Fatalf("corrected candidate admitted the undeclared path: %v", fix.Paths)
		}
	}

	request := capturePassedCorrectionEvidenceForTest(t, repo, started.LineageID)
	validationPath := filepath.Join(t.TempDir(), "validation.json")
	writeReviewCLIJSON(t, validationPath, facadeValidationResult{
		TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: request.CorrectionTargetIdentity,
		OriginalCriteria:     facadeValidationCheck{Passed: true, Evidence: []string{"original acceptance test passed for tracked.txt"}},
		CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"targeted regression passed for tracked.txt"}},
		FollowUps:            []reviewtransaction.FollowUp{},
	})
	var output bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--validation", validationPath, "--captured-evidence"}, &output); err != nil {
		t.Fatalf("undeclared correction scratch file blocked the correction: %v", err)
	}
	finalized := decodeFacadeFinalize(t, output.Bytes())
	if finalized.State != reviewtransaction.StateApproved || finalized.Action != reviewApprovedCompactBurnedFinalizeAction || finalized.ReceiptPath != "" {
		t.Fatalf("undeclared-path correction finalize = %#v", finalized)
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
	if _, statErr := os.Stat(filepath.Join(repo, "correction-evidence.json")); statErr != nil {
		t.Fatalf("fixture scratch file is gone, so the assertions proved nothing: %v", statErr)
	}
}
func TestReviewFacadePersistsOverBudgetForecastAndActual(t *testing.T) {
	// Not parallel: opting in writes the user's global mode through t.Setenv,
	// which Go forbids in a test that also calls t.Parallel.
	reviewEnabledHome(t)

	newCandidate := func(t *testing.T) (string, ReviewFacadeStartResult, string) {
		t.Helper()
		repo := initReviewCLIRepo(t)
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		started := startFacadeReview(t, repo)
		resultPath := filepath.Join(t.TempDir(), "review.json")
		writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
			Findings: []facadeFinding{{
				Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "candidate regression",
				ProofRefs:     []string{"differential test fails only on candidate"},
				EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
			}}, Evidence: []string{"focused differential test failed"},
		})
		return repo, started, resultPath
	}
	t.Run("forecast", func(t *testing.T) {
		repo, started, resultPath := newCandidate(t)
		if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--result", resultPath, "--correction-lines", "3"}, io.Discard); err != nil {
			t.Fatal(err)
		}
		store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
		record, err := store.Load()
		if err != nil || record.State.State != reviewtransaction.StateEscalated || record.State.ProposedCorrectionLines == nil || *record.State.ProposedCorrectionLines != 3 || record.State.ActualCorrectionLines != nil {
			t.Fatalf("over-budget forecast state = %#v, %v", record.State, err)
		}
	})
	t.Run("actual", func(t *testing.T) {
		repo, started, resultPath := newCandidate(t)
		if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--result", resultPath, "--correction-lines", "2"}, io.Discard); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\nfixed-one\nfixed-two\nthree\nfour\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		request := capturePassedCorrectionEvidenceForTest(t, repo, started.LineageID)
		validationPath := filepath.Join(t.TempDir(), "validation.json")
		writeReviewCLIJSON(t, validationPath, facadeValidationResult{
			TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: request.CorrectionTargetIdentity,
			OriginalCriteria:     facadeValidationCheck{Passed: true, Evidence: []string{"acceptance passes"}},
			CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"regression passes"}},
			FollowUps:            []reviewtransaction.FollowUp{},
		})
		store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
		before, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		if before.State.State != reviewtransaction.StateCorrectionRequired || before.State.ProposedCorrectionLines == nil || *before.State.ProposedCorrectionLines != 2 ||
			before.State.CumulativeCorrectionLines != 0 || len(before.State.CorrectionAttempts) != 0 || before.State.ActualCorrectionLines != nil {
			t.Fatalf("over-budget forecast authority = %#v", before.State)
		}
		if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--validation", validationPath, "--captured-evidence"}, io.Discard); err == nil || !strings.Contains(err.Error(), "budget") {
			t.Fatalf("over-budget actual error = %v", err)
		}
		record, loadErr := store.Load()
		if loadErr != nil || record.Revision != before.Revision || record.State.State != reviewtransaction.StateCorrectionRequired ||
			record.State.ProposedCorrectionLines == nil || *record.State.ProposedCorrectionLines != 2 ||
			record.State.CumulativeCorrectionLines != 0 || len(record.State.CorrectionAttempts) != 0 || record.State.ActualCorrectionLines != nil {
			t.Fatalf("over-budget actual authority = %#v, %v", record.State, loadErr)
		}
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		request = capturePassedCorrectionEvidenceForTest(t, repo, started.LineageID)
		writeReviewCLIJSON(t, validationPath, facadeValidationResult{
			TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: request.CorrectionTargetIdentity,
			OriginalCriteria:     facadeValidationCheck{Passed: true, Evidence: []string{"adjusted acceptance passes"}},
			CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"adjusted regression passes"}},
			FollowUps:            []reviewtransaction.FollowUp{},
		})
		prepared, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		if prepared.State.State != reviewtransaction.StateCorrectionRequired || prepared.State.ProposedCorrectionLines == nil || *prepared.State.ProposedCorrectionLines != 2 ||
			prepared.State.CumulativeCorrectionLines != 0 || len(prepared.State.CorrectionAttempts) != 0 || prepared.State.ActualCorrectionLines != nil {
			t.Fatalf("adjusted correction pre-approval accounting = %#v", prepared.State)
		}
		var finalizedOutput bytes.Buffer
		if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--validation", validationPath, "--captured-evidence"}, &finalizedOutput); err != nil {
			t.Fatal(err)
		}
		assertApprovedBurnedCompactFacadeFinalize(t, finalizedOutput.Bytes())
		assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
	})
}

func TestReviewFacadeCompactRefuterAndHostileGitSelection(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeReviewStartCandidate(t, repo, "new.txt", "untracked\n", 0o644)
	hostile := initReviewCLIRepo(t)
	for name, value := range map[string]string{
		"GIT_DIR": filepath.Join(hostile, ".git"), "GIT_WORK_TREE": hostile,
		"GIT_COMMON_DIR": filepath.Join(hostile, ".git"), "GIT_INDEX_FILE": filepath.Join(hostile, ".git", "index"),
	} {
		t.Setenv(name, value)
	}
	started := startFacadeReview(t, repo)
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	// #2394: new.txt is in scope because it was declared with `git add`, so it
	// reaches the candidate through the index rather than as intended-untracked.
	if !reflect.DeepEqual(record.State.InitialSnapshot.Paths, []string{"new.txt", "tracked.txt"}) || len(record.State.InitialSnapshot.IntendedUntracked) != 0 {
		t.Fatalf("hostile environment selected wrong compact target: %#v", record.State.InitialSnapshot)
	}
}

func TestReviewStatusReportsActiveAuthorityWithoutChangingAuthorityFiles(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate behavior\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", repo}, &output); err != nil {
		t.Fatal(err)
	}
	var report struct {
		Schema   string `json:"schema"`
		Complete bool   `json:"complete"`
		Entries  []struct {
			LineageID string `json:"lineage_id"`
			Status    string `json:"status"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Schema != reviewtransaction.ReviewAuthorityStatusSchema || !report.Complete || len(report.Entries) != 1 ||
		report.Entries[0].LineageID != started.LineageID || report.Entries[0].Status != "active" {
		t.Fatalf("status report = %s", output.String())
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("review status mutated compact authority")
	}
}

func TestReviewFacadeHelpAndFlatCompatibilityPathsRemainAvailable(t *testing.T) {
	for _, subcommand := range []string{"start", "finalize", "validate", "status", "recover"} {
		var output bytes.Buffer
		if err := RunReview([]string{subcommand, "--help"}, &output); err != nil || !strings.Contains(output.String(), "Usage: gentle-ai review "+subcommand) {
			t.Fatalf("facade %s help: %v\n%s", subcommand, err, output.String())
		}
	}
	var validateHelp bytes.Buffer
	if err := RunReview([]string{"validate", "--help"}, &validateHelp); err != nil || !strings.Contains(validateHelp.String(), "--lineage") {
		t.Fatalf("review validate help must expose --lineage: %v\n%s", err, validateHelp.String())
	}
	for _, test := range []struct {
		run  func([]string, io.Writer) error
		want string
	}{
		{RunReviewStart, "Usage: gentle-ai review-start"}, {RunReviewStep, "Usage: gentle-ai review-step"},
		{RunReviewResume, "Usage: gentle-ai review-resume"},
		{RunReviewBundleExport, "Usage: gentle-ai review-bundle-export"}, {RunReviewBundleImport, "Usage: gentle-ai review-bundle-import"},
	} {
		var output bytes.Buffer
		if err := test.run([]string{"--help"}, &output); err != nil || !strings.Contains(output.String(), test.want) {
			t.Fatalf("flat compatibility help %q: %v\n%s", test.want, err, output.String())
		}
	}
}

// reviewFacadeUsageVerbs reads only the compact facade's usage declaration;
// later help prose may explain a verb but cannot advertise it as a facade verb.
func reviewFacadeUsageVerbs(t *testing.T, help string) map[string]bool {
	t.Helper()
	line, _, _ := strings.Cut(help, "\n")
	const prefix = "Usage: gentle-ai review <"
	const suffix = "> [flags]"
	if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, suffix) {
		t.Fatalf("review facade usage declaration = %q", line)
	}

	verbs := map[string]bool{}
	for _, verb := range strings.Split(strings.TrimSuffix(strings.TrimPrefix(line, prefix), suffix), "|") {
		if verbs[verb] {
			t.Fatalf("review facade usage declaration names %q more than once", verb)
		}
		verbs[verb] = true
	}
	return verbs
}

func reviewFacadeUsageConformanceError(advertised, dispatched map[string]bool) error {
	missing := []string{}
	for verb := range dispatched {
		if !advertised[verb] {
			missing = append(missing, verb)
		}
	}
	unexpected := []string{}
	for verb := range advertised {
		if !dispatched[verb] {
			unexpected = append(unexpected, verb)
		}
	}
	if len(missing) == 0 && len(unexpected) == 0 {
		return nil
	}
	sort.Strings(missing)
	sort.Strings(unexpected)
	return fmt.Errorf("facade usage drift: missing dispatched verbs %v; advertised without facade dispatch %v", missing, unexpected)
}

func reviewFacadeUsageHelp(t *testing.T) string {
	t.Helper()
	var help bytes.Buffer
	if err := RunReview([]string{"--help"}, &help); err != nil {
		t.Fatal(err)
	}
	return help.String()
}

func copyReviewFacadeVerbSet(verbs map[string]bool) map[string]bool {
	clone := make(map[string]bool, len(verbs))
	for verb := range verbs {
		clone[verb] = true
	}
	return clone
}

func TestReviewFacadeUsageMatchesEveryFacadeDispatch(t *testing.T) {
	advertised := reviewFacadeUsageVerbs(t, reviewFacadeUsageHelp(t))
	dispatched := reviewCommandDispatchVerbs(t)
	if err := reviewFacadeUsageConformanceError(advertised, dispatched); err != nil {
		t.Fatal(err)
	}
}

func TestReviewFacadeHelpHasNoPartialCommandList(t *testing.T) {
	if strings.Contains(reviewFacadeUsageHelp(t), "Additive headless capabilities") {
		t.Fatal("review facade help contains a second partial command list")
	}
}

func TestEveryAdvertisedReviewFacadeVerbDispatches(t *testing.T) {
	advertised := reviewFacadeUsageVerbs(t, reviewFacadeUsageHelp(t))
	verbs := make([]string, 0, len(advertised))
	for verb := range advertised {
		verbs = append(verbs, verb)
	}
	sort.Strings(verbs)

	for _, verb := range verbs {
		t.Run(verb, func(t *testing.T) {
			var output bytes.Buffer
			err := RunReview([]string{verb, "--help"}, &output)
			evidence := output.String()
			if err != nil {
				evidence += "\n" + err.Error()
			}
			lowered := strings.ToLower(evidence)
			if strings.Contains(lowered, "unknown") || strings.Contains(lowered, "unrecognized") {
				t.Fatalf("review %s dispatch evidence is unrecognized:\n%s", verb, evidence)
			}
			usage := "Usage: gentle-ai review " + verb
			requires := "review " + verb + " requires"
			if !strings.Contains(evidence, usage) && !strings.Contains(evidence, requires) {
				t.Fatalf("review %s did not produce verb-specific dispatch evidence; want %q or %q:\n%s", verb, usage, requires, evidence)
			}
		})
	}
}

func TestReviewFacadeUsageBoundaryExcludesAppMode(t *testing.T) {
	facade := reviewCommandDispatchVerbs(t)
	if facade[reviewModeDispatchVerb] {
		t.Fatalf("review %s belongs to the app pre-dispatch, not the compact facade switch", reviewModeDispatchVerb)
	}
	if !reviewDispatchableReviewVerbs(t)[reviewModeDispatchVerb] {
		t.Fatalf("review %s is missing from the app dispatch boundary", reviewModeDispatchVerb)
	}
	if reviewFacadeUsageVerbs(t, reviewFacadeUsageHelp(t))[reviewModeDispatchVerb] {
		t.Fatalf("review %s must not be advertised by the compact facade usage declaration", reviewModeDispatchVerb)
	}
}

func TestReviewFacadeUsageRatchetRejectsDrift(t *testing.T) {
	advertised := reviewFacadeUsageVerbs(t, reviewFacadeUsageHelp(t))
	dispatched := reviewCommandDispatchVerbs(t)
	deletedAdvertised := copyReviewFacadeVerbSet(advertised)
	delete(deletedAdvertised, "capture-result")
	inventedDispatched := copyReviewFacadeVerbSet(dispatched)
	inventedDispatched["invented"] = true
	for _, test := range []struct {
		name       string
		advertised map[string]bool
		dispatched map[string]bool
	}{
		{
			name:       "deleted advertised verb",
			advertised: deletedAdvertised,
			dispatched: dispatched,
		},
		{
			name:       "invented dispatch verb",
			advertised: advertised,
			dispatched: inventedDispatched,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := reviewFacadeUsageConformanceError(test.advertised, test.dispatched); err == nil {
				t.Fatal("facade usage drift unexpectedly passed")
			}
		})
	}
}

func TestReviewSchemaExamplesMatchStrictFacadeContracts(t *testing.T) {
	for _, kind := range []string{"reviewer", "refuter", "validator"} {
		t.Run(kind, func(t *testing.T) {
			var output bytes.Buffer
			if err := RunReview([]string{"schema", kind}, &output); err != nil {
				t.Fatal(err)
			}
			var document struct {
				Schema   string            `json:"$schema"`
				ID       string            `json:"$id"`
				Examples []json.RawMessage `json:"examples"`
			}
			if err := json.Unmarshal(output.Bytes(), &document); err != nil || document.Schema == "" || document.ID == "" || len(document.Examples) != 1 {
				t.Fatalf("schema document = %#v, %v", document, err)
			}
			path := filepath.Join(t.TempDir(), kind+".json")
			if err := os.WriteFile(path, document.Examples[0], 0o600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "reviewer":
				if _, err := readFacadeReviewerResults([]string{path}); err != nil {
					t.Fatal(err)
				}
			case "refuter":
				var value facadeRefuterResult
				if err := readFacadeJSON(path, &value); err != nil {
					t.Fatal(err)
				}
			case "validator":
				var value facadeValidationResult
				if err := readFacadeJSON(path, &value); err != nil {
					t.Fatal(err)
				}
				if _, err := value.compact(reviewtransaction.EmptyFixDeltaHash, []string{}, reviewtransaction.TargetedValidationRequest{
					RequestHash: value.TargetedValidationRequestHash, CorrectionTargetIdentity: value.CorrectionTargetIdentity,
				}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestReviewerSchemaRequiresRuntimeMandatoryFindingEvidence(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			MinItems int      `json:"minItems"`
			Enum     []string `json:"enum"`
			Items    struct {
				Not struct {
					Pattern string `json:"pattern"`
				} `json:"not"`
				Required []string `json:"required"`
				AllOf    []struct {
					Then struct {
						Required []string `json:"required"`
					} `json:"then"`
				} `json:"allOf"`
				Properties map[string]struct {
					MinItems int `json:"minItems"`
					Items    struct {
						Not struct {
							Pattern string `json:"pattern"`
						} `json:"not"`
					} `json:"items"`
				} `json:"properties"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(reviewInputSchemas["reviewer"], &schema); err != nil {
		t.Fatal(err)
	}
	wantRequired := []string{"location", "severity", "claim", "proof_refs"}
	wantSevere := []string{"evidence_class", "causal_disposition"}
	wantLenses := []string{
		"risk", "resilience", "readability", "reliability",
		reviewtransaction.LensRisk, reviewtransaction.LensResilience,
		reviewtransaction.LensReadability, reviewtransaction.LensReliability,
	}
	if !reflect.DeepEqual(schema.Properties["findings"].Items.Required, wantRequired) || len(schema.Properties["findings"].Items.AllOf) != 1 || !reflect.DeepEqual(schema.Properties["findings"].Items.AllOf[0].Then.Required, wantSevere) || schema.Properties["evidence"].MinItems != 1 || schema.Properties["findings"].Items.Properties["proof_refs"].MinItems != 1 {
		t.Fatalf("reviewer schema requirements = %#v", schema)
	}
	if !reflect.DeepEqual(schema.Properties["lens"].Enum, wantLenses) {
		t.Fatalf("reviewer schema lens enum = %v, want %v", schema.Properties["lens"].Enum, wantLenses)
	}
	proofSentinels := schema.Properties["findings"].Items.Properties["proof_refs"].Items.Not.Pattern
	evidenceSentinels := schema.Properties["evidence"].Items.Not.Pattern
	if proofSentinels == "" || proofSentinels != evidenceSentinels {
		t.Fatalf("reviewer schema sentinel parity = proof %q evidence %q", proofSentinels, evidenceSentinels)
	}

	for name, payload := range map[string]string{
		"missing location":              `{"findings":[{"severity":"CRITICAL","claim":"x","proof_refs":["proof"],"evidence_class":"deterministic","causal_disposition":"introduced"}],"evidence":["reviewed"]}`,
		"empty evidence":                `{"findings":[],"evidence":[]}`,
		"empty proof refs":              `{"findings":[{"location":"x.go:1","severity":"CRITICAL","claim":"x","proof_refs":[],"evidence_class":"deterministic","causal_disposition":"introduced"}],"evidence":["reviewed"]}`,
		"missing severe classification": `{"findings":[{"location":"x.go:1","severity":"CRITICAL","claim":"x","proof_refs":["proof"]}],"evidence":["reviewed"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "reviewer.json")
			if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
				t.Fatal(err)
			}
			results, err := readFacadeReviewerResults([]string{path})
			if err != nil {
				return
			}
			state := reviewtransaction.CompactState{SelectedLenses: []string{reviewtransaction.LensReliability}}
			input, err := prepareCompactReviewerResults(state, results, facadeRefuterResult{})
			if err == nil {
				err = state.CompleteReview(input)
			}
			if err == nil {
				t.Fatal("runtime accepted schema-invalid reviewer input")
			}
		})
	}
}

func TestReviewSchemasRequireConcreteEvidenceStrings(t *testing.T) {
	for _, kind := range []string{"reviewer", "refuter", "validator"} {
		var compact bytes.Buffer
		if err := json.Compact(&compact, reviewInputSchemas[kind]); err != nil {
			t.Fatalf("compact %s schema: %v", kind, err)
		}
		if !bytes.Contains(compact.Bytes(), []byte(`"pattern":"\\S"`)) {
			t.Fatalf("%s schema lacks concrete-evidence pattern", kind)
		}
	}
}

func TestReviewFacadeRejectsMalformedInputsWithoutConsumingTerminalValidator(t *testing.T) {
	// Not parallel: opting in writes the user's global mode through t.Setenv,
	// which Go forbids in a test that also calls t.Parallel.
	reviewEnabledHome(t)

	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n01\n02\n03\n04\n05\n06\n07\n08\n09\n10\n11\n12\n13\n14\n15\n16\n17\n18\n19\n20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	assertUnchanged := func(before string, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("malformed input was accepted")
		}
		after, loadErr := store.Load()
		if loadErr != nil || after.Revision != before {
			t.Fatalf("malformed input changed authority: %v, %#v", loadErr, after)
		}
	}
	malformed := filepath.Join(t.TempDir(), "malformed.json")
	if err := os.WriteFile(malformed, []byte(`{"findings":[],"evidence":[],"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	record, _ := store.Load()
	assertUnchanged(record.Revision, finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--result", malformed}, io.Discard))

	reviewer := filepath.Join(t.TempDir(), "reviewer.json")
	writeReviewCLIJSON(t, reviewer, facadeReviewerResult{Findings: []facadeFinding{{Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "wrong value", ProofRefs: []string{"candidate-only failure"}, EvidenceClass: reviewtransaction.EvidenceInferential, CausalDisposition: reviewtransaction.CausalIntroduced}}, Evidence: []string{"reviewed once"}})
	if err := os.WriteFile(malformed, []byte(`{"results":[],"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	assertUnchanged(record.Revision, finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--result", reviewer, "--refuter", malformed}, io.Discard))
	refuter := filepath.Join(t.TempDir(), "refuter.json")
	writeReviewCLIJSON(t, refuter, facadeRefuterResult{Results: []facadeRefuterOutcome{{FindingID: "R3-001", Outcome: reviewtransaction.OutcomeCorroborated, ProofRefs: []string{"independent reproduction"}}}})
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--result", reviewer, "--refuter", refuter, "--correction-lines", "6"}, io.Discard); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n01\n02\n03\nfirst-fix\n05\n06\n07\n08\n09\n10\n11\n12\n13\n14\n15\n16\n17\n18\n19\n20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(malformed, []byte(`{"original_criteria":{"passed":false,"evidence":["failed"]},"correction_regression":{"passed":false,"evidence":["failed"]},"follow_ups":[],"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	record, _ = store.Load()
	assertUnchanged(record.Revision, RunReviewFacadeFinalize([]string{"--cwd", repo, "--validation", malformed}, io.Discard))
}

func TestReviewRecoverCreatesSuccessorAndDiscoveryRejectsHistoricalAuthority(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	resultPath := filepath.Join(t.TempDir(), "review.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{Findings: []facadeFinding{{Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "candidate regression", ProofRefs: []string{"candidate-only failure"}, EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced}}, Evidence: []string{"reviewed"}})
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--lineage", started.LineageID, "--result", resultPath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	predecessorStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	predecessor, err := predecessorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("changed scope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeReviewStartCandidate(t, repo, "expanded.txt", "expanded scope\n", 0o644)
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	intended, err := builder.DiscoverUnignoredUntracked(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	target, err := builder.Build(context.Background(), reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: intended})
	if err != nil {
		t.Fatal(err)
	}
	authorization := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + started.LineageID +
		"\npredecessor_revision=" + predecessor.Revision + "\ntarget_identity=" + target.Identity + "\nactor=maintainer\nreason=candidate changed"
	var output bytes.Buffer
	if err := RunReview([]string{"recover", "--cwd", repo, "--predecessor-lineage", started.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "review-recovered",
		"--disposition", "scope_changed", "--reason", "candidate changed", "--actor", "maintainer", "--maintainer-authorization", authorization}, &output); err != nil {
		t.Fatal(err)
	}
	var recovered ReviewRecoverResult
	if err := json.Unmarshal(output.Bytes(), &recovered); err != nil {
		t.Fatal(err)
	}
	if recovered.LineageID != "review-recovered" || recovered.Recovery.PredecessorRevision != predecessor.Revision {
		t.Fatalf("recovered = %#v", recovered)
	}
	before, err := os.ReadFile(predecessorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	err = RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--correction-lines", "1"}, io.Discard)
	after, loadErr := predecessorStore.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	afterBytes, readErr := os.ReadFile(predecessorStore.StatePath())
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err == nil || !strings.Contains(err.Error(), "superseded") || after.Revision != predecessor.Revision || !bytes.Equal(before, afterBytes) {
		t.Fatalf("superseded finalize = %v, before %s, after %s", err, predecessor.Revision, after.Revision)
	}
}

func TestReviewRecoverRetainsCommittedOnlyBaseDiffAndIgnoresWorkspace(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	baseRef := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "--", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-m", "candidate")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("ignored workspace\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Recovery is the operation under test. Its approved committed-only
	// predecessor is a historical compatibility premise, so construct that
	// authority and receipt directly rather than through current FINALIZE.
	fixture := finalizeHistoricalFacadeReviewForRepo(t, repo, "historical-committed-base-diff-recovery", "--base-ref", baseRef, "--committed-only")
	predecessor := fixture.Record
	lineage := predecessor.State.LineageID
	var output bytes.Buffer
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("recovered staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "--", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-m", "recovered candidate")
	committedTree := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^{tree}"))
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("unstaged workspace\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := RunReview([]string{"recover", "--cwd", repo, "--predecessor-lineage", lineage,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "review-staged-recovered",
		"--disposition", "scope_changed", "--reason", "staged scope changed", "--actor", "maintainer", "--base-ref", baseRef, "--committed-only"}, &output); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "review-staged-recovered")
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State.InitialSnapshot.Kind != reviewtransaction.TargetBaseDiff || recovered.State.InitialSnapshot.BaseTree != predecessor.State.InitialSnapshot.BaseTree || recovered.State.InitialSnapshot.CandidateTree != committedTree {
		t.Fatalf("recovered committed authority = %#v, want base %s and candidate %s", recovered.State, predecessor.State.InitialSnapshot.BaseTree, committedTree)
	}
}

func TestReviewBindSDDRequiresExplicitInputs(t *testing.T) {
	err := RunReview([]string{"bind-sdd", "--cwd", t.TempDir(), "--change", "thin", "--lineage", "approved"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "expected-binding-revision") {
		t.Fatalf("bind-sdd missing explicit CAS input error = %v", err)
	}
	err = RunReview([]string{"bind-sdd", "--cwd", t.TempDir(), "--change", "thin", "--lineage", "approved", "--expected-binding-revision", "sha256:deadbeef"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "empty or sha256") {
		t.Fatalf("bind-sdd malformed CAS input error = %v", err)
	}
}

func TestReviewBindSDDAcceptsEqualsFormForEmptyExpectedRevision(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	for path, content := range map[string]string{"tasks.md": "- [x] 1.1 Done\n", "proposal.md": "# Proposal\n", "design.md": "# Design\n", "specs/binding/spec.md": "# Spec\n"} {
		writeReviewStartCandidate(t, repo, "openspec/changes/thin/"+path, content, 0o644)
	}
	fixture := finalizeHistoricalFacadeReviewForRepo(t, repo, "historical-bind-sdd-equals")
	if err := RunReview([]string{"bind-sdd", "--cwd", repo, "--change", "thin", "--lineage", fixture.Record.State.LineageID, "--expected-binding-revision="}, io.Discard); err != nil {
		t.Fatalf("equals-form expected revision was rejected: %v", err)
	}
}

func TestReviewBindSDDFeedsSelectedSDDStatusRuntime(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	for path, content := range map[string]string{
		"tasks.md":              "- [x] 1.1 Done\n",
		"proposal.md":           "# Proposal\n",
		"design.md":             "# Design\n",
		"specs/binding/spec.md": "# Spec\n",
	} {
		writeReviewStartCandidate(t, repo, "openspec/changes/thin/"+path, content, 0o644)
	}
	fixture := finalizeHistoricalFacadeReviewForRepo(t, repo, "historical-bind-sdd-status")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "exact approved SDD candidate")
	var output bytes.Buffer
	if err := RunReview([]string{"bind-sdd", "--cwd", repo, "--change", "thin", "--lineage", fixture.Record.State.LineageID, "--expected-binding-revision", ""}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "gentle-ai.sdd-review-binding/v1") {
		t.Fatalf("binding output = %s", output.String())
	}
	output.Reset()
	if err := RunSDDStatus([]string{"thin", "--cwd", repo, "--json"}, &output); err != nil {
		t.Fatal(err)
	}
	var status sddstatus.Status
	if err := json.Unmarshal(output.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.NextRecommended != "verify" || status.Dependencies.Verify != sddstatus.DependencyReady || status.Dependencies.Archive != sddstatus.DependencyBlocked || status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateAllow {
		t.Fatalf("bound runtime status = %#v", status)
	}
}

func TestHistoricalLineageRequiresExplicitCompatibilityCommand(t *testing.T) {
	fixture := newLegacyCLIFixture(t, "legacy-read-only")
	before := readLegacyAuthorityTree(t, fixture.store.Dir)
	var resumed bytes.Buffer
	if err := RunReviewResume([]string{"--cwd", fixture.repo, "--lineage", fixture.lineage}, &resumed); err != nil {
		t.Fatalf("explicit historical compatibility resume: %v", err)
	}
	input := filepath.Join(t.TempDir(), "input.json")
	if err := os.WriteFile(input, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := RunReviewStep([]string{"--cwd", fixture.repo, "--lineage", fixture.lineage, "--operation", "freeze-findings", "--input", input}, io.Discard)
	if !errors.Is(err, reviewtransaction.ErrLegacyReadOnly) {
		t.Fatalf("legacy append error = %v", err)
	}

	var absent *reviewCompactFacadeLineageAbsentError
	err = RunReviewFacadeFinalize([]string{"--cwd", fixture.repo, "--lineage", fixture.lineage}, io.Discard)
	if !errors.As(err, &absent) || absent.LineageID != fixture.lineage ||
		!strings.Contains(err.Error(), "gentle-ai review start") || !strings.Contains(err.Error(), "gentle-ai review-resume") {
		t.Fatalf("ordinary FINALIZE historical-only refusal = %v, want typed fresh-compact/manual-compatibility route", err)
	}

	err = RunReviewCaptureResult([]string{
		"--cwd", fixture.repo, "--lineage", fixture.lineage,
		"--target", "sha256:" + strings.Repeat("0", 64), "--lens", reviewtransaction.LensReliability,
		"--order", "0", "--preflight",
	}, io.Discard)
	if !errors.As(err, &absent) || absent.LineageID != fixture.lineage {
		t.Fatalf("ordinary capture historical-only refusal = %v, want typed compact absence", err)
	}
	if after := readLegacyAuthorityTree(t, fixture.store.Dir); !reflect.DeepEqual(before, after) {
		t.Fatal("ordinary capture or FINALIZE mutated historical authority bytes")
	}
}

func TestCompactTransportCommandsRoundTripWithoutEventReconstruction(t *testing.T) {
	// Not parallel: opting in writes the user's global mode through t.Setenv,
	// which Go forbids in a test that also calls t.Parallel.
	reviewEnabledHome(t)

	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture := finalizeHistoricalFacadeReviewForRepo(t, repo, "historical-compact-transport-round-trip")
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "candidate")
	bundlePath := filepath.Join(t.TempDir(), "compact-transport.json")
	if err := RunReviewBundleExport([]string{"--cwd", repo, "--lineage", fixture.Record.State.LineageID, "--out", bundlePath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), `"events"`) || !strings.Contains(string(payload), reviewtransaction.CompactTransportSchema) {
		t.Fatalf("compact transport reintroduced event history: %s", payload)
	}
	clone := filepath.Join(t.TempDir(), "clone")
	runReviewCLIGit(t, repo, "clone", "--no-local", repo, clone)
	if err := RunReviewBundleImport([]string{"--cwd", clone, "--bundle", bundlePath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	cloneStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), clone, fixture.Record.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cloneStore.Load(); err != nil {
		t.Fatal(err)
	}
	assertCompactLineageFiles(t, cloneStore, []string{"review-receipt.json", "review-state.json"})
}

func TestCompactTransportAllowsCorrectedPrePushWithoutTransientBaseObject(t *testing.T) {
	// Not parallel: opting in writes the user's global mode through t.Setenv,
	// which Go forbids in a test that also calls t.Parallel.
	reviewEnabledHome(t)

	source := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, source)
	sourceStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), source, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := sourceStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(t.TempDir(), "review.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
		Findings: []facadeFinding{{
			Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "candidate returns the wrong terminal value",
			ProofRefs:     []string{"differential test passes on base and fails on candidate"},
			EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
		}}, Evidence: []string{"focused differential test failed on candidate"},
	})
	if err := finalizeReviewCLIArgs(t, source, []string{"--cwd", source, "--result", resultPath, "--correction-lines", "2"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := capturePassedCorrectionEvidenceForTest(t, source, started.LineageID)
	finalizeHistoricalCorrectedFacadeReviewForLineage(t, source, started.LineageID, facadeValidationResult{
		TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: request.CorrectionTargetIdentity,
		OriginalCriteria:     facadeValidationCheck{Passed: true, Evidence: []string{"original acceptance test passed"}},
		CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"targeted regression test passed"}},
		FollowUps:            []reviewtransaction.FollowUp{},
	})
	runReviewCLIGit(t, source, "add", "tracked.txt")
	runReviewCLIGit(t, source, "commit", "-qm", "corrected candidate")
	sourceRecord, err := sourceStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	sourceReceiptPayload, err := os.ReadFile(sourceStore.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	bundlePath := filepath.Join(t.TempDir(), "corrected-transport.json")
	if err := RunReviewBundleExport([]string{"--cwd", source, "--lineage", started.LineageID, "--out", bundlePath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	clone := filepath.Join(t.TempDir(), "clone")
	runReviewCLIGit(t, source, "clone", "--no-local", source, clone)
	runReviewCLIGit(t, source, "branch", "reviewed-base", "HEAD^")
	if sourceRecord.State.CurrentSnapshot.BaseTree != initial.State.InitialSnapshot.BaseTree {
		t.Fatalf("corrected authority base = %s, want reviewed base %s", sourceRecord.State.CurrentSnapshot.BaseTree, initial.State.InitialSnapshot.BaseTree)
	}
	intermediate := initial.State.InitialSnapshot.CandidateTree
	if err := exec.Command("git", "-C", clone, "cat-file", "-e", intermediate+"^{tree}").Run(); err == nil {
		t.Fatalf("clean clone unexpectedly retained dangling intermediate tree %s", intermediate)
	}
	if err := RunReviewBundleImport([]string{"--cwd", clone, "--bundle", bundlePath}, io.Discard); err != nil {
		t.Fatalf("corrected compact import: %v", err)
	}
	cloneStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), clone, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	cloneRecord, err := cloneStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	cloneReceiptPayload, err := os.ReadFile(cloneStore.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cloneRecord, sourceRecord) || !bytes.Equal(cloneReceiptPayload, sourceReceiptPayload) {
		t.Fatal("corrected compact recovery changed state or receipt")
	}
}

// finalizeHistoricalCorrectedFacadeReviewForLineage persists the one corrected
// historical receipt required by the compact transport fixture without routing
// its approval through current FINALIZE.
func finalizeHistoricalCorrectedFacadeReviewForLineage(t *testing.T, repo, lineage string, validation facadeValidationResult) historicalCompatibilityCompactReceiptFixture {
	t.Helper()
	ctx := t.Context()
	store, err := reviewtransaction.CompactAuthoritativeStore(ctx, repo, lineage)
	if err != nil {
		t.Fatalf("open historical corrected review store: %v", err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatalf("load historical corrected review authority: %v", err)
	}
	if record.State.LineageID != lineage || record.State.State != reviewtransaction.StateCorrectionRequired || record.State.ProposedCorrectionLines == nil {
		t.Fatalf("historical corrected review precondition failed: %#v", record.State)
	}
	if _, err := os.Stat(store.ReceiptPath()); err == nil {
		t.Fatalf("historical corrected receipt already exists; refusing to overwrite %q", store.ReceiptPath())
	} else if !os.IsNotExist(err) {
		t.Fatalf("inspect historical corrected receipt: %v", err)
	}

	fix, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(ctx, reviewtransaction.Target{
		Kind: reviewtransaction.TargetFixDiff, Projection: record.State.InitialSnapshot.Projection,
		BaseRef: record.State.CurrentSnapshot.CandidateTree, IntendedUntracked: record.State.InitialSnapshot.IntendedUntracked,
		LedgerIDs: record.State.FixFindingIDs,
	})
	if err != nil {
		t.Fatalf("build historical corrected fix target: %v", err)
	}
	captured, err := reviewtransaction.ReadCapturedVerificationEvidence(store.Dir, lineage, record.Revision, fix)
	if err != nil {
		t.Fatalf("read historical corrected verification evidence: %v", err)
	}
	if captured.Record.Outcome != reviewtransaction.VerificationOutcomePassed || captured.Record.AuthorityRevision != record.Revision || captured.Record.TargetIdentity != fix.Identity {
		t.Fatalf("historical corrected verification evidence binding = %#v, want passed revision %q target %q", captured.Record, record.Revision, fix.Identity)
	}
	actual, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ChangedLines(ctx, fix)
	if err != nil {
		t.Fatalf("count historical corrected lines: %v", err)
	}
	request, err := reviewtransaction.BuildTargetedValidationRequestFromSnapshot(ctx, repo, record.State, record.Revision, fix)
	if err != nil {
		t.Fatalf("build historical corrected validation request: %v", err)
	}
	complete, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).BuildCorrectedCandidate(ctx, record.State.InitialSnapshot, fix)
	if err != nil {
		t.Fatalf("build historical corrected candidate: %v", err)
	}
	native, err := validation.compact(reviewtransaction.FixDeltaHashForSnapshot(fix), record.State.FixFindingIDs, request)
	if err != nil {
		t.Fatalf("validate historical corrected result: %v", err)
	}
	state := record.State
	if err := state.CompleteCorrectionVerification(fix, actual, native, captured.Record, captured.Payload, complete); err != nil {
		t.Fatalf("complete historical corrected review: %v", err)
	}
	if state.State != reviewtransaction.StateApproved || state.CurrentSnapshot.Identity != complete.Identity {
		t.Fatalf("historical corrected approval = %#v, want approved candidate %q", state, complete.Identity)
	}
	revision, err := store.Replace(record.Revision, "review/complete-correction-verification", state)
	if err != nil {
		t.Fatalf("persist historical corrected approval: %v", err)
	}
	receipt, err := state.Receipt()
	if err != nil {
		t.Fatalf("create historical corrected receipt: %v", err)
	}
	if err := reviewtransaction.WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatalf("persist historical corrected receipt: %v", err)
	}
	persistedReceiptBytes, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatalf("read historical corrected receipt: %v", err)
	}
	persistedReceipt, err := reviewtransaction.ParseCompactReceipt(persistedReceiptBytes)
	if err != nil || !reviewtransaction.CompactReceiptEqual(receipt, persistedReceipt) {
		t.Fatalf("historical corrected receipt binding = %#v, %v", persistedReceipt, err)
	}
	approved, err := store.Load()
	if err != nil || approved.Revision != revision || approved.State.State != reviewtransaction.StateApproved || approved.State.CurrentSnapshot.Identity != complete.Identity {
		t.Fatalf("historical corrected approved record = %#v, %v", approved, err)
	}
	return historicalCompatibilityCompactReceiptFixture{Record: approved, Store: store}
}

func TestReviewFacadeRoutesStructuredCandidateCausality(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\ncandidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	record, _ := store.Load()
	for _, tt := range []struct {
		causality reviewtransaction.CausalDisposition
		want      reviewtransaction.CausalDisposition
		state     reviewtransaction.State
	}{{reviewtransaction.CausalIntroduced, reviewtransaction.CausalUnknown, reviewtransaction.StateEscalated}, {reviewtransaction.CausalWorsened, reviewtransaction.CausalUnknown, reviewtransaction.StateEscalated}, {reviewtransaction.CausalBehaviorActivated, reviewtransaction.CausalBehaviorActivated, reviewtransaction.StateCorrectionRequired}} {
		result := facadeReviewerResult{Lens: started.SelectedLenses[0], Findings: []facadeFinding{{Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "unchanged", ProofRefs: []string{string(tt.causality) + " structured proof"}, EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: tt.causality}}, Evidence: []string{"reviewed"}}
		input, err := prepareCompactReviewerResults(record.State, []facadeReviewerResult{result}, facadeRefuterResult{}, facadeRepositoryEvidence{ctx: context.Background(), repo: repo})
		state := record.State
		if err == nil {
			err = state.CompleteReview(input)
		}
		finding := state.Findings[0]
		if got := state.Classifications[finding.ID].Causality; err != nil || got != tt.want || state.State != tt.state {
			t.Fatalf("causality %q = %q, state %q, error %v", tt.causality, got, state.State, err)
		}
	}
}

func TestReviewFacadePropagatesCausalGitFailureBeforeMutation(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\ncandidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	record, _ := store.Load()
	result := facadeReviewerResult{Lens: started.SelectedLenses[0], Findings: []facadeFinding{{Location: "tracked.txt:2", Severity: "CRITICAL", Claim: "candidate", ProofRefs: []string{"candidate proof"}, EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced}}, Evidence: []string{"reviewed"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := prepareCompactReviewerResults(record.State, []facadeReviewerResult{result}, facadeRefuterResult{}, facadeRepositoryEvidence{ctx: ctx, repo: repo})
	var timeout *reviewtransaction.GitCommandTimeoutError
	if !errors.As(err, &timeout) {
		t.Fatalf("causal Git error = %T %v, want *GitCommandTimeoutError", err, err)
	}
	after, _ := store.Load()
	if after.Revision != record.Revision || !reflect.DeepEqual(after.State, record.State) {
		t.Fatalf("causal preflight changed authority: before %#v after %#v", record, after)
	}
}

// startFacadeReview builds and starts legacy (compact-v2) authority over the
// live workspace candidate, with an auto-derived lineage id -- the single
// most widely shared fixture primitive in this package (44 call sites
// across 11 files as of Wave 7 S7/WU18).
//
// Before the atomic START cutover, this called the CLI's own `review start`,
// which could create legacy compact-v2 authority. The cutover removed that
// branch entirely -- `review start` is now unconditionally
// v3, so there is no CLI-reachable way left to create a NEW legacy
// authority. Every caller of this helper needs genuine compact-v2 authority
// (proven by pervasive reviewtransaction.CompactAuthoritativeStore/
// CompactStore-typed follow-up reads throughout this file and its 10
// siblings), so this constructs it directly through the same
// reviewtransaction API runReviewFacadeStart's now-deleted legacy branch
// used to call -- identical to finalizeApprovedFacadeReview's and
// approveDiscoveryMarkdownProjection's own fixes.
func startFacadeReview(t *testing.T, repo string) ReviewFacadeStartResult {
	t.Helper()
	ctx := context.Background()
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	root, err := builder.ResolveRepositoryRoot(ctx)
	if err != nil {
		t.Fatalf("resolve facade review repository root: %v", err)
	}
	rootBuilder := reviewtransaction.SnapshotBuilder{Repo: root}
	snapshot, err := rootBuilder.Build(ctx, reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatalf("build facade review target: %v", err)
	}
	assessment, err := rootBuilder.AssessSnapshotRisk(ctx, snapshot)
	if err != nil {
		t.Fatalf("classify facade review target: %v", err)
	}
	lenses, err := facadeSelectedLenses(assessment, "reliability")
	if err != nil {
		t.Fatalf("select facade review lenses: %v", err)
	}
	request, err := prepareReviewFacadeCompactAtomicStart(ctx, root, "", "", reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	}, snapshot, assessment, assessment.ChangedLines, lenses)
	if err != nil {
		t.Fatalf("prepare facade review compact atomic fixture: %v", err)
	}
	compactStarted, err := runReviewFacadeCompactAtomicStart(ctx, root, request)
	if err != nil {
		t.Fatalf("start facade review compact atomic fixture: %v", err)
	}
	action := "created"
	if compactStarted.Replayed {
		action = "replayed"
	}
	return reviewFacadeStartResultFor(action, len(compactStarted.Record.State.SelectedLenses) > 0, compactStarted.Record.State)
}

// facadeReviewerResultArgs admits one reviewer result per selected lens through
// the native capture route and returns the finalize argument that consumes them.
//
// It used to hand back `--result <file>` arguments, a route that bound nothing
// and has since been retired for minting approvals over results no lens
// produced. Capturing here keeps every caller's shape unchanged while making the
// fixture prove the real path.
func facadeReviewerResultArgs(t *testing.T, repo string, started ReviewFacadeStartResult) []string {
	t.Helper()
	if len(started.SelectedLenses) == 0 {
		return []string{}
	}
	directory := t.TempDir()
	resultPaths := make([]string, 0, len(started.SelectedLenses))
	for index, lens := range started.SelectedLenses {
		resultPath := filepath.Join(directory, fmt.Sprintf("reviewer-%d.json", index))
		writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
			Lens: lens, Findings: []facadeFinding{}, Evidence: []string{"reviewed exact candidate tree"},
		})
		resultPaths = append(resultPaths, resultPath)
	}
	if err := captureReviewCLIResultFiles(t, repo, started.LineageID, resultPaths); err != nil {
		t.Fatalf("capture reviewer results for the finalize fixture: %v", err)
	}
	return []string{"--captured-results=true"}
}

// TestReviewFacadeFinalizeSurfacesEscalationAccounting closes the testing
// guide's Flow 17 at the moment it describes: pushing a correction forecast
// past the frozen budget. finalize used to answer with a bare
// state=escalated, so the numbers behind the decision were reachable from no
// organic surface at all. It renders
// reviewtransaction.EscalationAccountingReasonTemplate, the same template the
// organic gate and the SDD-bound remediation surface use.
func TestReviewFacadeFinalizeSurfacesEscalationAccounting(t *testing.T) {
	// Not parallel: opting in writes the user's global mode through t.Setenv,
	// which Go forbids in a test that also calls t.Parallel.
	reviewEnabledHome(t)

	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	resultPath := filepath.Join(t.TempDir(), "review.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
		Findings: []facadeFinding{{
			Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "candidate regression",
			ProofRefs:     []string{"differential test fails only on candidate"},
			EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
		}}, Evidence: []string{"focused differential test failed"},
	})
	var output bytes.Buffer
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--result", resultPath, "--correction-lines", "3"}, &output); err != nil {
		t.Fatal(err)
	}
	got := decodeFacadeFinalize(t, output.Bytes())
	if got.State != reviewtransaction.StateEscalated {
		t.Fatalf("finalize state = %q, want escalated\n%s", got.State, output.String())
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	accounting := record.State.EscalationAccounting()
	want := fmt.Sprintf(reviewtransaction.EscalationAccountingReasonTemplate,
		accounting.Cause, accounting.Spent, accounting.Remaining, accounting.Total)
	if got.Escalation != want {
		t.Fatalf("finalize escalation = %q, want %q\n%s", got.Escalation, want, output.String())
	}
	for _, label := range []string{"spent 3", "remaining 0", "total 2"} {
		if !strings.Contains(got.Escalation, label) {
			t.Fatalf("finalize escalation = %q, want it to name %q", got.Escalation, label)
		}
	}
}

// TestReviewFacadeFinalizeOmitsEscalationWhenNotEscalated pins that the field
// stays absent for every non-escalated finalize, so the approved and
// correction-required shapes keep their exact existing output.
func TestReviewFacadeFinalizeOmitsEscalationWhenNotEscalated(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "notes.md"), []byte("a documentation note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	startFacadeReview(t, repo)
	var output bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo}, &output); err != nil {
		t.Fatal(err)
	}
	got := decodeFacadeFinalize(t, output.Bytes())
	if got.State != reviewtransaction.StateApproved {
		t.Fatalf("finalize state = %q, want approved\n%s", got.State, output.String())
	}
	if got.Escalation != "" {
		t.Fatalf("approved finalize escalation = %q, want it absent", got.Escalation)
	}
	if strings.Contains(output.String(), "escalation") {
		t.Fatalf("approved finalize output carries an escalation key:\n%s", output.String())
	}
}

func decodeFacadeFinalize(t *testing.T, payload []byte) ReviewFacadeFinalizeResult {
	t.Helper()
	var result ReviewFacadeFinalizeResult
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func assertApprovedBurnedCompactFacadeFinalize(t *testing.T, payload []byte) ReviewFacadeFinalizeResult {
	t.Helper()
	result := decodeFacadeFinalize(t, payload)
	if result.State != reviewtransaction.StateApproved || result.Action != reviewApprovedCompactBurnedFinalizeAction || result.ReceiptPath != "" {
		t.Fatalf("approved burned FINALIZE result = %#v", result)
	}
	if fields := strictReviewJSONFields(t, payload); !reflect.DeepEqual(fields, []string{"action", "lineage_id", "operation", "state", "store_revision"}) {
		t.Fatalf("approved burned FINALIZE fields = %v", fields)
	}
	return result
}

func assertApprovedBurnedCompactNegotiatedFinalize(t *testing.T, payload []byte) ReviewIntegrationFinalizeResult {
	t.Helper()
	envelope := decodeReviewOperationEnvelope(t, payload)
	var result ReviewIntegrationFinalizeResult
	decodeStrictReviewJSON(t, envelope.Result, &result)
	if result.State != reviewtransaction.StateApproved || result.Action != reviewApprovedCompactBurnedFinalizeAction || result.NextTransition != nil {
		t.Fatalf("approved burned negotiated FINALIZE result = %#v", result)
	}
	if fields := strictReviewJSONFields(t, envelope.Result); !reflect.DeepEqual(fields, []string{"action", "lineage_id", "operation", "state", "store_revision"}) {
		t.Fatalf("approved burned negotiated FINALIZE fields = %v", fields)
	}
	return result
}

func assertCompactLineageFiles(t *testing.T, store reviewtransaction.CompactStore, want []string) {
	t.Helper()
	entries, err := os.ReadDir(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(entries))
	for index, entry := range entries {
		got[index] = entry.Name()
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("compact lineage files = %v, want %v", got, want)
	}
}

type facadeReceiptPendingFixture struct {
	repo         string
	evidencePath string
	started      ReviewFacadeStartResult
	store        reviewtransaction.CompactStore
	pending      reviewtransaction.CompactRecord
	diagnostic   *ReviewFacadeReceiptPublicationError
}

func prepareFacadeReceiptPending(t *testing.T) facadeReceiptPendingFixture {
	t.Helper()
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(t.TempDir(), "review.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{Findings: []facadeFinding{}, Evidence: []string{"review complete"}})
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--lineage", started.LineageID, "--result", resultPath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidencePath, []byte("focused tests pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	validating, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := RunReview([]string{"capture-evidence", "--cwd", repo, "--lineage", started.LineageID, "--target", validating.State.InitialSnapshot.Identity, "--expected-revision", validating.Revision, "--outcome", string(reviewtransaction.VerificationOutcomePassed), "--input", evidencePath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("injected receipt publication interruption")
	original := writeCompactFacadeReceipt
	writeCompactFacadeReceipt = func(context.Context, reviewtransaction.CompactStore, reviewtransaction.CompactReceipt) error {
		return sentinel
	}
	err = RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--captured-evidence"}, io.Discard)
	writeCompactFacadeReceipt = original
	if !errors.Is(err, sentinel) {
		t.Fatalf("receipt publication error = %v, want injected cause", err)
	}
	var diagnostic *ReviewFacadeReceiptPublicationError
	if !errors.As(err, &diagnostic) {
		t.Fatalf("receipt publication error type = %T, want *ReviewFacadeReceiptPublicationError", err)
	}
	pending, loadErr := store.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if pending.State.State != reviewtransaction.StateApproved {
		t.Fatalf("pending receipt authority state = %q, want approved", pending.State.State)
	}
	return facadeReceiptPendingFixture{
		repo: repo, evidencePath: evidencePath, started: started, store: store, pending: pending, diagnostic: diagnostic,
	}
}

func TestReviewFacadeOperationDeadlineSelector(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		want      time.Duration
	}{
		{name: "start uses the start-scoped deadline", operation: "review.start", want: reviewFacadeStartOperationTimeout},
		{name: "status uses the shared deadline", operation: "review.status", want: reviewFacadeOperationTimeout},
		{name: "finalize uses the shared deadline", operation: "review.finalize", want: reviewFacadeOperationTimeout},
		{name: "validate uses the shared deadline", operation: "review.validate", want: reviewFacadeOperationTimeout},
		{name: "unknown operation uses the shared deadline", operation: "review.unknown", want: reviewFacadeOperationTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reviewFacadeOperationDeadline(tt.operation, nil); got != tt.want {
				t.Fatalf("reviewFacadeOperationDeadline(%q) = %s, want %s", tt.operation, got, tt.want)
			}
		})
	}
	if reviewFacadeStartOperationTimeout <= reviewFacadeOperationTimeout {
		t.Fatalf("reviewFacadeStartOperationTimeout = %s, want > shared deadline %s", reviewFacadeStartOperationTimeout, reviewFacadeOperationTimeout)
	}
}

// TestReviewFacadeFinalizeStateValidating is the RED-first proof for the
// shared 1663/1788 fix site: a lineage-only finalize call at StateValidating
// with no request evidence used to silently return "continue the current
// review state" without ever consuming canonical captured evidence or telling
// the caller why nothing happened.
func TestReviewFacadeFinalizeStateValidating(t *testing.T) {
	// Not parallel: opting in writes the user's global mode through t.Setenv,
	// which Go forbids in a test that also calls t.Parallel.
	reviewEnabledHome(t)

	setup := func(t *testing.T) (string, ReviewFacadeStartResult) {
		t.Helper()
		repo := initReviewCLIRepo(t)
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte(strings.Repeat("candidate line\n", 40)), 0o644); err != nil {
			t.Fatal(err)
		}
		started := startFacadeReview(t, repo)
		if len(started.SelectedLenses) == 0 {
			t.Fatal("fixture must select at least one lens to reach StateValidating without native low-risk auto-verification")
		}
		finalizeArgs := append([]string{"--cwd", repo, "--lineage", started.LineageID}, facadeReviewerResultArgs(t, repo, started)...)
		var output bytes.Buffer
		if err := RunReviewFacadeFinalize(finalizeArgs, &output); err != nil {
			t.Fatalf("submit reviewer results: %v\n%s", err, output.String())
		}
		result := decodeFacadeFinalize(t, output.Bytes())
		if result.State != reviewtransaction.StateValidating {
			t.Fatalf("fixture did not reach validating: %#v", result)
		}
		return repo, started
	}

	t.Run("issue-1663: canonical captured evidence is consumed, not ignored", func(t *testing.T) {
		repo, started := setup(t)
		store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		record, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		evidence := []byte("focused tests pass\n")
		evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
		if err := os.WriteFile(evidencePath, evidence, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := RunReviewCaptureEvidence([]string{
			"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.CurrentSnapshot.Identity,
			"--expected-revision", record.Revision, "--outcome", string(reviewtransaction.VerificationOutcomePassed), "--input", evidencePath,
		}, io.Discard); err != nil {
			t.Fatalf("capture evidence out of band: %v", err)
		}
		captured, err := reviewtransaction.ReadCapturedVerificationEvidence(store.Dir, record.State.LineageID, record.Revision, record.State.CurrentSnapshot)
		if err != nil || !bytes.Equal(captured.Payload, evidence) {
			t.Fatalf("captured verification evidence = %#v, %v", captured, err)
		}

		var output bytes.Buffer
		if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID}, &output); err != nil {
			t.Fatalf("lineage-only finalize with canonical captured evidence failed: %v\n%s", err, output.String())
		}
		result := decodeFacadeFinalize(t, output.Bytes())
		if result.State != reviewtransaction.StateApproved || result.ReceiptPath != "" || result.StoreRevision == record.Revision {
			t.Fatalf("lineage-only finalize result = %#v, want approved burned terminal result", result)
		}
		assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
		if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID}, io.Discard); err == nil {
			t.Fatal("burned lineage accepted a second terminal finalize")
		}
	})

	t.Run("invalid canonical evidence fails closed instead of becoming a no-evidence continuation", func(t *testing.T) {
		repo, started := setup(t)
		store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		before, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
		if err := os.WriteFile(evidencePath, []byte("focused tests pass\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := RunReviewCaptureEvidence([]string{
			"--cwd", repo, "--lineage", started.LineageID, "--target", before.State.CurrentSnapshot.Identity,
			"--expected-revision", before.Revision, "--outcome", string(reviewtransaction.VerificationOutcomePassed), "--input", evidencePath,
		}, io.Discard); err != nil {
			t.Fatal(err)
		}
		// Candidate-and-revision-addressed since issue #2623.
		canonical := filepath.Join(store.Dir, reviewtransaction.CompactFinalEvidenceDir,
			strings.TrimPrefix(before.State.CurrentSnapshot.Identity, "sha256:"),
			strings.TrimPrefix(before.Revision, "sha256:"), reviewtransaction.CompactFinalEvidenceFile)
		if err := os.WriteFile(canonical, nil, 0o600); err != nil {
			t.Fatal(err)
		}

		err = RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID}, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "captured verification evidence is invalid") {
			t.Fatalf("invalid canonical evidence error = %v", err)
		}
		after, loadErr := store.Load()
		if loadErr != nil || !reflect.DeepEqual(after, before) {
			t.Fatalf("invalid canonical evidence mutated authority: before %#v after %#v, error %v", before, after, loadErr)
		}
		if _, statErr := os.Stat(store.ReceiptPath()); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("invalid canonical evidence published a receipt: %v", statErr)
		}
	})

	t.Run("issue-1788: no evidence anywhere is a typed rejection, not a silent success", func(t *testing.T) {
		repo, started := setup(t)
		var output bytes.Buffer
		err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID}, &output)
		if err == nil {
			t.Fatalf("lineage-only finalize with no evidence anywhere silently succeeded: %s", output.String())
		}
		var noTransition *ErrReviewFinalizeNoTransition
		if !errors.As(err, &noTransition) {
			t.Fatalf("no-evidence finalize error = %T %v, want *ErrReviewFinalizeNoTransition", err, err)
		}
		for _, want := range []string{"gentle-ai review capture-evidence", "gentle-ai review finalize --lineage " + started.LineageID + " --captured-evidence"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("no-transition error = %q, want it to name %q verbatim", err.Error(), want)
			}
		}
	})
}

func assertFacadeReceiptReplayRejected(t *testing.T, fixture facadeReceiptPendingFixture, args []string) {
	t.Helper()
	beforeReceipt, receiptErr := os.ReadFile(fixture.store.ReceiptPath())
	if receiptErr != nil && !os.IsNotExist(receiptErr) {
		t.Fatal(receiptErr)
	}
	receiptMissing := os.IsNotExist(receiptErr)
	original := writeCompactFacadeReceipt
	writes := 0
	writeCompactFacadeReceipt = func(context.Context, reviewtransaction.CompactStore, reviewtransaction.CompactReceipt) error {
		writes++
		return errors.New("unexpected receipt write")
	}
	defer func() { writeCompactFacadeReceipt = original }()
	err := RunReviewFacadeFinalize(args, io.Discard)
	if err == nil {
		t.Fatal("unsafe receipt replay succeeded")
	}
	if writes != 0 {
		t.Fatalf("unsafe receipt replay writes = %d, want 0", writes)
	}
	after, loadErr := fixture.store.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if after.Revision != fixture.pending.Revision || !reflect.DeepEqual(after.State, fixture.pending.State) {
		t.Fatalf("unsafe replay mutated authority: before %#v after %#v", fixture.pending, after)
	}
	afterReceipt, afterReceiptErr := os.ReadFile(fixture.store.ReceiptPath())
	if os.IsNotExist(afterReceiptErr) != receiptMissing || (!receiptMissing && !bytes.Equal(afterReceipt, beforeReceipt)) {
		t.Fatalf("unsafe replay mutated receipt: before %q/%v after %q/%v", beforeReceipt, receiptErr, afterReceipt, afterReceiptErr)
	}
}
