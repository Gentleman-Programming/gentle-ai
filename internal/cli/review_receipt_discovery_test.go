package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func assertScopeChangeRecovery(t *testing.T, failure ReviewIntegrationFailure, lineage, privatePath string) {
	if failure.Context == nil || failure.Context.ScopeChange == nil {
		t.Fatalf("scope-change context = %#v", failure)
	}
	scope := failure.Context.ScopeChange
	if scope.PredecessorLineageID != lineage || scope.PredecessorRevision == "" || scope.Expected.CandidateTree == "" || scope.Expected.PathsDigest == "" || scope.Actual.CandidateTree == "" || scope.Actual.PathsDigest == "" || scope.DifferingPathCount != 1 || !reflect.DeepEqual(failure.RequiredInputs, []string{"predecessor_lineage_id", "expected_predecessor_revision", "successor_lineage_id", "disposition", "reason", "actor"}) || !reflect.DeepEqual(scope.RecoveryRequiredInputs, failure.RequiredInputs) {
		t.Fatalf("scope-change provenance = %#v", failure)
	}
	payload, err := json.Marshal(failure)
	if err != nil || strings.Contains(string(payload), privatePath) || strings.Contains(string(payload), `"paths"`) || strings.Contains(string(payload), `"differing_paths"`) {
		t.Fatalf("scope-change failure exposed private paths: %s, %v", payload, err)
	}
}

