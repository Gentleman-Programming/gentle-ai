package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestNegotiatedReviewFinalizeRejectsStaleLiveTargetWithoutMutation(t *testing.T) {
	tests := []struct {
		name     string
		explicit bool
		breakGit bool
	}{
		{name: "explicit stale reviewing lineage", explicit: true},
		{name: "implicit sole stale reviewing lineage"},
		{name: "frozen Git evidence is unavailable", explicit: true, breakGit: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			started, store, resultArgs := startFinalizeLiveTarget(t, repo, "finalize-stale-"+reviewTestSlug(tt.name))
			record, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if tt.breakGit {
				object := record.State.CurrentSnapshot.CandidateTree
				if err := os.Remove(filepath.Join(repo, ".git", "objects", object[:2], object[2:])); err != nil {
					t.Fatalf("remove frozen candidate tree: %v", err)
				}
			} else if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("drifted\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			lineage := ""
			if tt.explicit {
				lineage = started.LineageID
			}
			assertFinalizeLiveTargetDenied(t, repo, lineage, resultArgs)
		})
	}
}

func TestNegotiatedReviewFinalizeRejectsStaleZeroTransitionWithoutMutation(t *testing.T) {
	for _, explicit := range []bool{true, false} {
		name := "implicit sole lineage"
		if explicit {
			name = "explicit lineage"
		}
		t.Run(name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
			started, store, resultArgs := startFinalizeLiveTarget(t, repo, "finalize-zero-transition-"+reviewTestSlug(name))
			args := []string{"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID}
			args = append(args, resultArgs...)
			if err := RunReview(args, &bytes.Buffer{}); err != nil {
				t.Fatalf("advance to validating: %v", err)
			}
			record, err := store.Load()
			if err != nil || record.State.State != reviewtransaction.StateValidating {
				t.Fatalf("validating authority = %#v, %v", record, err)
			}
			writeReviewStartCandidate(t, repo, "tracked.txt", "drifted\n", 0o644)
			lineage := ""
			if explicit {
				lineage = started.LineageID
			}
			assertFinalizeLiveTargetDenied(t, repo, lineage, nil)
		})
	}
}

func TestReviewFinalizeAcceptsExactLiveTargetSemantics(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, string) ([]string, func())
	}{
		{name: "workspace with intended untracked proof", prepare: func(t *testing.T, repo string) ([]string, func()) {
			writeReviewStartCandidate(t, repo, "tracked.txt", "workspace\n", 0o644)
			writeReviewStartCandidate(t, repo, "scope.txt", "included\n", 0o644)
			return nil, nil
		}},
		{name: "staged with unrelated unstaged divergence", prepare: func(t *testing.T, repo string) ([]string, func()) {
			writeReviewStartCandidate(t, repo, "tracked.txt", "staged\n", 0o644)
			runReviewCLIGit(t, repo, "add", "tracked.txt")
			return []string{"--projection", string(reviewtransaction.ProjectionStaged)}, func() {
				writeReviewStartCandidate(t, repo, "tracked.txt", "unstaged divergence\n", 0o644)
			}
		}},
		{name: "committed base diff with dirty workspace", prepare: func(t *testing.T, repo string) ([]string, func()) {
			base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
			writeReviewStartCandidate(t, repo, "tracked.txt", "committed\n", 0o644)
			runReviewCLIGit(t, repo, "add", "tracked.txt")
			runReviewCLIGit(t, repo, "commit", "-qm", "candidate")
			return []string{"--base-ref", base, "--committed-only"}, func() {
				writeReviewStartCandidate(t, repo, "tracked.txt", "dirty but excluded\n", 0o644)
			}
		}},
		{name: "base workspace overlay", prepare: func(t *testing.T, repo string) ([]string, func()) {
			base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
			writeReviewStartCandidate(t, repo, "tracked.txt", "committed\n", 0o644)
			runReviewCLIGit(t, repo, "add", "tracked.txt")
			runReviewCLIGit(t, repo, "commit", "-qm", "branch change")
			writeReviewStartCandidate(t, repo, "tracked.txt", "overlay\n", 0o644)
			return []string{"--base-ref", base, "--workspace-overlay"}, nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			startArgs, afterStart := tt.prepare(t, repo)
			started, store, resultArgs := startFinalizeLiveTarget(t, repo, "finalize-exact-"+reviewTestSlug(tt.name), startArgs...)
			if afterStart != nil {
				afterStart()
			}
			args := []string{"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID}
			args = append(args, resultArgs...)
			if err := RunReview(args, &bytes.Buffer{}); err != nil {
				t.Fatalf("exact FINALIZE failed: %v", err)
			}
			record, err := store.Load()
			if err != nil || record.State.State != reviewtransaction.StateValidating {
				t.Fatalf("exact FINALIZE authority = %#v, %v", record, err)
			}
		})
	}
}

