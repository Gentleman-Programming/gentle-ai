package cli

// Issue #1877: the disable -> deliver -> re-enable -> archive sequence must
// terminate in one fresh full review of the current state, runnably named at
// every stop. The maintainer's rule, applied to delivery: no retroactive
// reconciliation, no blessing of past unmanaged deliveries, no durable
// per-delivery ledger. Work delivered while the switch was off is recorded as
// unmanaged; re-enabling re-validates the current state through the entire
// RDD review as if it had never been done; that fresh review subsumes the
// unmanaged history.
//
// These tests drive the sequence through the real command surface and then run
// exactly what the product's own messages name, requiring each stop to clear.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// dispatchNamedReviewStart runs a `gentle-ai review start ...` invocation read
// out of a product message through the real review router. extra carries only
// the operator-supplied value a message placeholder explicitly asks for.
func dispatchNamedReviewStart(t *testing.T, repo string, tokens []string, extra ...string) ReviewFacadeStartResult {
	t.Helper()
	if len(tokens) < 2 || tokens[0] != "review" || tokens[1] != "start" {
		t.Fatalf("named continuation is %v, want gentle-ai review start", tokens)
	}
	args := append(append([]string{}, tokens[1:]...), extra...)
	args = append(args, "--cwd", repo)
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("the named continuation exits non-zero: gentle-ai review %v: %v\n%s", args, err, output.String())
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	return started
}

func commitAllSDDStatus(t *testing.T, repo, message string) string {
	t.Helper()
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", message)
	return strings.TrimSpace(runReviewCLIGitOutput(t, repo, "rev-parse", "HEAD"))
}

func finalizeFacadeLineage(t *testing.T, repo, lineage string) {
	t.Helper()
	var output bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", lineage}, &output); err != nil {
		t.Fatalf("finalize %q: %v\n%s", lineage, err, output.String())
	}
}

// TestSDDStatusReEnableSequenceLandsOnTheFreshFullReview drives the maintainer's
// sequence end to end: reviewed baseline, kill switch off, two unmanaged
// deliveries recorded as unmanaged, switch back on, and then only what the
// product's messages name until the archive stop clears through the fresh
// full review of the current state.
func TestSDDStatusReEnableSequenceLandsOnTheFreshFullReview(t *testing.T) {
	reviewModeHome(t)
	root := t.TempDir()
	seedArchiveGatedSDDChange(t, root)

	// Baseline: reviews on, work reviewed and delivered normally.
	writeSDDStatusFile(t, root+"/docs/baseline.md", "# baseline\n\nplain prose, no executable content.\n")
	runReviewCLIGit(t, root, "add", "-A")
	baselineStarted := startFacadeReviewResult(t, root, "reenable-baseline")
	finalizeFacadeLineage(t, root, baselineStarted.LineageID)
	baselineCommit := commitAllSDDStatus(t, root, "baseline reviewed delivery")

	// The kill switch goes off, and two units are delivered under it.
	disableReviewForClone(t, root)
	writeSDDStatusFile(t, root+"/docs/unmanaged-one.md", "# one\n\nplain prose, delivered while disabled.\n")
	commitAllSDDStatus(t, root, "unmanaged delivery one")
	writeSDDStatusFile(t, root+"/docs/unmanaged-two.md", "# two\n\nplain prose, delivered while disabled.\n")
	commitAllSDDStatus(t, root, "unmanaged delivery two")

	// The disabled window records the change as unmanaged: the stale baseline
	// receipt no longer governs anything, and declining to manage is not a
	// blocker demanding a review the switch refuses to run.
	disabled := resolveSDDStatusJSON(t, root)
	if disabled.Dependencies.Archive == sddstatus.DependencyBlocked {
		t.Fatalf("disabled archive over a stale receipt = blocked; reasons = %v", disabled.BlockedReasons)
	}
	if disabled.ReviewGate == nil || disabled.ReviewGate.Delivery != reviewtransaction.RDDDeliveryDisabledUnmanaged {
		t.Fatalf("disabled window did not record the unmanaged disposition: %#v", disabled.ReviewGate)
	}
	if disabled.ReviewGate.Result == reviewtransaction.GateAllow {
		t.Fatalf("disabled window fabricated an approval: %#v", disabled.ReviewGate)
	}

	// Re-enable. The archive stop must come back, and it must name the fresh
	// full review runnably instead of demanding an unnameable lineage.
	enableReviewForClone(t, root)
	blocked := resolveSDDStatusJSON(t, root)
	if blocked.Dependencies.Archive != sddstatus.DependencyBlocked || blocked.ReviewGate == nil {
		t.Fatalf("re-enabled archive over unmanaged history = %#v, want blocked", blocked.Dependencies)
	}
	if blocked.ReviewGate.Result == reviewtransaction.GateAllow {
		t.Fatalf("re-enabling silently passed over unmanaged history: %#v", blocked.ReviewGate)
	}
	tokens := namedReviewCommandTokens(t, blocked.ReviewGate.Reason)

	// Run exactly what the stop names. The tree is clean, so this start
	// freezes an empty candidate and its hint names the --base-ref rerun.
	emptyStart := dispatchNamedReviewStart(t, root, tokens)
	if emptyStart.ChangedFiles != 0 {
		t.Fatalf("clean-tree start froze %d files, fixture is wrong", emptyStart.ChangedFiles)
	}
	if !strings.Contains(emptyStart.Hint, "--base-ref") {
		t.Fatalf("empty start hint does not name the committed-work rerun: %q", emptyStart.Hint)
	}
	finalizeFacadeLineage(t, root, emptyStart.LineageID)

	// The approved empty receipt reviewed nothing, so it must not read as
	// coverage of the unmanaged history: the archive stop stays blocked and
	// keeps naming the fresh review.
	stillBlocked := resolveSDDStatusJSON(t, root)
	if stillBlocked.Dependencies.Archive != sddstatus.DependencyBlocked || stillBlocked.ReviewGate == nil {
		t.Fatalf("an empty-candidate review unblocked the archive: %#v", stillBlocked.Dependencies)
	}
	if stillBlocked.ReviewGate.Result == reviewtransaction.GateAllow {
		t.Fatalf("an empty-candidate review fabricated coverage of unmanaged history: %#v", stillBlocked.ReviewGate)
	}
	tokens = namedReviewCommandTokens(t, stillBlocked.ReviewGate.Reason)

	// Run the base-ref rerun the message names, supplying the one
	// operator-owned placeholder value: the boundary to re-govern from. The
	// fresh full review freezes the delivered range.
	if tokens[len(tokens)-1] != "--base-ref" {
		t.Fatalf("empty-receipt continuation = %v, want it to end at the operator's --base-ref value", tokens)
	}
	freshStart := dispatchNamedReviewStart(t, root, tokens, baselineCommit)
	if freshStart.ChangedFiles == 0 {
		t.Fatalf("the fresh full review froze no content: %#v", freshStart)
	}
	finalizeFacadeLineage(t, root, freshStart.LineageID)

	// Completing the named review clears the stop: the fresh receipt governs
	// the current state even though stale terminal receipts remain in history.
	cleared := resolveSDDStatusJSON(t, root)
	if cleared.ReviewGate == nil || cleared.ReviewGate.Result != reviewtransaction.GateAllow {
		t.Fatalf("completing the named fresh review did not clear the stop: %#v; reasons = %v",
			cleared.ReviewGate, cleared.BlockedReasons)
	}
	if cleared.Dependencies.Archive == sddstatus.DependencyBlocked {
		t.Fatalf("archive still blocked after the fresh full review: %v", cleared.BlockedReasons)
	}
	if cleared.NextRecommended != "archive" {
		t.Fatalf("nextRecommended = %q, want archive", cleared.NextRecommended)
	}
}