func TestUnqualifiedGateDiscoverySelectsOneExactReceiptAcrossUnrelatedHistory(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	first, _ := approveDiscoveryMarkdown(t, repo, "review-discovery-first", "docs/first.md", "first\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "first reviewed target")
	second, store := approveDiscoveryMarkdown(t, repo, "review-discovery-second", "docs/second.md", "second\n")
	receipt, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	receipt = bytes.Replace(receipt, []byte(`"selected_lenses": []`), []byte(`"selected_lenses": null`), 1)
	if err := os.WriteFile(store.ReceiptPath(), receipt, 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &output); err != nil {
		t.Fatalf("one exact receipt among unrelated history: %v\n%s", err, output.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, output.Bytes()).Result, &result)
	if !result.Allowed || result.Context.LineageID != second.LineageID || result.Context.LineageID == first.LineageID {
		t.Fatalf("exact receipt selection = %#v", result)
	}
}

// TestPreCommitGateDiscoverySkipsAssessmentForGenesisDisjointTerminalLeaves is
// organic-dx Phase 3d's RED/GREEN proof: as terminal lineages accumulate,
// discoverCompactFacadeGateReview must not pay the full, git-subprocess-heavy
// AssessCompactGateTarget cost for a lineage that provably cannot govern the
// live pre-commit candidate (its genesis paths are disjoint from every path
// the live staged candidate touches). Before the fix every terminal leaf is
// assessed individually (assessed == n); after the fix only the one leaf
// whose frozen candidate tree happens to equal the current HEAD tree (an
// "exact recommit" branch AssessCompactGateTarget itself special-cases) still
// needs the expensive path.
func TestPreCommitGateDiscoverySkipsAssessmentForGenesisDisjointTerminalLeaves(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	const n = 12
	for index := 0; index < n; index++ {
		lineage := fmt.Sprintf("pre-commit-discovery-leaf-%02d", index)
		logicalPath := fmt.Sprintf("docs/pre-commit-leaf-%02d.md", index)
		approveDiscoveryMarkdown(t, repo, lineage, logicalPath, fmt.Sprintf("leaf %d\n", index))
		runReviewCLIGit(t, repo, "add", "-A")
		runReviewCLIGit(t, repo, "commit", "-qm", lineage)
	}
	// Shift HEAD away from every leaf's own recorded candidate tree so none
	// of them can take AssessCompactGateTarget's "exact recommit" branch by
	// accident of this fixture's own construction.
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("shifted head\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "shift head away from every leaf candidate")

	// The live pre-commit candidate: a staged change on a path disjoint from
	// every leaf's genesis paths above.
	if err := os.WriteFile(filepath.Join(repo, "live-candidate.txt"), []byte("live staged change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "-A")

	assessed := 0
	original := assessCompactGateTargetForDiscovery
	assessCompactGateTargetForDiscovery = func(ctx context.Context, repo string, state reviewtransaction.CompactState, input reviewtransaction.NativeGateRequestInput) (reviewtransaction.CompactGateTargetAssessment, error) {
		assessed++
		return original(ctx, repo, state, input)
	}
	t.Cleanup(func() { assessCompactGateTargetForDiscovery = original })

	_, _, discoveryErr := discoverCompactFacadeGateReview(context.Background(), repo, "", reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePreCommit})
	var discovery *ReviewReceiptDiscoveryError
	// issue #3408: the unrelated denial no longer enumerates the terminal
	// leaves it walked -- none of them is actionable for this candidate -- so
	// the surviving observable is the kind, and the skip count below is what
	// this test actually proves.
	if !errors.As(discoveryErr, &discovery) || discovery.Kind != ReviewReceiptUnrelated || len(discovery.Candidates) != 0 {
		t.Fatalf("pre-commit discovery over %d disjoint terminal leaves = %#v, err=%v", n, discovery, discoveryErr)
	}
	if assessed >= n {
		t.Fatalf("expensive per-leaf assessment ran %d times for %d provably-unrelated terminal leaves, want fewer", assessed, n)
	}
	t.Logf("expensive per-leaf assessment ran %d times for %d provably-unrelated terminal leaves", assessed, n)
}

// TestPreCommitGateDiscoveryStillSelectsExactReceiptAmongDisjointNoiseLineages
// is organic-dx Phase 3d's task 3d.4 non-negotiable regression proof: the
// fast path introduced above must be byte-identical for any lineage that
// governs the candidate or contributes to a discovery bucket. It builds the
// exact same disjoint-noise history as the sibling test above, but the LIVE
// staged candidate is the genuine, unmodified output of one of the reviewed
// lineages -- so it must still be discovered and selected exactly as
// AssessCompactGateTarget's un-skipped path would.
func TestPreCommitGateDiscoveryStillSelectsExactReceiptAmongDisjointNoiseLineages(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	const n = 8
	for index := 0; index < n; index++ {
		lineage := fmt.Sprintf("pre-commit-exact-noise-%02d", index)
		logicalPath := fmt.Sprintf("docs/pre-commit-exact-noise-%02d.md", index)
		approveDiscoveryMarkdown(t, repo, lineage, logicalPath, fmt.Sprintf("noise %d\n", index))
		runReviewCLIGit(t, repo, "add", "-A")
		runReviewCLIGit(t, repo, "commit", "-qm", lineage)
	}
	governing, _ := approveDiscoveryMarkdownProjection(t, repo, "pre-commit-exact-governing", "docs/pre-commit-exact-governing.md", "governs\n", reviewtransaction.ProjectionStaged)

	assessed := 0
	original := assessCompactGateTargetForDiscovery
	assessCompactGateTargetForDiscovery = func(ctx context.Context, repo string, state reviewtransaction.CompactState, input reviewtransaction.NativeGateRequestInput) (reviewtransaction.CompactGateTargetAssessment, error) {
		assessed++
		return original(ctx, repo, state, input)
	}
	t.Cleanup(func() { assessCompactGateTargetForDiscovery = original })

	_, record, discoveryErr := discoverCompactFacadeGateReview(context.Background(), repo, "", reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePreCommit})
	if discoveryErr != nil {
		t.Fatalf("exact receipt among %d disjoint noise lineages failed: %v", n, discoveryErr)
	}
	if record.State.LineageID != governing.LineageID {
		t.Fatalf("selected lineage = %q, want %q", record.State.LineageID, governing.LineageID)
	}
	if assessed > n {
		t.Fatalf("expensive assessment ran %d times for %d noise lineages plus the governing one, want at most %d", assessed, n, n+1)
	}
	t.Logf("expensive per-leaf assessment ran %d times for %d disjoint noise lineages plus the governing one", assessed, n)
}

// TestUnrelatedReceiptDenialNamesTheCandidateSituationNotOtherLineages is
// issue #3408 at the live gate boundary. The denial itself is correct and
// stays a denial: a candidate with no approved receipt must be refused.
// What was wrong is what the refusal REPORTED. It carried every terminal
// lineage the discovery walk had examined -- an enumeration that grows with
// the store, because lineages accumulate and nothing reaps them (#1656) --
// and worded itself as "terminal review receipts exist only for unrelated
// targets", so an operator read it as "some old lineage is blocking me",
// went looking for the connection, and found none, because there is none.
//
// Every lineage in that set assessed as UNRELATED to this candidate. None of
// them can be selected, recovered, or acted on here, so none of them belongs
// in the refusal. The refusal must state the candidate's own situation and
// the one route out of it.
func TestUnrelatedReceiptDenialNamesTheCandidateSituationNotOtherLineages(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	const n = 6
	lineages := make([]string, 0, n)
	for index := 0; index < n; index++ {
		lineage := fmt.Sprintf("unrelated-denial-noise-%02d", index)
		approveDiscoveryMarkdown(t, repo, lineage, fmt.Sprintf("docs/unrelated-denial-%02d.md", index), fmt.Sprintf("noise %d\n", index))
		runReviewCLIGit(t, repo, "add", "-A")
		runReviewCLIGit(t, repo, "commit", "-qm", lineage)
		lineages = append(lineages, lineage)
	}
	// The live candidate touches a path none of those lineages reviewed, so
	// discovery classifies every one of them unrelated and denies.
	if err := os.WriteFile(filepath.Join(repo, "live-candidate.txt"), []byte("live staged change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "-A")

	var output bytes.Buffer
	gateErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &output)
	var denied ReviewGateDeniedError
	if !errors.As(gateErr, &denied) {
		t.Fatalf("candidate with no approved receipt was not denied: %T %v\n%s", gateErr, gateErr, output.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Allowed || result.Context.Denial == nil || result.Context.Denial.Code != string(ReviewReceiptUnrelated) {
		t.Fatalf("denial = %#v, want a fail-closed %q\n%s", result.Context.Denial, ReviewReceiptUnrelated, output.String())
	}
	if !strings.Contains(result.Reason, "no approved review receipt covers this candidate") ||
		!strings.Contains(result.Reason, "gentle-ai review start") {
		t.Fatalf("denial reason = %q, want it to lead with this candidate's own situation and name its route", result.Reason)
	}
	payload := output.String()
	for _, lineage := range lineages {
		if strings.Contains(payload, lineage) {
			t.Fatalf("denial reported unrelated lineage %q as though it were the obstacle:\n%s", lineage, payload)
		}
	}

	// The typed discovery error carries no enumeration either, so no other
	// renderer can grow one back.
	_, _, discoveryErr := discoverCompactFacadeGateReview(context.Background(), repo, "", reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePreCommit})
	var discovery *ReviewReceiptDiscoveryError
	if !errors.As(discoveryErr, &discovery) || discovery.Kind != ReviewReceiptUnrelated || len(discovery.Candidates) != 0 {
		t.Fatalf("unrelated discovery over %d terminal lineages = %#v, err=%v", n, discovery, discoveryErr)
	}

	// The negotiated failure envelope leads with the same statement.
	failure := newReviewIntegrationFailure("review.validate", nil, discoveryErr)
	if failure.Code != string(ReviewReceiptUnrelated) ||
		!strings.Contains(failure.Message, "No approved review receipt covers this candidate") ||
		!strings.Contains(failure.Message, "gentle-ai review start") {
		t.Fatalf("negotiated unrelated failure = %#v", failure)
	}
}

func TestReceiptlessTerminalLegacyChainIsInventoryReadableButNeverGateAuthority(t *testing.T) {
	reviewEnabledHome(t)
	fixture := newLegacyCLIFixture(t, "legacy-pre-receipt")
	if err := os.Remove(fixture.receiptPath); err != nil {
		t.Fatal(err)
	}

	report, err := reviewtransaction.InventoryAuthority(context.Background(), fixture.repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !report.Authoritative {
		t.Fatalf("receiptless terminal legacy chain forced incomplete inventory = %#v", report)
	}
	if len(report.Entries) != 1 || report.Entries[0].Status != reviewtransaction.AuthorityStatusHistorical {
		t.Fatalf("receiptless terminal legacy entry = %#v", report.Entries)
	}

	gateInput := reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePostApply}
	_, _, discoveryErr := discoverCompactFacadeGateReview(context.Background(), fixture.repo, "", gateInput)
	var discovery *ReviewReceiptDiscoveryError
	if !errors.As(discoveryErr, &discovery) || discovery.Kind != ReviewReceiptMissing {
		t.Fatalf("receiptless legacy gate discovery = %#v, %v", discovery, discoveryErr)
	}
	if exact := legacyExactFacadeGateLineages(context.Background(), fixture.repo, gateInput); exact != 0 {
		t.Fatalf("receiptless legacy chain counted as exact gate lineage %d times", exact)
	}
	if _, _, _, legacyErr := discoverFacadeReview(context.Background(), fixture.repo, fixture.lineage, true); legacyErr == nil {
		t.Fatal("receiptless legacy chain was discovered as terminal facade authority")
	}

	var output bytes.Buffer
	err = RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", fixture.repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &output)
	if err == nil {
		t.Fatal("receiptless legacy chain acted as gate authority")
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Code != "receipt_missing" || failure.MutationOutcome != ReviewMutationNotStarted || failure.RetrySafe {
		t.Fatalf("receiptless legacy gate failure = %#v", failure)
	}
}

func TestUnqualifiedGateDiscoveryReturnsTypedMissingAndScopeChanged(t *testing.T) {
	reviewEnabledHome(t)
	t.Run("missing", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		var output bytes.Buffer
		err := RunReview([]string{
			"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
			"--gate", string(reviewtransaction.GatePostApply),
		}, &output)
		if err == nil {
			t.Fatal("missing receipt validation succeeded")
		}
		failure := decodeReviewIntegrationFailure(t, output.Bytes())
		// organic-dx Phase 3b task 3b.1: an agent running a gate before any
		// governing review exists is now told the way in (review.start)
		// instead of a bare stop.
		if failure.Code != "receipt_missing" || failure.MutationOutcome != ReviewMutationNotStarted || failure.RetrySafe || failure.NextAction != "review.start" {
			t.Fatalf("missing receipt failure = %#v", failure)
		}
	})

	t.Run("scope changed", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		_, _ = approveDiscoveryMarkdown(t, repo, "review-discovery-scope", "docs/reviewed.md", "reviewed\n")
		if err := os.WriteFile(filepath.Join(repo, "docs", "reviewed.md"), []byte("drifted\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		err := RunReview([]string{
			"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
			"--gate", string(reviewtransaction.GatePostApply),
		}, &output)
		if err == nil {
			t.Fatal("scope-changed receipt validation succeeded")
		}
		failure := decodeReviewIntegrationFailure(t, output.Bytes())
		if failure.Code != "receipt_scope_changed" || failure.AuthorityApplicability != "current_target" || failure.RetrySafe ||
			failure.Replayability != reviewtransaction.ReplayabilityManualActionRequired || failure.NextAction != "explicit-maintainer-action" {
			t.Fatalf("scope-changed receipt failure = %#v", failure)
		}
		assertScopeChangeRecovery(t, failure, "review-discovery-scope", "docs/reviewed.md")
	})

	t.Run("unrelated", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		_, store := approveDiscoveryMarkdown(t, repo, "review-discovery-unrelated", "docs/reviewed.md", "reviewed\n")
		runReviewCLIGit(t, repo, "add", "-A")
		runReviewCLIGit(t, repo, "commit", "-qm", "reviewed target")
		if err := os.WriteFile(filepath.Join(repo, "unrelated.txt"), []byte("unrelated\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runReviewCLIGit(t, repo, "add", "-A")
		runReviewCLIGit(t, repo, "commit", "-qm", "unrelated target")
		record, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		assessment, err := reviewtransaction.AssessCompactGateTarget(context.Background(), repo, record.State, reviewtransaction.NativeGateRequestInput{
			Gate:      reviewtransaction.GateRelease,
			LineageID: record.State.LineageID,
		})
		if err != nil {
			t.Fatalf("assess unrelated release target: %v", err)
		}
		if assessment.Applicability != reviewtransaction.CompactGateTargetUnrelated {
			t.Fatalf("unrelated release applicability = %q; expected=%#v actual=%#v", assessment.Applicability, assessment.Expected, assessment.Actual)
		}
		var output bytes.Buffer
		err = RunReview([]string{
			"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
			"--gate", string(reviewtransaction.GateRelease),
		}, &output)
		if err == nil {
			t.Fatal("unrelated receipt validation succeeded")
		}
		failure := decodeReviewIntegrationFailure(t, output.Bytes())
		// organic-dx Phase 3b task 3b.1: STATUS re-derives the same
		// discovery and can name candidate lineages (or confirm none apply),
		// so the receipt_unrelated block now names review.status instead of
		// a bare stop.
		if failure.Code != "receipt_unrelated" || failure.AuthorityApplicability != "unrelated" || failure.RetrySafe || failure.NextAction != "review.status" {
			t.Fatalf("unrelated receipt failure = %#v", failure)
		}
	})
}

func TestUnqualifiedPrePushDiscoveryReportsTargetResolutionWithoutMutation(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	_, store := approveDiscoveryMarkdown(t, repo, "review-discovery-missing-upstream", "docs/reviewed.md", "reviewed\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "deliver reviewed target")
	stateBefore, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	receiptBefore, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}

	var first bytes.Buffer
	runErr := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePrePush),
	}, &first)
	if runErr == nil {
		t.Fatal("missing-upstream pre-push validation succeeded")
	}
	failure := decodeReviewIntegrationFailure(t, first.Bytes())
	if failure.Phase != "pre_native" || failure.Code != "target_resolution_failed" ||
		failure.MutationOutcome != ReviewMutationNotStarted || failure.AuthorityApplicability != "not_evaluated" ||
		!failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityNotReplayable ||
		!reflect.DeepEqual(failure.RequiredInputs, []string{"base_ref"}) || failure.NextAction != "correct_request" ||
		!strings.Contains(failure.Message, "--base-ref") {
		t.Fatalf("target-resolution failure = %#v", failure)
	}
	var targetErr *reviewtransaction.GateTargetResolutionError
	if !errors.As(runErr, &targetErr) {
		t.Fatalf("target-resolution cause = %T %v", runErr, runErr)
	}

	var second bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePrePush),
	}, &second); err == nil || !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("repeated target-resolution failure changed: %v\nfirst=%s\nsecond=%s", err, first.String(), second.String())
	}
	stateAfter, stateErr := os.ReadFile(store.StatePath())
	receiptAfter, receiptErr := os.ReadFile(store.ReceiptPath())
	if stateErr != nil || receiptErr != nil || !bytes.Equal(stateBefore, stateAfter) || !bytes.Equal(receiptBefore, receiptAfter) {
		t.Fatalf("target resolution mutated authority: state=%v receipt=%v", stateErr, receiptErr)
	}

	var raw bytes.Buffer
	rawErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePrePush)}, &raw)
	var evaluation ReviewValidateResult
	decodeStrictReviewJSON(t, raw.Bytes(), &evaluation)
	if rawErr == nil || evaluation.Result != reviewtransaction.GateInvalidated || evaluation.Context.Denial == nil ||
		evaluation.Context.Denial.Stage != "target-resolution" || evaluation.Context.Denial.Code != "target_resolution_failed" ||
		!errors.As(rawErr, &targetErr) {
		t.Fatalf("non-negotiated target resolution = %#v, %T %v", evaluation, rawErr, rawErr)
	}
}