func TestReviewFinalizeCrowdedStoreSelectsOnlyFullLiveSnapshotMatch(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "target\n", 0o644)
	writeReviewStartCandidate(t, repo, "scope.txt", "scope\n", 0o644)
	_, stale, staleResults := startFinalizeLiveTarget(t, repo, "finalize-crowded-stale")
	staleRecord, err := stale.Load()
	if err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt", "scope.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "stale candidate tree")
	writeReviewStartCandidate(t, repo, "tracked.txt", "intermediate\n", 0o644)
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "new live base")
	writeReviewStartCandidate(t, repo, "tracked.txt", "target\n", 0o644)
	live, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	// #2394: both snapshots declare no untracked scope, so their untracked
	// proofs are equal by construction and can no longer distinguish them.
	if live.CandidateTree != staleRecord.State.CurrentSnapshot.CandidateTree || live.BaseTree == staleRecord.State.CurrentSnapshot.BaseTree ||
		reflect.DeepEqual(live.Paths, staleRecord.State.CurrentSnapshot.Paths) {
		t.Fatalf("fixture does not share only CandidateTree: stale=%#v live=%#v", staleRecord.State.CurrentSnapshot, live)
	}
	assertFinalizeLiveTargetDenied(t, repo, staleRecord.State.LineageID, staleResults)
	changed, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ChangedLines(context.Background(), live)
	if err != nil {
		t.Fatal(err)
	}
	exactState, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: "finalize-crowded-exact", Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1, Snapshot: live,
		PolicyHash: staleRecord.State.PolicyHash, RiskLevel: staleRecord.State.RiskLevel,
		SelectedLenses: append([]string{}, staleRecord.State.SelectedLenses...), OriginalChangedLines: &changed,
	})
	if err != nil {
		t.Fatal(err)
	}
	exact, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, exactState.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := exact.Replace("", "review/start", exactState); err != nil {
		t.Fatal(err)
	}
	staleBytes := readReviewOperationFile(t, stale.StatePath())
	resultArgs := facadeReviewerResultArgs(t, repo, ReviewFacadeStartResult{LineageID: exactState.LineageID, SelectedLenses: exactState.SelectedLenses})
	args := []string{"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo}
	args = append(args, resultArgs...)
	if err := RunReview(args, &bytes.Buffer{}); err != nil {
		t.Fatalf("crowded exact FINALIZE failed: %v", err)
	}
	exactRecord, err := exact.Load()
	if err != nil || exactRecord.State.State != reviewtransaction.StateValidating {
		t.Fatalf("crowded exact authority = %#v, %v", exactRecord, err)
	}
	if got := readReviewOperationFile(t, stale.StatePath()); !bytes.Equal(got, staleBytes) {
		t.Fatal("crowded FINALIZE mutated stale lineage")
	}
	// The fixture admitted this lineage's reviewer results before the crowded
	// FINALIZE ran; what must not change is the state, which is asserted above.
	assertCompactLineageFiles(t, stale, []string{"review-state.json", "reviewer-results"})
}

