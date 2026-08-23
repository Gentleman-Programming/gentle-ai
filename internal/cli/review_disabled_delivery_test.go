package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// wantEnabledReviewGateFields is the exact shipped field set of a gate result
// produced while receipt-driven development is on. It guards the regression that
// matters most here: the delivery disposition must stay invisible on every path
// that already worked, so no consumer of the current projection changes.
var wantEnabledReviewGateFields = []string{"action", "allowed", "context", "reason", "result", "schema"}

// TestReviewValidateReportsDisabledUnmanagedDeliveryWithoutReceipt closes the
// contract breach: the guidance installed on every agent promises that delivery
// under a disabled switch reports `disabled/unmanaged`, and until now nothing
// emitted that token on the wire.
func TestReviewValidateReportsDisabledUnmanagedDeliveryWithoutReceipt(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate authored while disabled\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	disableReviewForClone(t, repo)

	var output bytes.Buffer
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &output)
	// The gate reports; it does not veto. Ordinary repository policy governs
	// delivery once the user has switched receipt-driven development off.
	if err != nil {
		t.Fatalf("disabled delivery gate vetoed delivery instead of reporting it: %v\n%s", err, output.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Schema != ReviewValidateSchema {
		t.Fatalf("disabled delivery left the typed gate schema = %q", result.Schema)
	}
	if result.Delivery != reviewtransaction.RDDDeliveryDisabledUnmanaged {
		t.Fatalf("disabled receiptless delivery = %q, want %q", result.Delivery, reviewtransaction.RDDDeliveryDisabledUnmanaged)
	}
	// Unmanaged by choice is neither an approval nor a fault.
	if result.Allowed || result.Result == reviewtransaction.GateAllow {
		t.Fatalf("disabled delivery fabricated an approval: %#v", result)
	}
	var denied ReviewGateDeniedError
	if errors.As(err, &denied) {
		t.Fatalf("disabled delivery was reported as a denial: %#v", denied)
	}
	// Wave 5 Slice 2 (design decision 4): the switch is consulted before any
	// authority read, so this report carries no discovery-kind detail at
	// all -- there is no receipt-discovery outcome to describe, because
	// discovery never runs. (Previously this asserted
	// Denial.Stage == "receipt-discovery"; the enabled-mode sibling
	// TestReviewValidateWithoutReceiptStillDeniesWhileReviewIsEnabled below
	// still asserts that shape, where discovery genuinely still runs.)
	if result.Context.Denial != nil {
		t.Fatalf("disabled delivery leaked discovery-kind detail: %#v", result.Context.Denial)
	}

	// The report is an observation, so replaying the same request must return
	// the same bytes and must not create review authority.
	var replay bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &replay); err != nil {
		t.Fatalf("replayed disabled delivery gate: %v\n%s", err, replay.String())
	}
	if !bytes.Equal(replay.Bytes(), output.Bytes()) {
		t.Fatalf("disabled delivery report is not replay stable:\nfirst:\n%s\nreplay:\n%s", output.String(), replay.String())
	}
	// The clone-local kill-switch override shares the review-transactions root,
	// so the assertion targets the authority generation directory itself.
	if _, err := os.Stat(filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "v2")); !os.IsNotExist(err) {
		t.Fatalf("a disabled delivery report created review authority: %v", err)
	}
}

// TestReviewValidateWithoutReceiptStillDeniesWhileReviewIsEnabled is the
// regression guard: with the switch on, a receiptless candidate keeps today's
// denial, today's exit status, and today's exact field set.
func TestReviewValidateWithoutReceiptStillDeniesWhileReviewIsEnabled(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("unreviewed candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &output); err != nil {
		t.Fatalf("enabled receiptless delivery: %v\n%s", err, output.String())
	}
	assertEnabledUnmanagedGatePayload(t, output.Bytes(), reviewtransaction.GatePreCommit)
}