// TestUnqualifiedPrePushDiscoveryKeepsCorruptAuthorityPrecedence pinned an
// ordering between two denials the unqualified path can no longer produce
// together, because an unrelated corrupt entry is no longer a denial of this
// candidate at all. The precedence that survives is the one that was ever
// load-bearing: naming the corrupt lineage produces the corruption
// classification, and nothing else silently takes its place.
func TestUnqualifiedPrePushDiscoveryKeepsCorruptAuthorityPrecedence(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	approveDiscoveryMarkdown(t, repo, "review-discovery-target-and-corruption", "docs/reviewed.md", "reviewed\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "deliver reviewed target")
	unqualifiedPrePush := func() (string, error) {
		var output bytes.Buffer
		err := RunReview([]string{
			"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
			"--gate", string(reviewtransaction.GatePrePush),
		}, &output)
		return output.String(), err
	}

	// The same fixture, evaluated twice, with the corrupt entry as the only
	// difference. Anything the unqualified path says about this candidate has
	// to be identical on both sides, because nothing about this candidate
	// changed.
	beforeOutput, beforeErr := unqualifiedPrePush()

	commonDir := filepath.Clean(strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir")))
	broken := filepath.Join(commonDir, "gentle-ai", "review-transactions", "v2", "corrupt-target-candidate")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "review-state.json"), []byte("{\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	afterOutput, afterErr := unqualifiedPrePush()
	if afterOutput != beforeOutput || (afterErr == nil) != (beforeErr == nil) {
		t.Fatalf("an unrelated corrupt entry changed the unqualified answer:\nbefore(%v):\n%s\nafter(%v):\n%s",
			beforeErr, beforeOutput, afterErr, afterOutput)
	}

	// Naming the corrupt lineage still fails closed, and still names it.
	var scoped bytes.Buffer
	scopedErr := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePrePush), "--lineage", "corrupt-target-candidate",
	}, &scoped)
	if scopedErr == nil {
		t.Fatal("naming the corrupt lineage validated")
	}
	if !strings.Contains(scoped.String(), "corrupt-target-candidate") {
		t.Fatalf("the scoped refusal does not name the lineage it refuses:\n%s", scoped.String())
	}
	var targetErr *reviewtransaction.GateTargetResolutionError
	if errors.As(scopedErr, &targetErr) {
		t.Fatalf("a corrupt named lineage was reported as a target-resolution failure: %v", scopedErr)
	}
}

func TestUnqualifiedGateDiscoveryRoutesCommittedNextSliceWorkspace(t *testing.T) {
	reviewEnabledHome(t)
	// #1401: after one approved slice is committed exactly as reviewed, new
	// dirty tracked work on top must classify as unrelated or scope-changed,
	// never as authority_corrupted against the healthy predecessor.
	t.Run("unrelated dirty tracked next slice", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		_, _ = approveDiscoveryMarkdown(t, repo, "review-discovery-next-slice", "docs/reviewed.md", "reviewed\n")
		runReviewCLIGit(t, repo, "add", "-A")
		runReviewCLIGit(t, repo, "commit", "-qm", "reviewed target")
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("next slice\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		err := RunReview([]string{
			"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
			"--gate", string(reviewtransaction.GatePostApply),
		}, &output)
		if err == nil {
			t.Fatal("next-slice workspace validation succeeded")
		}
		failure := decodeReviewIntegrationFailure(t, output.Bytes())
		if failure.Code == "authority_corrupted" {
			t.Fatalf("healthy committed receipt misrouted as corrupted: %#v", failure)
		}
		if failure.Code != "receipt_unrelated" || failure.AuthorityApplicability != "unrelated" || failure.RetrySafe || failure.NextAction != "review.status" {
			t.Fatalf("committed next-slice workspace failure = %#v", failure)
		}
	})

	t.Run("overlapping dirty tracked next slice", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		_, _ = approveDiscoveryMarkdown(t, repo, "review-discovery-next-slice-overlap", "docs/reviewed.md", "reviewed\n")
		runReviewCLIGit(t, repo, "add", "-A")
		runReviewCLIGit(t, repo, "commit", "-qm", "reviewed target")
		if err := os.WriteFile(filepath.Join(repo, "docs", "reviewed.md"), []byte("drifted after commit\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		err := RunReview([]string{
			"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
			"--gate", string(reviewtransaction.GatePostApply),
		}, &output)
		if err == nil {
			t.Fatal("overlapping next-slice validation succeeded")
		}
		failure := decodeReviewIntegrationFailure(t, output.Bytes())
		if failure.Code == "authority_corrupted" {
			t.Fatalf("healthy committed receipt misrouted as corrupted: %#v", failure)
		}
		if failure.Code != "receipt_scope_changed" || failure.AuthorityApplicability != "current_target" || failure.RetrySafe ||
			failure.Replayability != reviewtransaction.ReplayabilityManualActionRequired || failure.NextAction != "explicit-maintainer-action" {
			t.Fatalf("overlapping next-slice failure = %#v", failure)
		}
		assertScopeChangeRecovery(t, failure, "review-discovery-next-slice-overlap", "docs/reviewed.md")
	})
}

