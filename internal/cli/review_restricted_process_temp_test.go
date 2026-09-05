package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// Issues #2044 and #2132: negotiated review status failed in `pre_native` with
// the content-free `operation_failed` envelope for every reporter whose runtime
// did not expose a usable process temp directory. Sandboxed agent runtimes are
// exactly that shape, which is why the reports came from OpenCode, Claude Code
// and Codex on Windows, Linux and macOS alike while plain `review status`
// stayed healthy: only the negotiated route derives a candidate snapshot, and
// only snapshot derivation reached for the process temp directory.
//
// These tests hold the product to the repository-local behaviour on the exact
// candidate shapes the reporters ran. They are registered in the ci.yml
// windows-runtime allowlist, so the same assertions execute on windows-latest
// and on Linux through `go test ./...`.

// unavailableProcessTemp points TEMP, TMP and TMPDIR at a path that does not
// exist, reproducing a runtime whose process temp directory is unusable.
//
// The path is under t.TempDir() and is deliberately never created: an
// unwritable-but-present directory would exercise only the permission arm,
// while a missing one fails identically on Windows, where GetTempPath returns
// TMP verbatim without checking that it resolves.
func unavailableProcessTemp(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "unavailable-process-temp")
	for _, name := range []string{"TEMP", "TMP", "TMPDIR"} {
		t.Setenv(name, path)
	}
	return path
}

// requireNoReviewProcessTempResidue proves the repository-local relocation did
// not trade a process-temp dependency for durable litter inside the user's Git
// directory. Cleanup must hold on the success path and on the failure path.
func requireNoReviewProcessTempResidue(t *testing.T, repo string) {
	t.Helper()
	seen := map[string]bool{}
	for _, selector := range []string{"--git-dir", "--git-common-dir"} {
		directory := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", selector))
		if !filepath.IsAbs(directory) {
			directory = filepath.Join(repo, directory)
		}
		if seen[directory] {
			continue
		}
		seen[directory] = true
		err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			name := entry.Name()
			if strings.HasPrefix(name, ".gentle-ai-review-index-") || strings.HasPrefix(name, ".gentle-ai-frozen-git-") {
				t.Errorf("review left a temporary Git artifact behind: %s", path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", directory, err)
		}
	}
}

// negotiatedStatusArgs exercises the manual compatibility route. Generated
// runtimes declare --agent and are rejected before snapshot work when no
// executor capability is advertised.
func negotiatedStatusArgs(repo, contract string, selectors ...string) []string {
	args := []string{"status", "--cwd", repo, "--contract", contract}
	args = append(args, "--next-transition")
	return append(args, selectors...)
}

// dirtyTrackedCandidate reproduces the reporters' candidate shape: tracked
// working-tree modifications only, no staged files and no untracked candidate.
func dirtyTrackedCandidate(t *testing.T, repo string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\ncandidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestNegotiatedStatusUnderUnavailableProcessTempOffersFreshStartOnBothContracts
// covers the #2044 variant reported by G81Adolfo and doblas: a repository with
// no review history at all, where negotiated status must classify the target as
// unrelated and authorize review.start. Both published contract versions are
// asserted because reporters confirmed v1 returns the byte-equivalent failure,
// so selecting the older contract was never a workaround.
func TestNegotiatedStatusUnderUnavailableProcessTempOffersFreshStartOnBothContracts(t *testing.T) {
	reviewEnabledHome(t)
	for _, contract := range []string{ReviewIntegrationContractV1, ReviewIntegrationContractV2} {
		t.Run(contract, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			dirtyTrackedCandidate(t, repo)
			unavailableProcessTemp(t)

			var output bytes.Buffer
			if err := RunReview(negotiatedStatusArgs(repo, contract), &output); err != nil {
				t.Fatalf("negotiated status with an unavailable process temp: %v\n%s", err, output.String())
			}
			var status ReviewTargetStatusResult
			decodeStrictReviewJSON(t, output.Bytes(), &status)
			if status.Contract != contract {
				t.Fatalf("status contract = %q, want %q", status.Contract, contract)
			}
			if status.Applicability != reviewtransaction.TargetApplicabilityUnrelated ||
				status.Action != reviewtransaction.TargetStatusActionStart || status.NextTransition == nil ||
				status.NextTransition.Kind != reviewNextTransitionExecute || status.NextTransition.Execute == nil ||
				status.NextTransition.Execute.Operation != "review.start" {
				t.Fatalf("fresh negotiated status = %#v", status)
			}
			if status.Authority != nil {
				t.Fatalf("fresh negotiated status published an authority: %#v", status.Authority)
			}
			requireNoReviewProcessTempResidue(t, repo)
		})
	}
}

// TestNegotiatedStatusUnderUnavailableProcessTempResolvesWorkspaceOverlayBaseRef
// covers the second command form in #2044, which the original reporter ran
// explicitly and which failed identically. The overlay form resolves its base
// through a named ref rather than the current index, so it is a distinct
// snapshot-derivation path and needs its own proof.
func TestNegotiatedStatusUnderUnavailableProcessTempResolvesWorkspaceOverlayBaseRef(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "--abbrev-ref", "HEAD"))
	dirtyTrackedCandidate(t, repo)
	unavailableProcessTemp(t)

	var output bytes.Buffer
	if err := RunReview(negotiatedStatusArgs(repo, ReviewIntegrationContractV2,
		"--workspace-overlay", "--base-ref", base), &output); err != nil {
		t.Fatalf("workspace-overlay negotiated status with an unavailable process temp: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if strings.Contains(output.String(), "operation_outcome_unknown") {
		t.Fatalf("post-correction status returned unknown operation outcome: %s", output.String())
	}
	if status.Applicability != reviewtransaction.TargetApplicabilityUnrelated ||
		status.Action != reviewtransaction.TargetStatusActionStart || status.NextTransition == nil ||
		status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("workspace-overlay negotiated status = %#v", status)
	}
	if status.Authority != nil {
		t.Fatalf("fresh workspace-overlay status published an authority: %#v", status.Authority)
	}
	requireNoReviewProcessTempResidue(t, repo)
}

