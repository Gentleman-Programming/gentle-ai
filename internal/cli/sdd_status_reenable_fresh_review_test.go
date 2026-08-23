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

// dispatchReviewStart runs an explicit fresh full review through the public
// review router. It deliberately does not derive a start command from archive
// status: an absent governing review leaves archive under ordinary policy.
func dispatchReviewStart(t *testing.T, repo, lineage string, extra ...string) ReviewFacadeStartResult {
	t.Helper()
	args := []string{"start", "--cwd", repo, "--lineage", lineage}
	args = append(args, extra...)
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("fresh review start %q exits non-zero: gentle-ai review %v: %v\n%s", lineage, args, err, output.String())
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	return started
}

func requireEnabledOrdinaryArchive(t *testing.T, status sddstatus.Status, subject string) {
	t.Helper()
	if status.ReviewGate != nil {
		t.Fatalf("%s reviewGate = %#v, want structural absence without governing review authority", subject, status.ReviewGate)
	}
	if status.ReviewOffer == nil || !status.ReviewOffer.Available {
		t.Fatalf("%s reviewOffer = %#v, want an available non-deciding review invitation", subject, status.ReviewOffer)
	}
	if status.Dependencies.Archive != sddstatus.DependencyReady || status.NextRecommended != "archive" {
		t.Fatalf("%s archive=%q next=%q, want ordinary ready/archive", subject, status.Dependencies.Archive, status.NextRecommended)
	}
}

func commitAllSDDStatus(t *testing.T, repo, message string) string {
	t.Helper()
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", message)
	return strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
}

// TestSDDStatusReEnableSequenceLandsOnTheFreshFullReview drives the maintainer's
// sequence end to end: reviewed baseline, kill switch off, two unmanaged
// deliveries, switch back on, and an explicitly requested fresh full review.
// Terminal capture burns each authority, so archive never replays either
// approval as coverage; without a governing review, it remains ordinary policy.
func TestSDDStatusReEnableSequenceLandsOnTheFreshFullReview(t *testing.T) {
	reviewEnabledHome(t)
	root := t.TempDir()
	seedArchiveGatedSDDChange(t, root)

	writeSDDStatusFile(t, root+"/docs/baseline.md", "# baseline\n\nplain prose, no executable content.\n")
	runReviewCLIGit(t, root, "add", "-A")
	baselineStarted := startFacadeReviewResult(t, root, "reenable-baseline")
	if baselineStarted.State != "approved" || baselineStarted.Action != "closed" {
		t.Fatalf("zero-lens baseline START = %#v", baselineStarted)
	}
	baselineCommit := commitAllSDDStatus(t, root, "baseline reviewed delivery")

	disableReviewForClone(t, root)
	writeSDDStatusFile(t, root+"/docs/unmanaged-one.md", "# one\n\nplain prose, delivered while disabled.\n")
	commitAllSDDStatus(t, root, "unmanaged delivery one")
	writeSDDStatusFile(t, root+"/docs/unmanaged-two.md", "# two\n\nplain prose, delivered while disabled.\n")
	commitAllSDDStatus(t, root, "unmanaged delivery two")

	disabled := resolveSDDStatusJSON(t, root)
	if disabled.Dependencies.Archive != sddstatus.DependencyReady || disabled.NextRecommended != "archive" {
		t.Fatalf("disabled archive=%q next=%q, want ordinary ready/archive", disabled.Dependencies.Archive, disabled.NextRecommended)
	}
	if disabled.ReviewGate != nil || disabled.ReviewOffer != nil {
		t.Fatalf("disabled status fabricated review state: gate=%#v offer=%#v", disabled.ReviewGate, disabled.ReviewOffer)
	}

	// Re-enabling does not turn a burned baseline approval into a governing
	// receipt or an archive stop. It makes a fresh review available, but the
	// invitation is non-deciding until the operator actually starts one.
	enableReviewForClone(t, root)
	reenabled := resolveSDDStatusJSON(t, root)
	requireEnabledOrdinaryArchive(t, reenabled, "re-enabled archive over unmanaged history")

	// A full review exists only because this fixture explicitly starts it over
	// the committed delivered range. Its terminal event burns authority, so
	// SDD status remains ordinary policy rather than replayable coverage.
	freshStart := dispatchReviewStart(t, root, "reenable-fresh", "--base-ref", baselineCommit)
	if freshStart.ChangedFiles == 0 {
		t.Fatalf("the explicitly requested fresh full review froze no content: %#v", freshStart)
	}
	if freshStart.State != "approved" || freshStart.Action != "closed" {
		t.Fatalf("fresh zero-lens START = %#v", freshStart)
	}

	postBurn := resolveSDDStatusJSON(t, root)
	requireEnabledOrdinaryArchive(t, postBurn, "closed fresh review")
}

// TestSDDStatusArchiveNeverTreatsAnEmptyCandidateReviewAsCoverage keeps one
// historical compatibility fixture: an older build could mint an approved
// empty-candidate authority. Current closure burns it. Therefore it cannot
// become coverage or a deciding archive gate; SDD proceeds under ordinary
// repository policy with only a non-deciding review invitation.
func TestSDDStatusArchiveNeverTreatsAnEmptyCandidateReviewAsCoverage(t *testing.T) {
	reviewEnabledHome(t)
	root := t.TempDir()
	seedArchiveGatedSDDChange(t, root)

	disableReviewForClone(t, root)
	writeSDDStatusFile(t, root+"/docs/unmanaged.md", "# unmanaged\n\nplain prose, delivered while disabled.\n")
	commitAllSDDStatus(t, root, "unmanaged delivery")
	enableReviewForClone(t, root)

	// A current zero-lens START burns immediately. It leaves no compact
	// receipt or authority that could govern this later archive decision.
	lineage := startLowRiskFacadeReview(t, root)
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), root, lineage)
	if err != nil {
		t.Fatal(err)
	}
	assertApprovedCompactAuthorityBurned(t, store, lineage)

	status := resolveSDDStatusJSON(t, root)
	requireEnabledOrdinaryArchive(t, status, "burned empty-candidate review")
}

// TestSDDStatusEnabledMissingReceiptIsDeclineNotAStop is corrective verify
// cycle 4's BLOCKER-1 (rdd-post-verify-review-offer's "Decline Proceeds to
// Unmanaged Ordinary Archive"): a change at its archive decision with no
// review authority anywhere is decline-by-absence-of-action, not a stop --
// the offer is an invitation, never a gate. Superseded (documented, not
// silently dropped): this test previously required naming `gentle-ai review
// start` as a runnable exit from a blocked state; there is no blocked state
// to exit from anymore for this exact fixture.
func TestSDDStatusEnabledMissingReceiptIsDeclineNotAStop(t *testing.T) {
	reviewEnabledHome(t)
	root := t.TempDir()
	seedArchiveGatedSDDChange(t, root)

	status := resolveSDDStatusJSON(t, root)
	if status.ReviewGate != nil {
		t.Fatalf("enabled missing-receipt gate = %#v, want structural absence (decline)", status.ReviewGate)
	}
	if status.ReviewOffer == nil || !status.ReviewOffer.Available {
		t.Fatalf("enabled missing-receipt reviewOffer = %#v, want an available invitation", status.ReviewOffer)
	}
	if status.Dependencies.Archive != sddstatus.DependencyReady || status.NextRecommended != "archive" {
		t.Fatalf("enabled missing-receipt archive=%q next=%q, want ready/archive", status.Dependencies.Archive, status.NextRecommended)
	}
}