// TestSDDStatusArchiveNeverTreatsAnEmptyCandidateReviewAsCoverage pins the
// fabrication defect in its purest shape: no prior receipts at all, unmanaged
// delivered history, and a single approved empty-candidate receipt. The
// archive must record that nothing was reviewed and keep naming the fresh
// review with its base-ref selector.
func TestSDDStatusArchiveNeverTreatsAnEmptyCandidateReviewAsCoverage(t *testing.T) {
	reviewModeHome(t)
	root := t.TempDir()
	seedArchiveGatedSDDChange(t, root)

	disableReviewForClone(t, root)
	writeSDDStatusFile(t, root+"/docs/unmanaged.md", "# unmanaged\n\nplain prose, delivered while disabled.\n")
	commitAllSDDStatus(t, root, "unmanaged delivery")
	enableReviewForClone(t, root)

	emptyStart := startFacadeReviewResult(t, root, "reenable-empty-only")
	if emptyStart.ChangedFiles != 0 {
		t.Fatalf("clean-tree start froze %d files, fixture is wrong", emptyStart.ChangedFiles)
	}
	finalizeFacadeLineage(t, root, emptyStart.LineageID)

	status := resolveSDDStatusJSON(t, root)
	if status.ReviewGate == nil || status.ReviewGate.Result == reviewtransaction.GateAllow {
		t.Fatalf("a review of zero bytes was recorded as coverage: %#v", status.ReviewGate)
	}
	if status.Dependencies.Archive != sddstatus.DependencyBlocked {
		t.Fatalf("archive = %q over unreviewed history with only an empty receipt, want blocked", status.Dependencies.Archive)
	}
	if tokens := namedReviewCommandTokens(t, status.ReviewGate.Reason); len(tokens) < 2 || tokens[0] != "review" || tokens[1] != "start" {
		t.Fatalf("empty-receipt stop named %v, want the fresh review start", tokens)
	}
	if !strings.Contains(status.ReviewGate.Reason, "--base-ref") {
		t.Fatalf("empty-receipt stop does not name the base-ref rerun: %q", status.ReviewGate.Reason)
	}
}

// TestSDDStatusEnabledMissingReceiptStopNamesTheFreshReview keeps the plainest
// enabled stop runnable: a change at its archive decision with no review
// authority anywhere must name `gentle-ai review start` instead of only
// stating that a receipt is missing.
func TestSDDStatusEnabledMissingReceiptStopNamesTheFreshReview(t *testing.T) {
	reviewModeHome(t)
	root := t.TempDir()
	seedArchiveGatedSDDChange(t, root)

	status := resolveSDDStatusJSON(t, root)
	if status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateInvalidated {
		t.Fatalf("enabled missing-receipt gate = %#v, want invalidated", status.ReviewGate)
	}
	if !strings.Contains(status.ReviewGate.Reason, "terminal review receipt is missing") {
		t.Fatalf("missing-receipt reason lost its cause: %q", status.ReviewGate.Reason)
	}
	if tokens := namedReviewCommandTokens(t, status.ReviewGate.Reason); len(tokens) < 2 || tokens[0] != "review" || tokens[1] != "start" {
		t.Fatalf("missing-receipt stop named %v, want the fresh review start", tokens)
	}
}