// TestNegotiatedStatusUnderUnavailableProcessTempContinuesCompactCorrection is
// #2132's own sequence, which is the variant that stranded reporters worst:
// a compact-v2 authority already in correction_required, one bounded
// correction applied, and status asked for the continuation. Reporters proved
// there was no off-ramp from this state (odjaramillo, gregoconst), so the fresh
// no-authority proof above is not sufficient on its own -- this path also
// derives a live snapshot to compare against the frozen target.
func TestNegotiatedStatusUnderUnavailableProcessTempContinuesCompactCorrection(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	const candidatePath = "internal/candidate.go"
	writeReviewStartCandidate(t, repo, candidatePath, "package candidate\n\nfunc value() int { return 1 }\n", 0o644)
	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{
		"--cwd", repo, "--lineage", "restricted-temp-correction-continuation",
	})
	if err != nil {
		t.Fatalf("start compact correction fixture: %v", err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &started)
	if len(started.SelectedLenses) == 0 {
		t.Fatalf("review start selected no lenses: %#v", started)
	}
	captureCLIReviewerResultWithFindings(t, repo, started, 0, []facadeFinding{{
		Location: candidatePath + ":3", Severity: "CRITICAL", Claim: "candidate value is wrong",
		ProofRefs: []string{candidatePath + ":3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
		CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, &bytes.Buffer{})
	captureCorrectionPlanFromCurrentStatus(t, repo, started.LineageID, 1)
	writeReviewStartCandidate(t, repo, candidatePath, "package candidate\n\nfunc value() int { return 2 }\n", 0o644)

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	unavailableProcessTemp(t)

	var output bytes.Buffer
	if err := RunReview(negotiatedStatusArgs(repo, ReviewIntegrationContractV2,
		"--lineage", started.LineageID), &output); err != nil {
		t.Fatalf("post-correction negotiated status with an unavailable process temp: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.Authority == nil || status.Authority.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("post-correction status lost the preserved authority: %#v", status.Authority)
	}
	if status.ValidationRequest != nil || status.NextTransition == nil ||
		status.NextTransition.Kind != reviewNextTransitionStop ||
		status.NextTransition.ReasonCode != "manual_intervention_required" {
		t.Fatalf("post-correction status fabricated a validator continuation without an inspectable correction candidate: %#v", status.NextTransition)
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("read-only post-correction status mutated authority: %v", err)
	}
	requireNoReviewProcessTempResidue(t, repo)
}

// TestNegotiatedStatusLeavesNoTemporaryGitArtifactsAfterFailure holds the
// cleanup contract on the arm a success-only test cannot reach. Snapshot
// derivation succeeds here and the operation still fails afterwards, on
// unreadable authority, so the temporary index must already have been removed
// by the time the typed failure is published.
func TestNegotiatedStatusLeavesNoTemporaryGitArtifactsAfterFailure(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	dirtyTrackedCandidate(t, repo)

	var startedOutput bytes.Buffer
	if err := RunReviewFacadeStart([]string{
		"--cwd", repo, "--lineage", "restricted-temp-unreadable-authority",
	}, &startedOutput); err != nil {
		t.Fatalf("facade start: %v\n%s", err, startedOutput.String())
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(startedOutput.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(store.StatePath()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(store.StatePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	unavailableProcessTemp(t)

	var output bytes.Buffer
	if err := RunReview(negotiatedStatusArgs(repo, ReviewIntegrationContractV2,
		"--lineage", started.LineageID), &output); err == nil {
		t.Fatalf("negotiated status converted unreadable authority into semantic output: %s", output.String())
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("read-only failure reported a mutation: %#v", failure)
	}
	requireNoReviewProcessTempResidue(t, repo)
}