func TestUnqualifiedGateDiscoveryRejectsMultipleExactReceiptsButExplicitLineageIsDirect(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started, store := approveDiscoveryMarkdown(t, repo, "review-discovery-exact-a", "docs/exact.md", "exact\n")
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	lines := record.State.OriginalChangedLines
	clone, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: "review-discovery-exact-b", Mode: reviewtransaction.ModeOrdinaryBounded, Generation: record.State.Generation,
		Snapshot: record.State.InitialSnapshot, PolicyHash: record.State.PolicyHash, RiskLevel: record.State.RiskLevel,
		SelectedLenses: []string{}, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	cloneStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, clone.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := cloneStore.Replace("", "review/start", clone)
	if err != nil {
		t.Fatal(err)
	}
	if err := clone.CompleteReview(reviewtransaction.CompactReviewInput{LensResults: []reviewtransaction.LensResult{}}); err != nil {
		t.Fatal(err)
	}
	revision, err = cloneStore.Replace(revision, "review/complete-review", clone)
	if err != nil {
		t.Fatal(err)
	}
	if err := clone.CompleteVerification([]byte("independent duplicate fixture evidence"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := cloneStore.Replace(revision, "review/complete-verification", clone); err != nil {
		t.Fatal(err)
	}
	cloneReceipt, err := clone.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.WriteCompactReceiptAtomic(cloneStore.ReceiptPath(), cloneReceipt); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err = RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &output)
	if err == nil {
		t.Fatal("multiple exact receipts were selected implicitly")
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Code != "receipt_ambiguous" || failure.AuthorityApplicability != "ambiguous" || failure.RetrySafe ||
		len(failure.RequiredInputs) != 1 || failure.RequiredInputs[0] != "lineage_id" {
		t.Fatalf("ambiguous receipt failure = %#v", failure)
	}

	output.Reset()
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &output); err != nil {
		t.Fatalf("explicit exact lineage failed: %v\n%s", err, output.String())
	}
}

func TestUnqualifiedGateDiscoveryRequiresSelectionForMultipleScopeChangedReceipts(t *testing.T) {
	reviewEnabledHome(t)
	for _, tt := range []struct {
		name       string
		projection reviewtransaction.Projection
		gate       reviewtransaction.GateKind
	}{
		{name: "workspace", projection: reviewtransaction.ProjectionWorkspace, gate: reviewtransaction.GatePostApply},
		{name: "staged", projection: reviewtransaction.ProjectionStaged, gate: reviewtransaction.GatePreCommit},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			lineages := []string{"review-discovery-scope-a-" + tt.name, "review-discovery-scope-b-" + tt.name}
			logicalPath := "docs/scope-" + tt.name + ".md"
			_, first := approveDiscoveryMarkdownProjection(t, repo, lineages[0], logicalPath, "reviewed\n", tt.projection)
			cloneApprovedDiscoveryAuthority(t, repo, first, lineages[1])
			if err := os.WriteFile(filepath.Join(repo, filepath.FromSlash(logicalPath)), []byte("drifted\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if tt.projection == reviewtransaction.ProjectionStaged {
				runReviewCLIGit(t, repo, "add", "-A")
			}

			gateInput := reviewtransaction.NativeGateRequestInput{Gate: tt.gate}
			_, _, discoveryErr := discoverCompactFacadeGateReview(context.Background(), repo, "", gateInput)
			var discovery *ReviewReceiptDiscoveryError
			if !errors.As(discoveryErr, &discovery) || discovery.Kind != ReviewReceiptAmbiguous || !reflect.DeepEqual(discovery.Candidates, lineages) || discovery.Context != nil {
				t.Fatalf("multiple scope-changed discovery = %#v, %v", discovery, discoveryErr)
			}
			// organic-dx Phase 3c: both contributing lineages are
			// deterministically stale (scope-changed) -- discovery proved
			// neither governs this candidate.
			if !discovery.DeterministicallyStaleOnly {
				t.Fatalf("multiple scope-changed discovery composition = %#v, want deterministically stale only", discovery)
			}

			var output bytes.Buffer
			err := RunReview([]string{
				"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--gate", string(tt.gate),
			}, &output)
			if err == nil {
				t.Fatal("multiple scope-changed receipts were selected implicitly")
			}
			failure := decodeReviewIntegrationFailure(t, output.Bytes())
			// organic-dx Phase 3c (maintainer scope extension): with reviews
			// enabled, blocking stays correct -- this candidate was never
			// reviewed -- but the caller is told the truth (nothing governs)
			// instead of being sent to disambiguate history. review.start is
			// the primary continuation; either prior lineage remains
			// available as optional recovery, named in Message, never
			// required.
			if failure.Code != "receipt_ambiguous" || failure.AuthorityApplicability != "not_evaluated" || failure.Context != nil || failure.RetrySafe ||
				failure.Replayability != reviewtransaction.ReplayabilityManualActionRequired || failure.NextAction != "review.start" ||
				!reflect.DeepEqual(failure.RequiredInputs, []string{}) {
				t.Fatalf("multiple scope-changed failure = %#v", failure)
			}
			for _, lineage := range lineages {
				if !strings.Contains(failure.Message, lineage) {
					t.Fatalf("multiple scope-changed failure message = %q, want it to name candidate %q", failure.Message, lineage)
				}
			}

			output.Reset()
			if err := RunReview([]string{
				"status", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--projection", string(tt.projection),
			}, &output); err != nil {
				t.Fatalf("target-scoped status: %v\n%s", err, output.String())
			}
			var status ReviewTargetStatusResult
			decodeStrictReviewJSON(t, output.Bytes(), &status)
			// organic-dx Phase 3e: this assertion previously required
			// Applicability=Ambiguous/Action=SelectLineage for `review
			// status` itself -- the exact stale-history-decides framing
			// Phase 3c's own completion notes named as a deliberately
			// untouched, structurally separate follow-up
			// (internal/reviewtransaction/target_status.go's own
			// classification, not discoverCompactFacadeGateReview's). Two
			// deterministically stale lineages must never force
			// disambiguation here either: nothing governs this candidate, so
			// `review status` now reports the same "start fresh" shape a
			// genuinely empty candidate set already reports, while still
			// listing both stale lineages as optional recovery candidates.
			if status.Applicability != reviewtransaction.TargetApplicabilityUnrelated || status.Action != reviewtransaction.TargetStatusActionStart ||
				status.Replayability != reviewtransaction.ReplayabilityNotReplayable || !reflect.DeepEqual(status.Candidates, lineages) {
				t.Fatalf("multiple scope-changed status = %#v", status)
			}
			if err := status.Validate(); err != nil {
				t.Fatalf("multiple scope-changed status failed contract validation: %v", err)
			}

			for _, lineage := range lineages {
				output.Reset()
				err := RunReview([]string{
					"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage, "--gate", string(tt.gate),
				}, &output)
				if err == nil {
					t.Fatalf("explicit scope-changed lineage %s unexpectedly passed", lineage)
				}
				explicit := decodeReviewIntegrationFailure(t, output.Bytes())
				if explicit.Code != "gate_scope_changed" || explicit.NextAction != "explicit-maintainer-action" {
					t.Fatalf("explicit scope-changed lineage %s failure = %#v", lineage, explicit)
				}
				assertScopeChangeRecovery(t, explicit, lineage, logicalPath)
			}
		})
	}
}

// TestUnqualifiedGateDiscoveryOnMixedCompactAndLegacyAuthorityHonorsTheKillSwitch
// covers the "same-pass candidate": a compact v2 receipt that exactly governs
// the live candidate, contested by an independent terminal legacy v1 chain that
// ALSO exactly governs it.
//
// This test previously asserted that the contest fails closed in BOTH modes
// (`TestUnqualifiedGateDiscoveryFailsClosedOnMixedCompactAndLegacyAuthority`),
// on the reasoning that reclassifying would mean silently picking one authority
// system over the other. That reasoning holds only while reviews are ON, where
// answering requires naming a governing receipt — and it still does. While
// reviews are OFF nothing is picked: the gate names neither store, invents no
// receipt, and defers to ordinary repository policy, so the contest has no
// delivery consequence until the operator turns reviews back on.
func TestUnqualifiedGateDiscoveryOnMixedCompactAndLegacyAuthorityHonorsTheKillSwitch(t *testing.T) {
	reviewEnabledHome(t)
	fixture := newLegacyCLIFixture(t, "review-discovery-mixed-legacy")
	// Reviews the identical, still-dirty candidate a second time under an
	// independent compact v2 lineage: nothing changed on disk between the two
	// reviews, so both exactly govern the live workspace target.
	finalizeFacadeReviewForRepo(t, fixture.repo)

	var enabled bytes.Buffer
	err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", fixture.repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &enabled)
	if err == nil {
		t.Fatal("mixed compact/legacy authority was silently resolved while enabled")
	}
	// Wave 5 Slice 2 supersession: this is now the sole home of the
	// competing-stores-named-in-the-message property that
	// review_disabled_reach_test.go's
	// TestReviewValidateReportsDisabledUnmanagedDeliveryOverMixedCompactAndLegacyAuthority
	// used to assert on its DISABLED half too (removed there -- design
	// decision 4 means that half no longer discovers the contest at all).
	// The negotiated envelope wraps the raw error behind a fixed catch-all
	// message with no Cause populated for this unclassified error type, so
	// the raw (non-negotiated) call is what still carries
	// errReviewMixedCompactLegacyAuthority's own text verbatim.
	var rawEnabled bytes.Buffer
	rawErr := RunReviewFacadeValidate([]string{"--cwd", fixture.repo, "--gate", string(reviewtransaction.GatePostApply)}, &rawEnabled)
	if rawErr == nil || !strings.Contains(rawErr.Error(), "compact v2 and legacy v1") {
		t.Fatalf("enabled mixed-authority error hid the competing stores: %v", rawErr)
	}

	disableReviewForClone(t, fixture.repo)

	var disabled bytes.Buffer
	err = RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", fixture.repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &disabled)
	if err != nil {
		t.Fatalf("mixed compact/legacy authority vetoed delivery while disabled: %v\n%s", err, disabled.String())
	}
	if !bytes.Contains(disabled.Bytes(), []byte(string(reviewtransaction.RDDDeliveryDisabledUnmanaged))) {
		t.Fatalf("mixed compact/legacy authority while disabled did not report the disposition: %s", disabled.String())
	}
	if bytes.Contains(disabled.Bytes(), []byte(`"allowed":true`)) {
		t.Fatalf("mixed compact/legacy authority while disabled fabricated an approval: %s", disabled.String())
	}
}