// TestReviewValidateReportsDisabledUnmanagedDeliveryOverDeliveredWorkspaceReceiptAtPrePush
// closes the second community-reported gap (Wladimirfn, PR #1801): a workspace
// (current-changes) receipt whose candidate was delivered exactly as reviewed,
// then a new commit authored while disabled, then pre-push. The candidate now
// publishes two commits past the reviewed base, so the receipt's one-commit
// delivery rule cannot hold — a deterministic statement about candidate shape
// versus the reviewed receipt, made over a provably healthy authority store.
// It must classify as a receipt/scope mismatch that the disabled switch
// reports as `disabled/unmanaged` with exit 0, never as `authority_corrupted`.
//
// Wave 5 Slice 2 supersession (design decision 4): the "delivery-shape-mismatch"
// detail this disabled report used to carry moved to the enabled-mode
// sibling TestReviewValidateDeniesDeliveredWorkspaceReceiptPrePushAsScopeMismatchWhileEnabled
// below, where discovery genuinely still runs.
func TestReviewValidateReportsDisabledUnmanagedDeliveryOverDeliveredWorkspaceReceiptAtPrePush(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	// The publication boundary stays at the base commit: the reviewed delivery
	// was never pushed, which is exactly why pre-push runs here.
	configureCLIReviewPublicationRemote(t, repo, branch)

	// A workspace review of the dirty candidate, delivered exactly as reviewed
	// in one commit.
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("reviewed candidate behavior\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finalizeFacadeReviewForRepo(t, repo)
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "reviewed candidate")

	disableReviewForClone(t, repo)

	// New work authored and committed while disabled: no receipt can exist for
	// it, and the healthy prior receipt must not become a corruption verdict.
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("work authored while disabled\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "authored while disabled")

	var output bytes.Buffer
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePrePush)}, &output)
	// The gate reports; it does not veto: ordinary repository policy governs
	// delivery once receipt-driven development is off.
	if err != nil {
		t.Fatalf("disabled delivery over a delivered workspace receipt was denied instead of reported: %v\n%s", err, output.String())
	}
	assertDisabledUnmanagedGate(t, err, output.Bytes())

	// The report is an observation: replaying returns the same bytes.
	var replay bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePrePush)}, &replay); err != nil {
		t.Fatalf("replayed disabled delivery gate: %v\n%s", err, replay.String())
	}
	if !bytes.Equal(replay.Bytes(), output.Bytes()) {
		t.Fatalf("disabled delivery report is not replay stable:\nfirst:\n%s\nreplay:\n%s", output.String(), replay.String())
	}
}

// TestReviewValidateReportsDisabledUnmanagedDeliveryOverCorruptedAuthorityAtPrePush
// is the pre-push half of the corrupted-authority decision. This test used to
// assert the opposite (`...KeepsFailingClosedOnCorruptedAuthorityWhileDisabledAtPrePush`)
// on the reasoning that damage is not "unmanaged by choice". The maintainer's
// rule supersedes that: with reviews off, receipt-driven development does not
// exist, so damage to its own private store cannot stop an ordinary push. The
// damage is reported, not hidden — the denial code stays `authority_corrupted`
// and the reason names it — and it is deferred, not forgiven, because
// re-enabling resumes ordinary non-deciding delivery behavior.
//
// Wave 5 Slice 2 supersession (design decision 4): the switch is consulted
// BEFORE any authority read regardless of gate, so this pre-push disabled
// report no longer discovers the corruption either -- the "unavailable or
// corrupted" visibility property is gate-independent (corruption is about
// the authority inventory, not the boundary) and is already the sole
// responsibility of the enabled-mode non-deciding gate coverage
// (pre-commit gate, identical inventory-corruption mechanism).
func TestReviewValidateReportsDisabledUnmanagedDeliveryOverCorruptedAuthorityAtPrePush(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	configureCLIReviewPublicationRemote(t, repo, branch)

	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("reviewed candidate behavior\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finalizeFacadeReviewForRepo(t, repo)
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "reviewed candidate")

	disableReviewForClone(t, repo)

	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("work authored while disabled\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "authored while disabled")

	// Damage the authority inventory: a truncated compact record is corruption,
	// not a stale-but-healthy receipt.
	broken := filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "v2", "corrupt-while-disabled")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "review-state.json"), []byte("{\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	runErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePrePush)}, &output)
	assertDisabledUnmanagedGate(t, runErr, output.Bytes())
}

// TestReviewValidateReportsDisabledUnmanagedDeliveryWhenNoUpstreamConfigured
// proves issue-1832: a disposable repository with no remote and no branch
// upstream has no publication boundary to derive at all. While
// receipt-driven development is disabled, that is not authority damage and
// not something the gate should be blocking pre-push on — it is exactly the
// same "no receipt can govern this while off" shape as a missing,
// scope-changed, or unrelated receipt. Before the fix, the gate resolved the
// remote target BEFORE honoring the kill switch and failed closed with a
// typed target-resolution denial instead of reporting disabled/unmanaged.
func TestReviewValidateReportsDisabledUnmanagedDeliveryWhenNoUpstreamConfigured(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	// Deliberately NO remote and NO branch upstream: initReviewCLIRepo never
	// configures one, and this test must not call
	// configureCLIReviewPublicationRemote — that is the entire point of the
	// reporter's fixture.

	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("reviewed candidate behavior\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finalizeFacadeReviewForRepo(t, repo)
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "reviewed candidate")

	disableReviewForClone(t, repo)

	var output bytes.Buffer
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePrePush)}, &output)
	// The gate reports; it does not veto. A repository with no upstream simply
	// has no publication boundary to derive — that is not authority damage,
	// and while reviews are disabled it is not something the gate should be
	// blocking on at all.
	if err != nil {
		t.Fatalf("disabled delivery with no configured upstream was denied instead of reported: %v\n%s", err, output.String())
	}
	assertDisabledUnmanagedGate(t, err, output.Bytes())

	// The report is an observation, so replaying the same request must return
	// the same bytes.
	var replay bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePrePush)}, &replay); err != nil {
		t.Fatalf("replayed disabled delivery gate: %v\n%s", err, replay.String())
	}
	if !bytes.Equal(replay.Bytes(), output.Bytes()) {
		t.Fatalf("disabled delivery report is not replay stable:\nfirst:\n%s\nreplay:\n%s", output.String(), replay.String())
	}
}