func TestReviewFinalizeConvergesCommittedPendingJournalAfterWorktreeDrift(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	started, store, resultArgs := startFinalizeLiveTarget(t, repo, "finalize-pending-drift")
	args := append([]string{"--cwd", repo, "--lineage", started.LineageID}, resultArgs...)
	sentinel := errors.New("interrupt after committed transition")
	original := reviewFacadeCommittedTransitionHook
	reviewFacadeCommittedTransitionHook = func(_ context.Context, _ string, operation, _ string) error {
		if operation == "review/complete-review" {
			return sentinel
		}
		return nil
	}
	t.Cleanup(func() { reviewFacadeCommittedTransitionHook = original })
	if err := RunReviewFacadeFinalize(args, &bytes.Buffer{}); !errors.Is(err, sentinel) {
		t.Fatalf("committed interruption = %v", err)
	}
	reviewFacadeCommittedTransitionHook = original
	committed, err := store.Load()
	if err != nil || committed.State.State != reviewtransaction.StateValidating {
		t.Fatalf("committed pending authority = %#v, %v", committed, err)
	}
	writeReviewStartCandidate(t, repo, "tracked.txt", "later drift\n", 0o644)
	if err := RunReviewFacadeFinalize(args, &bytes.Buffer{}); err != nil {
		t.Fatalf("exact pending replay after drift: %v", err)
	}
	after, err := store.Load()
	if err != nil || after.Revision != committed.Revision || !reflect.DeepEqual(after.State, committed.State) {
		t.Fatalf("pending replay changed committed authority: before=%#v after=%#v err=%v", committed, after, err)
	}
	if pending, err := store.PendingFinalizeAttempt(); err != nil || pending != nil {
		t.Fatalf("pending replay did not converge journal: %#v, %v", pending, err)
	}
}

func TestReviewFinalizeBindsCorrectedRetrySuccessorToLiveFixDiff(t *testing.T) {
	for _, drift := range []bool{false, true} {
		name := "exact"
		if drift {
			name = "drift denied"
		}
		t.Run(name, func(t *testing.T) {
			fixture := failedCorrectedFinalVerificationCLIFixture(t)
			successor := "finalize-retry-" + reviewTestSlug(name)
			if err := RunReview(finalVerificationRetryCLIArgs(t, fixture, successor, fixture.incidentPath), &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), fixture.repo, successor)
			if err != nil {
				t.Fatal(err)
			}
			record, err := store.Load()
			if err != nil || record.State.State != reviewtransaction.StateValidating || record.State.CurrentSnapshot.Kind != reviewtransaction.TargetFixDiff {
				t.Fatalf("retry successor = %#v, %v", record, err)
			}
			evidence := filepath.Join(t.TempDir(), "evidence.txt")
			if err := os.WriteFile(evidence, []byte("retry verification passed\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if drift {
				writeReviewStartCandidate(t, fixture.repo, "tracked.txt", "retry drift\n", 0o644)
			}
			before := cliReviewAuthoritySnapshot(t, fixture.repo)
			var output bytes.Buffer
			err = RunReview([]string{"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", fixture.repo,
				"--lineage", successor, "--evidence", evidence}, &output)
			if !drift {
				if err != nil {
					t.Fatalf("exact retry FINALIZE: %v", err)
				}
				terminal, loadErr := store.Load()
				if loadErr != nil || terminal.State.State != reviewtransaction.StateApproved {
					t.Fatalf("exact retry terminal = %#v, %v", terminal, loadErr)
				}
				return
			}
			if err == nil {
				t.Fatalf("drifted retry FINALIZE succeeded: %s", output.String())
			}
			failure := decodeReviewIntegrationFailure(t, output.Bytes())
			if failure.Phase != "preflight" || failure.MutationOutcome != ReviewMutationNotStarted || failure.LineageID != successor {
				t.Fatalf("drifted retry failure = %#v", failure)
			}
			if after := cliReviewAuthoritySnapshot(t, fixture.repo); !reflect.DeepEqual(after, before) {
				t.Fatalf("drifted retry mutated authority: before=%v after=%v", before, after)
			}
		})
	}
}