func TestUnscopedGateDiscoveryToleratesCorruptedUnrelatedLegacyInventory(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started, _ := approveDiscoveryMarkdown(t, repo, "review-discovery-valid", "docs/valid.md", "valid\n")
	commonDir := filepath.Clean(string(bytes.TrimSpace([]byte(runReviewCLIGit(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir")))))
	broken := filepath.Join(commonDir, "gentle-ai", "review-transactions", "v1", "unrelated-broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "HEAD"), []byte("not-a-revision\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var explicit bytes.Buffer
	statePath := filepath.Join(commonDir, "gentle-ai", "review-transactions", "v2", started.LineageID, "review-state.json")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		var current bytes.Buffer
		if err := RunReview([]string{
			"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID,
			"--gate", string(reviewtransaction.GatePostApply),
		}, &current); err != nil {
			t.Fatalf("explicit lineage was poisoned by unrelated inventory: %v\n%s", err, current.String())
		}
		if attempt == 0 {
			explicit = current
		} else if current.String() != explicit.String() {
			t.Fatalf("repeated explicit validation changed bytes:\n%s\n%s", explicit.String(), current.String())
		}
	}
	after, err := os.ReadFile(statePath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("explicit validation mutated authority: %v", err)
	}

	var unscoped bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &unscoped); err != nil {
		t.Fatalf("unscoped discovery was poisoned by corrupted unrelated legacy inventory: %v\n%s", err, unscoped.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, unscoped.Bytes()).Result, &result)
	if !result.Allowed || result.Context.LineageID != started.LineageID {
		t.Fatalf("unscoped discovery across corrupted legacy inventory = %#v", result)
	}
	if strings.Contains(unscoped.String(), broken) || strings.Contains(unscoped.String(), "not-a-revision") {
		t.Fatalf("unscoped discovery exposed private payload: %s", unscoped.String())
	}
	brokenHead, err := os.ReadFile(filepath.Join(broken, "HEAD"))
	if err != nil || string(brokenHead) != "not-a-revision\n" {
		t.Fatalf("unscoped discovery mutated corrupted legacy inventory: %v", err)
	}
}

func TestReleaseGateToleratesCorruptionConfinedToLegacyEntriesIncludingLockResidue(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started, _ := approveDiscoveryMarkdown(t, repo, "review-release-tolerance", "docs/valid.md", "valid\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "deliver reviewed candidate")

	evidenceDir := t.TempDir()
	releaseArgs := []string{}
	for _, artifact := range [][2]string{
		{"--release-configuration", "release configuration\n"},
		{"--release-generated", "generated manifest\n"},
		{"--release-provenance", "release provenance\n"},
		{"--release-publication-boundary", "sealed publication boundary\n"},
		{"--release-evidence-freshness", "current release evidence\n"},
	} {
		path := filepath.Join(evidenceDir, strings.TrimPrefix(artifact[0], "--"))
		if err := os.WriteFile(path, []byte(artifact[1]), 0o644); err != nil {
			t.Fatal(err)
		}
		releaseArgs = append(releaseArgs, artifact[0], path)
	}
	validateRelease := func() (*bytes.Buffer, error) {
		var output bytes.Buffer
		err := RunReview(append([]string{
			"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
			"--gate", string(reviewtransaction.GateRelease),
		}, releaseArgs...), &output)
		return &output, err
	}

	control, err := validateRelease()
	if err != nil {
		t.Fatalf("healthy release-gate control denied: %v\n%s", err, control.String())
	}

	commonDir := filepath.Clean(strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir")))
	legacyBroken := filepath.Join(commonDir, "gentle-ai", "review-transactions", "v1", "legacy-alias-broken")
	if err := os.MkdirAll(legacyBroken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyBroken, "HEAD"), []byte("not-a-revision\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyBroken, "LOCK"), []byte("not-json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := reviewtransaction.InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Complete || report.Authoritative {
		t.Fatalf("legacy lock residue fixture left the inventory authoritative = %#v", report)
	}
	invalidLegacy, ambiguousLegacyLock := false, false
	for _, entry := range report.Entries {
		if entry.LineageID == "legacy-alias-broken" && entry.Version == reviewtransaction.AuthorityVersionLegacy && entry.Status == reviewtransaction.AuthorityStatusInvalid {
			invalidLegacy = true
		}
	}
	for _, lock := range report.Locks {
		if lock.LineageID == "legacy-alias-broken" && lock.Version == reviewtransaction.AuthorityVersionLegacy && lock.Status == reviewtransaction.AuthorityLockAmbiguous {
			ambiguousLegacyLock = true
		}
	}
	if !invalidLegacy || !ambiguousLegacyLock {
		t.Fatalf("fixture did not confine ambiguous lock residue to an invalid legacy entry: entries=%#v locks=%#v", report.Entries, report.Locks)
	}

	tolerated, err := validateRelease()
	if err != nil {
		t.Fatalf("release gate denied corruption confined to legacy entries: %v\n%s", err, tolerated.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, tolerated.Bytes()).Result, &result)
	if !result.Allowed || result.Context.LineageID != started.LineageID {
		t.Fatalf("release gate across legacy-confined corruption = %#v", result)
	}
	incompleteCompact := filepath.Join(commonDir, "gentle-ai", "review-transactions", "v2", "compact-incomplete")
	if err := os.MkdirAll(incompleteCompact, 0o755); err != nil {
		t.Fatal(err)
	}
	incompleteDenied, err := validateRelease()
	if err == nil {
		t.Fatalf("legacy lock masked an independent incomplete compact-v2 entry:\n%s", incompleteDenied.String())
	}
	if err := os.Remove(incompleteCompact); err != nil {
		t.Fatal(err)
	}
	brokenHead, err := os.ReadFile(filepath.Join(legacyBroken, "HEAD"))
	if err != nil || string(brokenHead) != "not-a-revision\n" {
		t.Fatalf("release validation mutated legacy residue head: %v", err)
	}
	brokenLock, err := os.ReadFile(filepath.Join(legacyBroken, "LOCK"))
	if err != nil || string(brokenLock) != "not-json\n" {
		t.Fatalf("release validation mutated legacy lock residue: %v", err)
	}

	sharedLock := filepath.Join(commonDir, "gentle-ai", "review-transactions", "v2", "LOCK")
	originalSharedLock, err := os.ReadFile(sharedLock)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sharedLock, []byte("not-json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sharedDenied, err := validateRelease()
	if err == nil {
		t.Fatalf("shared compact-v2 ambiguous lock was tolerated at the release gate:\n%s", sharedDenied.String())
	}
	sharedFailure := decodeReviewIntegrationFailure(t, sharedDenied.Bytes())
	if sharedFailure.Code != "authority_corrupted" || sharedFailure.AuthorityApplicability != "corrupted" || sharedFailure.CauseCategory != "lock_ambiguous" {
		t.Fatalf("shared ambiguous lock failure = %#v", sharedFailure)
	}
	if err := os.WriteFile(sharedLock, originalSharedLock, 0o644); err != nil {
		t.Fatal(err)
	}

	compactBroken := filepath.Join(commonDir, "gentle-ai", "review-transactions", "v2", "compact-broken")
	if err := os.MkdirAll(compactBroken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(compactBroken, "review-state.json"), []byte("{\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	compactDenied, err := validateRelease()
	if err == nil {
		t.Fatalf("live compact corruption was tolerated at the release gate:\n%s", compactDenied.String())
	}
	compactFailure := decodeReviewIntegrationFailure(t, compactDenied.Bytes())
	if compactFailure.Code != "authority_corrupted" || compactFailure.AuthorityApplicability != "corrupted" || compactFailure.CauseCategory != "record_or_graph_invalid" {
		t.Fatalf("live compact corruption failure = %#v", compactFailure)
	}
}

func TestUnscopedGateDiscoveryExcludesTamperedLegacyReceiptFromCandidates(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started, _ := approveDiscoveryMarkdown(t, repo, "review-discovery-valid", "docs/valid.md", "valid\n")
	legacyStore, authoritative := approveLegacyDiscoveryChain(t, repo, "review-legacy-tampered")
	tampered := authoritative
	tampered.EvidenceHash = "sha256:" + strings.Repeat("ab", 32)
	if tampered.EvidenceHash == authoritative.EvidenceHash {
		tampered.EvidenceHash = "sha256:" + strings.Repeat("cd", 32)
	}
	receiptPath := filepath.Join(legacyStore.Dir, "artifacts", "receipt.json")
	if err := reviewtransaction.WriteReceiptAtomic(receiptPath, tampered); err != nil {
		t.Fatal(err)
	}

	report, err := reviewtransaction.InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	mismatch := false
	for _, entry := range report.Entries {
		if entry.LineageID == "review-legacy-tampered" && entry.Version == reviewtransaction.AuthorityVersionLegacy &&
			entry.Status == reviewtransaction.AuthorityStatusInvalid &&
			reflect.DeepEqual(entry.Problems, []string{"legacy receipt does not match terminal authority"}) {
			mismatch = true
		}
	}
	if !mismatch {
		t.Fatalf("fixture did not produce a receipt-mismatch legacy entry: %#v", report.Entries)
	}

	gateInput := reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePostApply}
	if exact := legacyExactFacadeGateLineages(context.Background(), repo, gateInput); exact != 0 {
		t.Fatalf("tampered legacy receipt counted as exact gate candidate: %d", exact)
	}

	var unscoped bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &unscoped); err != nil {
		t.Fatalf("unscoped discovery was poisoned by tampered legacy receipt: %v\n%s", err, unscoped.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, unscoped.Bytes()).Result, &result)
	if !result.Allowed || result.Context.LineageID != started.LineageID {
		t.Fatalf("unscoped discovery across tampered legacy receipt = %#v", result)
	}
}

func approveLegacyDiscoveryChain(t *testing.T, repo, lineage string) (reviewtransaction.Store, reviewtransaction.Receipt) {
	t.Helper()
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.md")
	ledgerPath := filepath.Join(dir, "ledger.json")
	evidencePath := filepath.Join(dir, "evidence.txt")
	if err := os.WriteFile(policyPath, []byte("legacy bounded policy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger, err := reviewtransaction.CanonicalLedger([]reviewtransaction.Finding{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ledgerPath, ledger, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(evidencePath, []byte("legacy verification passed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	policyHash, _ := reviewtransaction.HashArtifact(policyPath)
	ledgerHash, _ := reviewtransaction.HashLedgerArtifact(ledgerPath)
	evidenceHash, _ := reviewtransaction.HashArtifact(evidencePath)
	tx, err := reviewtransaction.NewTransaction(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinary4R, Generation: 1,
		Snapshot: snapshot, PolicyHash: policyHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.AuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	head := appendLegacyCLIRecord(t, store, "", "review/start", *tx)
	if err := tx.FreezeFindings([]reviewtransaction.Finding{}, ledger, ledgerHash); err != nil {
		t.Fatal(err)
	}
	head = appendLegacyCLIRecord(t, store, head, "review/freeze-findings", *tx)
	if _, err := tx.ClassifyEvidence([]reviewtransaction.FindingEvidence{}); err != nil {
		t.Fatal(err)
	}
	head = appendLegacyCLIRecord(t, store, head, "review/classify-evidence", *tx)
	if err := tx.BeginFinalVerification(); err != nil {
		t.Fatal(err)
	}
	head = appendLegacyCLIRecord(t, store, head, "review/begin-final-verification", *tx)
	if err := tx.CompleteFinalVerification(evidenceHash, true); err != nil {
		t.Fatal(err)
	}
	appendLegacyCLIRecord(t, store, head, "review/complete-final-verification", *tx)
	receipt, err := tx.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.WriteReceiptAtomic(filepath.Join(store.Dir, "artifacts", "receipt.json"), receipt); err != nil {
		t.Fatal(err)
	}
	return store, receipt
}

// TestUnscopedGateDiscoveryFailsClosedOnCorruptedCompactLeaf is the shape
// #2167 reported: a healthy approved lineage and one unrelated corrupt leaf in
// the same shared store, and the healthy candidate denied `authority_corrupted`
// with no way to tell which entry caused it. The corrupt leaf still fails
// closed -- when something names it.
func TestUnscopedGateDiscoveryFailsClosedOnCorruptedCompactLeaf(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	approveDiscoveryMarkdown(t, repo, "review-discovery-valid", "docs/valid.md", "valid\n")
	commonDir := filepath.Clean(string(bytes.TrimSpace([]byte(runReviewCLIGit(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir")))))
	broken := filepath.Join(commonDir, "gentle-ai", "review-transactions", "v2", "unrelated-broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "review-state.json"), []byte("{\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var unscoped bytes.Buffer
	unscopedErr := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &unscoped)
	if unscopedErr != nil {
		failure := decodeReviewIntegrationFailure(t, unscoped.Bytes())
		if failure.Code == "authority_corrupted" || failure.AuthorityApplicability == "corrupted" {
			t.Fatalf("an unrelated corrupted leaf became this candidate's verdict: %#v", failure)
		}
	}

	var scoped bytes.Buffer
	err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply), "--lineage", "unrelated-broken",
	}, &scoped)
	if err == nil {
		t.Fatal("naming the corrupted compact leaf validated")
	}
	// The named refusal must be closed, must name the entry it refuses, and
	// must not leak the store path. Its exact code is the explicit-selector
	// path's own classification and is not asserted here: what this test owns
	// is that the leaf refuses for itself and for nobody else.
	if !strings.Contains(scoped.String(), "unrelated-broken") {
		t.Fatalf("the scoped refusal does not name the leaf it refuses:\n%s", scoped.String())
	}
	if strings.Contains(scoped.String(), broken) {
		t.Fatalf("corrupted compact leaf failure exposed private payload: %s", scoped.String())
	}
}

func TestExplicitMalformedLineageFailsClosedWithoutMutation(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started, store := approveDiscoveryMarkdown(t, repo, "review-selected-malformed", "docs/selected.md", "selected\n")
	malformed := []byte("{\n")
	if err := os.WriteFile(store.StatePath(), malformed, 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err := RunReview([]string{"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID, "--gate", string(reviewtransaction.GatePostApply)}, &output)
	if err == nil {
		t.Fatal("selected malformed lineage validated")
	}
	after, readErr := os.ReadFile(store.StatePath())
	if readErr != nil || !bytes.Equal(after, malformed) || strings.Contains(output.String(), store.StatePath()) || strings.Contains(output.String(), "unexpected end") {
		t.Fatalf("selected malformed lineage was exposed or mutated: %v\n%s", readErr, output.String())
	}
}

// TestUnqualifiedPrePRDiscoveryDeniesSequentialCompactReceiptsWithoutComposition
// supersedes TestUnqualifiedPrePRDiscoveryComposesExactSequentialCompactReceipts
// (pre-Wave-5-Slice-5 name; S1's characterization corpus pinned the identical
// fixture's "before" allow at the package level via
// TestPrePRChainCompositionRemovalDelta, compact_chain_test.go, now itself
// superseded by TestPrePRChainCompositionDeletionSupersedesRemovalDelta,
// internal/reviewtransaction). Before this slice, three sequentially
// delivered, individually-approved lineages composed into one allow for the
// full delivered range. EvaluateCompactPrePRChain is deleted
// (TestPrePRComposition_ZeroCallers proves it by call-absence), so the
// lineage-free discovery this test drives can no longer resolve to a single
// governing receipt: three terminal candidates each cover only their own
// segment, none spans the whole delivered range, so discovery reports
// receipt_ambiguous with review.start as the runnable next action -- a
// named divergence, not a silent one.
func TestUnqualifiedPrePRDiscoveryDeniesSequentialCompactReceiptsWithoutComposition(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	remote := filepath.Join(t.TempDir(), "remote.git")
	runReviewCLIGit(t, repo, "clone", "--bare", repo, remote)
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)

	lineages := []string{"review-chain-first", "review-chain-second", "review-chain-third"}
	for index, lineage := range lineages {
		approveDiscoveryMarkdown(t, repo, lineage, "docs/segment-"+string(rune('a'+index))+".md", "reviewed\n")
		runReviewCLIGit(t, repo, "add", "-A")
		runReviewCLIGit(t, repo, "commit", "-qm", "deliver "+lineage)
	}

	var output bytes.Buffer
	err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePrePR), "--base-ref", "origin/" + branch,
	}, &output)
	if err == nil {
		t.Fatalf("lineage-free pre-PR discovery unexpectedly allowed without composition:\n%s", output.String())
	}
	var failure struct {
		Code       string `json:"code"`
		NextAction string `json:"next_action"`
	}
	if err := json.Unmarshal(output.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Code != "receipt_ambiguous" || failure.NextAction != "review.start" {
		t.Fatalf("unqualified pre-PR discovery denial = %#v\n%s", failure, output.String())
	}

	output.Reset()
	err = RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineages[2],
		"--gate", string(reviewtransaction.GatePrePR), "--base-ref", "origin/" + branch,
	}, &output)
	if err == nil {
		t.Fatal("explicit terminal lineage unexpectedly allowed a receipt that only covers its own segment")
	}
}

// TestUnqualifiedPrePRDiscoveryDeniesSequentialReceiptsForSamePathWithoutComposition
// supersedes TestUnqualifiedPrePRDiscoveryComposesSequentialReceiptsForSamePath.
// Three lineages reviewing the SAME path in sequence used to compose to an
// allow for the last lineage; auto-discovery here resolves a single
// candidate (the same path narrows the ambiguity the sibling test above
// hits), but that candidate's own receipt only covers its own segment, so
// EvaluateCompactGate denies it directly -- gate_invalidated, not
// receipt_ambiguous, since discovery itself was unambiguous.
func TestUnqualifiedPrePRDiscoveryDeniesSequentialReceiptsForSamePathWithoutComposition(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	remote := filepath.Join(t.TempDir(), "remote.git")
	runReviewCLIGit(t, repo, "clone", "--bare", repo, remote)
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)

	for index, lineage := range []string{"review-chain-overlap-first", "review-chain-overlap-second", "review-chain-overlap-third"} {
		approveDiscoveryMarkdown(t, repo, lineage, "docs/shared.md", "reviewed "+strings.Repeat(string(rune('a'+index)), index+1)+"\n")
		runReviewCLIGit(t, repo, "add", "-A")
		runReviewCLIGit(t, repo, "commit", "-qm", "deliver "+lineage)
	}

	var output bytes.Buffer
	err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePrePR), "--base-ref", "origin/" + branch,
	}, &output)
	if err == nil {
		t.Fatalf("same-path lineage-free pre-PR discovery unexpectedly allowed without composition:\n%s", output.String())
	}
	var failure struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(output.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Code != "gate_invalidated" {
		t.Fatalf("same-path unqualified pre-PR discovery denial code = %q, want gate_invalidated\n%s", failure.Code, output.String())
	}
}

func TestUnqualifiedPrePRDiscoveryReconcilesCurrentChangesReceiptAcrossDivergedBase(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	initialCommit := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	remote := filepath.Join(t.TempDir(), "remote.git")
	runReviewCLIGit(t, repo, "clone", "--bare", repo, remote)
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)
	started, _ := approveDiscoveryMarkdown(t, repo, "review-current-diverged-base", "docs/single.md", "reviewed\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "deliver reviewed change")
	runReviewCLIGit(t, repo, "checkout", "-qb", "base-advance", initialCommit)
	if err := os.WriteFile(filepath.Join(repo, "base-only.txt"), []byte("base advance\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "base-only.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "advance publication base")
	runReviewCLIGit(t, repo, "push", "-q", "origin", "base-advance:refs/heads/"+branch)
	runReviewCLIGit(t, repo, "checkout", "-q", branch)

	var output bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePrePR), "--base-ref", "origin/" + branch,
	}, &output); err != nil {
		t.Fatalf("diverged-base current-changes pre-PR validation: %v\n%s", err, output.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, output.Bytes()).Result, &result)
	if !result.Allowed || result.Context.LineageID != started.LineageID || result.Context.BaseAdvance == nil ||
		!result.Context.BaseAdvance.Compatible || result.Context.BaseAdvance.Status != "current-changes-boundary-compatible" {
		t.Fatalf("diverged-base current-changes pre-PR result = %#v", result)
	}
}

func TestUnqualifiedPrePRDiscoveryKeepsExactSingleReceiptContext(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	remote := filepath.Join(t.TempDir(), "remote.git")
	runReviewCLIGit(t, repo, "clone", "--bare", repo, remote)
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)
	started, store := approveDiscoveryMarkdown(t, repo, "review-single-pre-pr", "docs/single.md", "reviewed\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "deliver exact single receipt")
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePrePR), "--base-ref", "origin/" + branch,
	}, &output); err != nil {
		t.Fatalf("exact single pre-PR validation: %v\n%s", err, output.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, output.Bytes()).Result, &result)
	if !result.Allowed || result.Context.LineageID != started.LineageID || result.Context.StoreRevision != record.Revision || result.Context.ChainIdentity != record.Revision {
		t.Fatalf("exact single receipt context changed = %#v", result.Context)
	}
}

func approveDiscoveryMarkdown(t *testing.T, repo, lineage, logicalPath, content string) (ReviewFacadeStartResult, reviewtransaction.CompactStore) {
	return approveDiscoveryMarkdownProjection(t, repo, lineage, logicalPath, content, reviewtransaction.ProjectionWorkspace)
}

// TestNegotiatedStatusRequiresExactStagedDeliveryCandidate proves #2758 at the
// negotiated CLI boundary. A workspace receipt remains applicable to the
// workspace while the index is empty, partial, or different, so STATUS must
// inspect the pre-commit candidate before it offers validation.
func TestNegotiatedStatusRequiresExactStagedDeliveryCandidate(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	const lineage = "review-staged-delivery-candidate"
	for path, content := range map[string]string{"docs/alpha.md": "baseline alpha\n", "docs/bravo.md": "baseline bravo\n"} {
		fullPath := filepath.Join(repo, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runReviewCLIGit(t, repo, "add", "docs/alpha.md", "docs/bravo.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "fixture: add tracked delivery paths")
	files := map[string]string{
		"docs/alpha.md": "# Alpha\n",
		"docs/bravo.md": "# Bravo\n",
	}
	_, store := approveDiscoveryMarkdownFilesProjection(t, repo, lineage, files, reviewtransaction.ProjectionWorkspace)
	stateBefore, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	receiptBefore, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}

	status := func(selectors ...string) ReviewTargetStatusResult {
		t.Helper()
		args := []string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition", "--lineage", lineage, "--gate", "pre-commit"}
		args = append(args, selectors...)
		var output bytes.Buffer
		if err := RunReview(args, &output); err != nil {
			t.Fatalf("STATUS %v: %v\n%s", selectors, err, output.String())
		}
		var result ReviewTargetStatusResult
		decodeStrictReviewJSON(t, output.Bytes(), &result)
		return result
	}
	requireBlocked := func(name string) {
		t.Helper()
		result := status()
		if result.NextTransition == nil || result.NextTransition.Kind != reviewNextTransitionStop ||
			result.NextTransition.ReasonCode != "staged_delivery_candidate_required" ||
			result.NextTransition.Execute != nil || result.NextTransition.Collect != nil {
			t.Fatalf("%s staged delivery transition = %#v", name, result.NextTransition)
		}
		stateAfter, stateErr := os.ReadFile(store.StatePath())
		receiptAfter, receiptErr := os.ReadFile(store.ReceiptPath())
		if stateErr != nil || receiptErr != nil || !bytes.Equal(stateBefore, stateAfter) || !bytes.Equal(receiptBefore, receiptAfter) {
			t.Fatalf("%s STATUS mutated authority: state=%v receipt=%v", name, stateErr, receiptErr)
		}
	}

	runReviewCLIGit(t, repo, "reset", "--", "docs/alpha.md", "docs/bravo.md")
	requireBlocked("empty index")

	runReviewCLIGit(t, repo, "add", "docs/alpha.md")
	requireBlocked("partial index")

	runReviewCLIGit(t, repo, "add", "docs/bravo.md")
	if err := os.WriteFile(filepath.Join(repo, "docs", "alpha.md"), []byte("# Different\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "docs/alpha.md")
	if err := os.WriteFile(filepath.Join(repo, "docs", "alpha.md"), []byte(files["docs/alpha.md"]), 0o644); err != nil {
		t.Fatal(err)
	}
	requireBlocked("different index")

	runReviewCLIGit(t, repo, "add", "docs/alpha.md", "docs/bravo.md")
	ready := status("--projection", "staged")
	if ready.NextTransition == nil || ready.NextTransition.Kind != reviewNextTransitionExecute ||
		ready.NextTransition.Execute == nil || ready.NextTransition.Execute.Operation != "review.validate" ||
		ready.NextTransition.ReasonCode != "approved_receipt_ready" {
		t.Fatalf("exact staged delivery transition = %#v", ready.NextTransition)
	}
	payload, err := runSelectorTransition(repo, ready)
	if err != nil {
		t.Fatalf("execute exact staged validation: %v\n%s", err, payload)
	}
	var validation ReviewValidateResult
	decodeStrictReviewJSON(t, payload, &validation)
	if !validation.Allowed || validation.Result != reviewtransaction.GateAllow {
		t.Fatalf("exact staged validation = %#v", validation)
	}
}

func TestNegotiatedStatusRequiresStagingApprovedUnbornIntendedCandidate(t *testing.T) {
	reviewEnabledHome(t)
	repo := initUnbornReviewCLIRepo(t)
	const (
		lineage = "review-unborn-intended-staged-delivery"
		path    = "docs/candidate.md"
	)
	writeUndeclaredWorkspaceFile(t, repo, path, "# Reviewed candidate\n", 0o644)
	digest, inventory := intendedUntrackedSelection(t, intendedUntrackedStatus(t, repo))
	if !reflect.DeepEqual(inventory, []string{path}) {
		t.Fatalf("unborn inventory = %v, want %q", inventory, path)
	}
	_, store := approveDiscoveryMarkdownFilesWithIntendedProjection(t, repo, lineage,
		map[string]string{path: "# Reviewed candidate\n"}, reviewtransaction.ProjectionWorkspace, []string{path})
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.State.State != reviewtransaction.StateApproved || !record.State.InitialSnapshot.UnbornHead ||
		!reflect.DeepEqual(record.State.CurrentSnapshot.IntendedUntracked, []string{path}) {
		t.Fatalf("approved unborn intended authority = %#v", record.State)
	}
	if err := exec.Command("git", "-C", repo, "rev-parse", "--verify", "HEAD").Run(); err == nil {
		t.Fatal("fixture unexpectedly has a born HEAD")
	}
	if cached := strings.TrimSpace(runReviewCLIGit(t, repo, "ls-files", "--cached")); cached != "" {
		t.Fatalf("real index is not empty: %q", cached)
	}
	stateBefore, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	receiptBefore, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	indexPath := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "--git-path", "index"))
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(repo, indexPath)
	}
	indexBefore, indexBeforeErr := os.ReadFile(indexPath)

	status := func(selectors ...string) ReviewTargetStatusResult {
		t.Helper()
		args := []string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition", "--lineage", lineage, "--gate", "pre-commit"}
		args = append(args, selectors...)
		var output bytes.Buffer
		if err := RunReview(args, &output); err != nil {
			t.Fatalf("STATUS %v: %v\n%s", selectors, err, output.String())
		}
		var result ReviewTargetStatusResult
		decodeStrictReviewJSON(t, output.Bytes(), &result)
		return result
	}

	blocked := status(intendedUntrackedSelectArgs(digest, path)...)
	if blocked.NextTransition == nil || blocked.NextTransition.Kind != reviewNextTransitionStop ||
		blocked.NextTransition.ReasonCode != "staged_delivery_candidate_required" || blocked.NextTransition.Execute != nil {
		t.Fatalf("empty unborn staged delivery transition = %#v", blocked.NextTransition)
	}
	stateAfter, stateErr := os.ReadFile(store.StatePath())
	receiptAfter, receiptErr := os.ReadFile(store.ReceiptPath())
	indexAfter, indexAfterErr := os.ReadFile(indexPath)
	indexChanged := false
	switch {
	case indexBeforeErr == nil && indexAfterErr == nil:
		indexChanged = !bytes.Equal(indexBefore, indexAfter)
	case errors.Is(indexBeforeErr, os.ErrNotExist) && errors.Is(indexAfterErr, os.ErrNotExist):
	default:
		indexChanged = true
	}
	if stateErr != nil || receiptErr != nil || !bytes.Equal(stateBefore, stateAfter) || !bytes.Equal(receiptBefore, receiptAfter) ||
		indexChanged {
		t.Fatalf("STATUS mutated authority, receipt, or index: state=%v receipt=%v index_before=%v index_after=%v", stateErr, receiptErr, indexBeforeErr, indexAfterErr)
	}

	runReviewCLIGit(t, repo, "add", "--", path)
	ready := status("--projection", "staged")
	if ready.NextTransition == nil || ready.NextTransition.Kind != reviewNextTransitionExecute || ready.NextTransition.Execute == nil ||
		ready.NextTransition.Execute.Operation != "review.validate" || ready.NextTransition.ReasonCode != "approved_receipt_ready" {
		t.Fatalf("exact unborn staged delivery transition = %#v", ready.NextTransition)
	}
	payload, err := runSelectorTransition(repo, ready)
	if err != nil {
		t.Fatalf("execute exact unborn staged validation: %v\n%s", err, payload)
	}
	var validation ReviewValidateResult
	decodeStrictReviewJSON(t, payload, &validation)
	if !validation.Allowed || validation.Result != reviewtransaction.GateAllow {
		t.Fatalf("exact unborn staged validation = %#v", validation)
	}
}

// approveDiscoveryMarkdownProjection builds and finalizes a LOW-risk legacy
// (compact-v2) approved receipt over one passive markdown candidate.
//
// Before Wave 7 S7 (WU18), this called the CLI's own `review start` with the
// (default, unset) activation switch off, which took the legacy compact-v2
// branch. WU18 removed that branch entirely -- `review start` is now
// unconditionally v3, so there is no CLI-reachable way left to create a NEW
// legacy authority. Every one of this helper's ~44 call sites across 9 files
// specifically needs genuine compact-v2 authority (proven by their own
// reviewtransaction.CompactAuthoritativeStore/CompactStore-typed return
// value and downstream use), so this constructs it directly through the
// same reviewtransaction API runReviewFacadeStart's now-deleted legacy
// branch used to call -- identical to finalizeApprovedFacadeReview's own
// fix -- then finalizes through the unchanged CLI RunReviewFacadeFinalize
// (finalize's discovery-by-kind logic never depended on the switch).
func approveDiscoveryMarkdownProjection(t *testing.T, repo, lineage, logicalPath, content string, projection reviewtransaction.Projection) (ReviewFacadeStartResult, reviewtransaction.CompactStore) {
	return approveDiscoveryMarkdownFilesProjection(t, repo, lineage, map[string]string{logicalPath: content}, projection)
}

func approveDiscoveryMarkdownFilesProjection(t *testing.T, repo, lineage string, files map[string]string, projection reviewtransaction.Projection) (ReviewFacadeStartResult, reviewtransaction.CompactStore) {
	return approveDiscoveryMarkdownFilesWithIntendedProjection(t, repo, lineage, files, projection, []string{})
}

func approveDiscoveryMarkdownFilesWithIntendedProjection(t *testing.T, repo, lineage string, files map[string]string, projection reviewtransaction.Projection, intended []string) (ReviewFacadeStartResult, reviewtransaction.CompactStore) {
	t.Helper()
	for logicalPath, content := range files {
		path := filepath.Join(repo, filepath.FromSlash(logicalPath))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// #2394: a new file is reviewable only once the user declared it. The index
	// declares ordinary paths; intended-untracked paths carry explicit intent.
	if len(intended) == 0 {
		runReviewCLIGit(t, repo, "add", "-A")
	}

	ctx := context.Background()
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	root, err := builder.ResolveRepositoryRoot(ctx)
	if err != nil {
		t.Fatalf("resolve discovery fixture repository root: %v", err)
	}
	rootBuilder := reviewtransaction.SnapshotBuilder{Repo: root}
	snapshot, err := rootBuilder.Build(ctx, reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, Projection: projection, IntendedUntracked: append([]string{}, intended...)})
	if err != nil {
		t.Fatalf("build discovery fixture target %q: %v", lineage, err)
	}
	assessment, err := rootBuilder.AssessSnapshotRisk(ctx, snapshot)
	if err != nil {
		t.Fatalf("classify discovery fixture target %q: %v", lineage, err)
	}
	if assessment.Level != reviewtransaction.RiskLow {
		t.Fatalf("discovery fixture risk = %q", assessment.Level)
	}
	lenses, err := facadeSelectedLenses(assessment, "reliability")
	if err != nil {
		t.Fatalf("select discovery fixture lenses %q: %v", lineage, err)
	}
	policy, err := facadePolicyBytes("")
	if err != nil {
		t.Fatalf("read discovery fixture policy %q: %v", lineage, err)
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: facadePayloadHash(policy), RiskLevel: assessment.Level,
		SelectedLenses: lenses, OriginalChangedLines: &assessment.ChangedLines,
	})
	if err != nil {
		t.Fatalf("create discovery fixture state %q: %v", lineage, err)
	}
	compactStarted, err := reviewtransaction.StartCompactAuthority(ctx, root, reviewtransaction.CompactStartRequest{
		State: state, ExplicitLineage: true,
	})
	if err != nil {
		t.Fatalf("start discovery fixture compact authority %q: %v", lineage, err)
	}
	started := reviewFacadeStartResultFor(compactStarted.Action, compactStarted.LensesRequired, compactStarted.Record.State)
	if started.RiskLevel != reviewtransaction.RiskLow {
		t.Fatalf("discovery fixture risk = %q", started.RiskLevel)
	}

	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", lineage}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	return started, store
}

func cloneApprovedDiscoveryAuthority(t *testing.T, repo string, source reviewtransaction.CompactStore, lineage string) reviewtransaction.CompactStore {
	t.Helper()
	record, err := source.Load()
	if err != nil {
		t.Fatal(err)
	}
	lines := record.State.OriginalChangedLines
	clone, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: record.State.Generation,
		Snapshot: record.State.InitialSnapshot, PolicyHash: record.State.PolicyHash, RiskLevel: record.State.RiskLevel,
		SelectedLenses: append([]string{}, record.State.SelectedLenses...), OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	cloneStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, clone.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := cloneStore.Replace("", "review/start", clone)
	if err != nil {
		t.Fatal(err)
	}
	if err := clone.CompleteReview(reviewtransaction.CompactReviewInput{LensResults: []reviewtransaction.LensResult{}}); err != nil {
		t.Fatal(err)
	}
	revision, err = cloneStore.Replace(revision, "review/complete-review", clone)
	if err != nil {
		t.Fatal(err)
	}
	if err := clone.CompleteVerification([]byte("independent duplicate fixture evidence"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := cloneStore.Replace(revision, "review/complete-verification", clone); err != nil {
		t.Fatal(err)
	}
	receipt, err := clone.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.WriteCompactReceiptAtomic(cloneStore.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	return cloneStore
}