// TestReviewValidateReportsDisabledUnmanagedDeliveryOverCorruptedAuthorityNoUpstream
// covers issue-1832's own fix site under the maintainer's rule. It previously
// asserted the opposite (`...KeepsFailingClosedOnCorruptedAuthorityWhileDisabledNoUpstream`).
// A damaged store with no upstream and the switch off no longer stops a push,
// because a switched-off system has no implications.
//
// Wave 5 Slice 2 supersession (design decision 4): same as the pre-push
// corrupted-authority case above -- corruption visibility is gate-independent
// and already the sole responsibility of
// the enabled-mode non-deciding gate coverage.
func TestReviewValidateReportsDisabledUnmanagedDeliveryOverCorruptedAuthorityNoUpstream(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)

	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("reviewed candidate behavior\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finalizeFacadeReviewForRepo(t, repo)
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "reviewed candidate")

	disableReviewForClone(t, repo)

	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("work authored while disabled\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "authored while disabled")

	// Damage the authority inventory: a truncated compact record is
	// corruption, not a stale-but-healthy receipt or an unresolvable target.
	broken := filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "v2", "corrupt-while-disabled-no-upstream")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "review-state.json"), []byte("{\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	runErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePrePush)}, &output)
	assertDisabledUnmanagedGate(t, runErr, output.Bytes())
}

// TestReviewValidateReportsDisabledUnmanagedDeliveryAtPostApply keeps direct
// post-apply coverage for the disabled non-deciding gate.
func TestReviewValidateReportsDisabledUnmanagedDeliveryAtPostApply(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	disableReviewForClone(t, repo)

	var output bytes.Buffer
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &output)
	assertDisabledUnmanagedGate(t, err, output.Bytes())

	var replay bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &replay); err != nil {
		t.Fatalf("replayed disabled post-apply delivery gate: %v\n%s", err, replay.String())
	}
	if !bytes.Equal(replay.Bytes(), output.Bytes()) {
		t.Fatalf("disabled post-apply delivery is not replay stable:\nfirst:\n%s\nreplay:\n%s", output.String(), replay.String())
	}
}

// finalizeFacadeReviewForRepo runs one complete reviewed flow over the live
// candidate: start with the given extra arguments, submit one clean result per
// selected lens, and finalize to a terminal receipt.
//
// Uses runLegacyFacadeStartForTest (Wave 7 S7a/WU18a), not RunReviewFacadeStart
// directly: every caller of this helper needs genuine legacy (compact-v2)
// authority (proven by pervasive reviewtransaction.CompactAuthoritativeStore/
// CompactAuthorityLeaves follow-up reads across its call sites). It also
// sidesteps issue #2447's direct-route refusal for base-diff/workspace-overlay
// candidates large enough to select a lens: this helper is SETUP for behavior
// unrelated to that refusal (delivery gates), so bypassing
// RunReviewFacadeStart's CLI dispatch entirely, the same pattern every other
// legacy fixture in this codebase now uses, is the right tool rather than
// routing SETUP through the negotiated contract.
func finalizeFacadeReviewForRepo(t *testing.T, repo string, startExtra ...string) {
	t.Helper()
	var output bytes.Buffer
	if err := runLegacyFacadeStartForTest(t, append([]string{"--cwd", repo}, startExtra...), &output); err != nil {
		t.Fatalf("start facade review: %v\n%s", err, output.String())
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if len(started.SelectedLenses) == 0 {
		if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID}, io.Discard); err != nil {
			t.Fatal(err)
		}
		return
	}
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidencePath, []byte("focused tests pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finalizeArgs := append([]string{"--cwd", repo, "--lineage", started.LineageID}, facadeReviewerResultArgs(t, repo, started)...)
	if err := RunReviewFacadeFinalize(append(finalizeArgs, "--evidence", evidencePath), io.Discard); err != nil {
		t.Fatal(err)
	}
}