func startFinalizeLiveTarget(t *testing.T, repo, lineage string, extra ...string) (ReviewFacadeStartResult, reviewtransaction.CompactStore, []string) {
	t.Helper()
	isBaseDiff := false
	for _, item := range extra {
		if item == "--base-ref" || item == "--workspace-overlay" ||
			strings.HasPrefix(item, "--base-ref=") || strings.HasPrefix(item, "--workspace-overlay=") {
			isBaseDiff = true
			break
		}
	}
	var output bytes.Buffer
	var started ReviewFacadeStartResult
	if isBaseDiff {
		// A base-diff/workspace-overlay candidate large enough to select a
		// lens now refuses a direct start up front (issue #2447); this
		// helper is SETUP for the finalize behavior under test, so it starts
		// through the negotiated contract instead. boundNegotiatedStartArgs
		// derives its own --committed-only from --base-ref, so drop any copy
		// already present to avoid a duplicate flag.
		filtered := make([]string, 0, len(extra))
		for _, item := range extra {
			if item == "--committed-only" || strings.HasPrefix(item, "--committed-only=") {
				continue
			}
			filtered = append(filtered, item)
		}
		startArgs := boundNegotiatedStartArgs(t, append([]string{
			"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage,
		}, filtered...))
		if err := RunReview(startArgs, &output); err != nil {
			t.Fatal(err)
		}
	} else {
		args := []string{"--cwd", repo, "--lineage", lineage}
		args = append(args, extra...)
		if err := RunReviewFacadeStart(args, &output); err != nil {
			t.Fatal(err)
		}
	}
	if isBaseDiff {
		// The negotiated envelope carries additional fields (schema, contract,
		// repository_context, ...) that ReviewFacadeStartResult does not
		// declare, so a strict decode here would fault on plumbing, not on
		// anything this helper's callers actually assert.
		if err := json.Unmarshal(output.Bytes(), &started); err != nil {
			t.Fatal(err)
		}
	} else {
		decodeStrictReviewJSON(t, output.Bytes(), &started)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	return started, store, facadeReviewerResultArgs(t, repo, started)
}
func assertFinalizeLiveTargetDenied(t *testing.T, repo, lineage string, resultArgs []string) {
	t.Helper()
	before := cliReviewAuthoritySnapshot(t, repo)
	args := []string{"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo}
	if lineage != "" {
		args = append(args, "--lineage", lineage)
	}
	args = append(args, resultArgs...)
	var output bytes.Buffer
	if err := RunReview(args, &output); err == nil {
		t.Fatalf("stale FINALIZE succeeded: %s", output.String())
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	// Reading admitted reviewer results needs the frozen candidate context, so a
	// repository whose frozen Git evidence was destroyed fails as a typed Git
	// failure before preflight rather than as a stale target. Both are denials
	// that start no mutation, which is the property this helper exists to hold;
	// the phase differs because the two states are genuinely different.
	staleShape := failure.Phase == "preflight" && failure.RetrySafe
	gitEvidenceLost := failure.Phase == "pre_native" && failure.Code == "git_command_failed" && failure.NextAction == "stop"
	if failure.Operation != ReviewIntegrationOperationFinalize || failure.MutationOutcome != ReviewMutationNotStarted ||
		!(staleShape || gitEvidenceLost) || lineage != "" && failure.LineageID != lineage {
		t.Fatalf("stale FINALIZE failure = %#v", failure)
	}
	if after := cliReviewAuthoritySnapshot(t, repo); !reflect.DeepEqual(after, before) {
		t.Fatalf("stale FINALIZE mutated durable authority: before=%v after=%v", before, after)
	}
}

func TestNegotiatedReviewFinalizeRejectsReviewerPreflightWithoutAuthorityMutation(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		missing bool
	}{
		{name: "missing result file", missing: true},
		{name: "malformed JSON", payload: `{`},
		{name: "invalid shape", payload: `{}`},
		{name: "non-concrete exact sentinel", payload: `{"findings":[],"evidence":["tbd"]}`},
		{name: "selected lens mismatch", payload: `{"lens":"review-risk","findings":[],"evidence":["reviewed exact candidate tree"]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			lineage := "review-preflight-" + reviewTestSlug(tt.name)
			var startedOutput bytes.Buffer
			if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, &startedOutput); err != nil {
				t.Fatal(err)
			}
			var started ReviewFacadeStartResult
			decodeStrictReviewJSON(t, startedOutput.Bytes(), &started)
			if !reflect.DeepEqual(started.SelectedLenses, []string{reviewtransaction.LensReliability}) {
				t.Fatalf("selected lenses = %v, want reliability", started.SelectedLenses)
			}
			store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
			if err != nil {
				t.Fatal(err)
			}
			before, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			beforeBytes := readReviewOperationFile(t, store.StatePath())

			resultPath := filepath.Join(t.TempDir(), "reviewer.json")
			if !tt.missing {
				if err := os.WriteFile(resultPath, []byte(tt.payload), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var output bytes.Buffer
			runErr := RunReview([]string{
				"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
				"--lineage", lineage, "--result", resultPath,
			}, &output)
			if runErr == nil {
				t.Fatalf("invalid reviewer payload succeeded: %s", output.String())
			}
			failure := decodeReviewIntegrationFailure(t, output.Bytes())
			if failure.Operation != ReviewIntegrationOperationFinalize || failure.Phase != "preflight" ||
				failure.Code != "invalid_request" || failure.MutationOutcome != ReviewMutationNotStarted ||
				!failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityNotReplayable ||
				failure.NextAction != "correct_request" || failure.LineageID != lineage {
				t.Fatalf("preflight failure = %#v", failure)
			}
			after, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if after.Revision != before.Revision || !bytes.Equal(readReviewOperationFile(t, store.StatePath()), beforeBytes) {
				t.Fatalf("invalid reviewer payload changed compact authority: before=%s after=%s", before.Revision, after.Revision)
			}
		})
	}
}

func TestNegotiatedReviewFinalizeRetriesSameLineageAfterReviewerSchemaRejection(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lineage := "review-preflight-retry"
	var startedOutput bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, &startedOutput); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedOutput.Bytes(), &started)
	if !reflect.DeepEqual(started.SelectedLenses, []string{reviewtransaction.LensReliability}) {
		t.Fatalf("selected lenses = %v, want reliability", started.SelectedLenses)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	beforeBytes := readReviewOperationFile(t, store.StatePath())

	// The reviewer payload is now rejected where it is admitted rather than at
	// finalize, because finalize only consumes results the provider already
	// admitted. The property under test is unchanged: a rejected payload leaves
	// the lineage retryable with a corrected one.
	resultPath := filepath.Join(t.TempDir(), "reviewer.json")
	if err := os.WriteFile(resultPath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{
		"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--lineage", lineage, "--captured-results=true",
	}
	var output bytes.Buffer
	if err := captureReviewCLIResultFiles(t, repo, lineage, []string{resultPath}); err == nil {
		t.Fatal("invalid reviewer payload was admitted")
	}
	rejected, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Revision != before.Revision || !bytes.Equal(readReviewOperationFile(t, store.StatePath()), beforeBytes) {
		t.Fatalf("schema rejection changed compact authority: before=%s after=%s", before.Revision, rejected.Revision)
	}
	if _, err := os.Stat(store.FinalizeAttemptJournalPath()); !os.IsNotExist(err) {
		t.Fatalf("schema rejection created finalize attempt journal: %v", err)
	}

	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
		Lens: reviewtransaction.LensReliability, Findings: []facadeFinding{}, Evidence: []string{"reviewed exact candidate tree"},
	})
	if err := captureReviewCLIResultFiles(t, repo, lineage, []string{resultPath}); err != nil {
		t.Fatalf("corrected reviewer payload capture: %v", err)
	}
	output.Reset()
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("corrected reviewer payload retry: %v\n%s", err, output.String())
	}
	var finalized ReviewFacadeFinalizeResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, output.Bytes()).Result, &finalized)
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if finalized.LineageID != lineage || finalized.State != reviewtransaction.StateValidating ||
		finalized.StoreRevision != after.Revision || after.Revision == before.Revision {
		t.Fatalf("finalize retry result = %#v, authority = %#v", finalized, after)
	}
	if after.State.LineageID != before.State.LineageID || after.State.Generation != before.State.Generation ||
		after.State.CorrectionBudget != before.State.CorrectionBudget {
		t.Fatalf("finalize retry changed frozen review identity or budget: before=%#v after=%#v", before.State, after.State)
	}
}

func TestNegotiatedReviewFinalizeRequiresExplicitLineageWhenAuthorityIsAmbiguous(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var startedOutput bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-finalize-ambiguous-a"}, &startedOutput); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedOutput.Bytes(), &started)
	first, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	firstRecord, err := first.Load()
	if err != nil {
		t.Fatal(err)
	}
	lines := firstRecord.State.OriginalChangedLines
	clone, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: "review-finalize-ambiguous-b", Mode: reviewtransaction.ModeOrdinaryBounded, Generation: firstRecord.State.Generation,
		Snapshot: firstRecord.State.InitialSnapshot, PolicyHash: firstRecord.State.PolicyHash, RiskLevel: firstRecord.State.RiskLevel,
		SelectedLenses: append([]string{}, firstRecord.State.SelectedLenses...), OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, clone.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.Replace("", "review/start", clone); err != nil {
		t.Fatal(err)
	}
	before := map[string][]byte{}
	for _, store := range []reviewtransaction.CompactStore{first, second} {
		before[store.StatePath()], err = os.ReadFile(store.StatePath())
		if err != nil {
			t.Fatal(err)
		}
	}
	// Ambiguity is reported by discovery, so the request only has to name a
	// reviewer-result source that survives; it never gets far enough to read one.
	args := []string{"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--captured-results=true"}
	var output bytes.Buffer
	if err := RunReview(args, &output); err == nil {
		t.Fatalf("ambiguous FINALIZE error = %v\n%s", err, output.String())
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.MutationOutcome != ReviewMutationUnknown || failure.Replayability != reviewtransaction.ReplayabilityStatusRequired || failure.NextAction != "review.status" {
		t.Fatalf("ambiguous FINALIZE failure = %#v", failure)
	}
	for _, store := range []reviewtransaction.CompactStore{first, second} {
		after, readErr := os.ReadFile(store.StatePath())
		if readErr != nil || !bytes.Equal(before[store.StatePath()], after) {
			t.Fatalf("ambiguous FINALIZE changed %s: %v", store.StatePath(), readErr)
		}
		if _, statErr := os.Stat(store.FinalizeAttemptJournalPath()); !os.IsNotExist(statErr) {
			t.Fatalf("ambiguous FINALIZE created journal for %s: %v", store.StatePath(), statErr)
		}
	}

	resultPath := filepath.Join(t.TempDir(), "reviewer.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
		Lens: started.SelectedLenses[0], Findings: []facadeFinding{}, Evidence: []string{"reviewed exact candidate tree"},
	})
	if err := captureReviewCLIResultFiles(t, repo, started.LineageID, []string{resultPath}); err != nil {
		t.Fatalf("capture reviewer result for the named lineage: %v", err)
	}
	output.Reset()
	args = append(args, "--lineage", started.LineageID)
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("explicit FINALIZE failed: %v\n%s", err, output.String())
	}
	finalized, err := first.Load()
	if err != nil || finalized.State.State != reviewtransaction.StateValidating {
		t.Fatalf("explicit FINALIZE authority = %#v, %v", finalized, err)
	}
	secondAfter, err := os.ReadFile(second.StatePath())
	if err != nil || !bytes.Equal(before[second.StatePath()], secondAfter) {
		t.Fatalf("explicit FINALIZE changed unselected authority: %v", err)
	}
}

func TestPrepareCompactReviewerResultsPreservesSelectedLensOrderAndAliases(t *testing.T) {
	selected := []string{
		reviewtransaction.LensRisk,
		reviewtransaction.LensResilience,
		reviewtransaction.LensReadability,
		reviewtransaction.LensReliability,
	}
	results := []facadeReviewerResult{
		{Lens: "risk", Findings: []facadeFinding{}, Evidence: []string{"risk review evidence"}},
		{Lens: reviewtransaction.LensResilience, Findings: []facadeFinding{}, Evidence: []string{"resilience review evidence"}},
		{Lens: "readability", Findings: []facadeFinding{}, Evidence: []string{"readability review evidence"}},
		{Lens: reviewtransaction.LensReliability, Findings: []facadeFinding{}, Evidence: []string{"reliability review evidence"}},
	}
	input, err := prepareCompactReviewerResults(reviewtransaction.CompactState{SelectedLenses: selected}, results, facadeRefuterResult{})
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(input.LensResults))
	for index := range input.LensResults {
		got[index] = input.LensResults[index].Lens
	}
	if !reflect.DeepEqual(got, selected) {
		t.Fatalf("canonical lens order = %v, want %v", got, selected)
	}
}

func reviewTestSlug(value string) string {
	result := make([]byte, 0, len(value))
	separator := false
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			result = append(result, byte(char))
			separator = false
			continue
		}
		if len(result) > 0 && !separator {
			result = append(result, '-')
			separator = true
		}
	}
	for len(result) > 0 && result[len(result)-1] == '-' {
		result = result[:len(result)-1]
	}
	return string(result)
}