// finalizeFacadeReviewForRepoSelectingUntracked declares only the caller's
// explicitly intended untracked paths against the current canonical inventory.
// It never broadens a fixture by sweeping unrelated untracked files into scope.
func finalizeFacadeReviewForRepoSelectingUntracked(t *testing.T, repo string, intended ...string) {
	t.Helper()
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	_, digest, err := builder.IntendedUntrackedInventory(t.Context())
	if err != nil {
		t.Fatalf("discover intended untracked inventory: %v", err)
	}
	startExtra := []string{
		"--untracked-scope=select",
		"--expected-untracked-inventory=" + digest,
	}
	for _, path := range intended {
		startExtra = append(startExtra, "--intended-untracked="+path)
	}
	finalizeFacadeReviewForRepo(t, repo, startExtra...)
}

func disableReviewForClone(t *testing.T, repo string) {
	t.Helper()
	var output bytes.Buffer
	if err := RunReviewMode([]string{"disable", "--cwd", repo, "--scope", "clone", "--json"}, &output); err != nil {
		t.Fatalf("disable receipt-driven development: %v\n%s", err, output.String())
	}
	if status := decodeReviewModeResult(t, output.Bytes()).Status; status.Effective != reviewtransaction.RDDModeOff {
		t.Fatalf("kill switch did not take effect: %#v", status)
	}
}

// TestReviewValidateReportsDisabledUnmanagedDeliveryOverThreeStaleReceiptsAtPrePush
// adopts tester fisidj's exploratory repro (Windows + OpenCode, Refresh 4, PR
// #1801 comment 2026-07-26T10:58) as an explicit fixture for the Phase 3c fix
// (organic-dx Phase 3f task 3f.1). Phase 3c's own fixture
// (TestReviewValidatePluralStaleReceiptsReportDisabledUnmanagedDelivery above)
// used two receipts built by cloning compact authority directly; fisidj's
// repro is the cleaner, real-world shape -- three separate reviewed and
// finalized commits over a real bare remote, the first two actually pushed --
// and proves the DeterministicallyStaleOnly fix is not count-specific.
func TestReviewValidateReportsDisabledUnmanagedDeliveryOverThreeStaleReceiptsAtPrePush(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	configureCLIReviewPublicationRemote(t, repo, branch)

	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	docPath := filepath.Join(repo, "docs", "plural-stale-three.md")

	// Docs commit 1: reviewed, finalized, pushed.
	if err := os.WriteFile(docPath, []byte("first reviewed docs change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finalizeFacadeReviewForRepoSelectingUntracked(t, repo, "docs/plural-stale-three.md")
	runReviewCLIGit(t, repo, "add", "docs/plural-stale-three.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "docs commit 1")
	runReviewCLIGit(t, repo, "push", "origin", branch)

	// Docs commit 2: reviewed, finalized, pushed.
	if err := os.WriteFile(docPath, []byte("second reviewed docs change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finalizeFacadeReviewForRepo(t, repo)
	runReviewCLIGit(t, repo, "add", "docs/plural-stale-three.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "docs commit 2")
	runReviewCLIGit(t, repo, "push", "origin", branch)

	// Docs commit 3: reviewed, finalized, NOT pushed -- this is the receipt
	// that must still classify stale (scope-changed) once the fourth,
	// unreviewed commit lands below.
	if err := os.WriteFile(docPath, []byte("third reviewed docs change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finalizeFacadeReviewForRepo(t, repo)
	runReviewCLIGit(t, repo, "add", "docs/plural-stale-three.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "docs commit 3")

	disableReviewForClone(t, repo)

	// One unreviewed docs commit, authored entirely while disabled.
	if err := os.WriteFile(docPath, []byte("unreviewed docs change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "docs/plural-stale-three.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "unreviewed docs commit")

	var output bytes.Buffer
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePrePush)}, &output)
	if err != nil {
		t.Fatalf("three stale receipts while disabled were denied instead of reported: %v\n%s", err, output.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Delivery != reviewtransaction.RDDDeliveryDisabledUnmanaged {
		t.Fatalf("three stale receipts while disabled = %q, want %q", result.Delivery, reviewtransaction.RDDDeliveryDisabledUnmanaged)
	}
	if result.Allowed || result.Result == reviewtransaction.GateAllow {
		t.Fatalf("three stale receipts while disabled fabricated an approval: %#v", result)
	}
	var denied ReviewGateDeniedError
	if errors.As(err, &denied) {
		t.Fatalf("three stale receipts while disabled were reported as a denial: %#v", denied)
	}
}
