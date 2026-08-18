package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
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

// TestReviewValidateReportsDisabledUnmanagedDeliveryEvenOverAGoverningReceipt
// is Wave 5 Slice 2's most consequential behavior reversal (design decision
// 4; rdd-receipt-only-gates/spec.md's "Kill switch off short-circuits before
// authority discovery" scenario, stated as a firm requirement, not a tagged
// pending assumption): the kill switch is now consulted before ANY receipt
// or authority read, so even an EXACTLY governing receipt is never
// consulted while disabled -- not read, not silently kept authoritative, not
// distinguished from "no receipt at all". This replaces the former
// TestReviewValidateKeepsGoverningReceiptAuthoritativeWhileDisabled, which
// asserted the opposite: that disabling froze a governing receipt read-only
// and it kept authorizing delivery. Under the new ordered contract
// (capability -> kill switch -> governing authority -> relation -> verdict)
// that asymmetry no longer holds: while disabled, receipt-driven development
// does not exist at all, so nothing -- not even an approval earned moments
// earlier over the exact same bytes -- governs. Re-enabling rediscovers the
// SAME receipt and it governs again unchanged (proven below): the receipt
// itself is never mutated or invalidated, it is simply not consulted while
// the switch is off.
func TestReviewValidateReportsDisabledUnmanagedDeliveryEvenOverAGoverningReceipt(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("reviewed candidate behavior\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidencePath, []byte("focused tests pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finalizeArgs := append([]string{"--cwd", repo, "--lineage", started.LineageID}, facadeReviewerResultArgs(t, repo, started)...)
	if err := RunReviewFacadeFinalize(append(finalizeArgs, "--evidence", evidencePath), io.Discard); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")

	// Before disabling: the receipt genuinely governs and allows.
	var enabled bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &enabled); err != nil {
		t.Fatalf("receipt-governed gate before disabling: %v\n%s", err, enabled.String())
	}
	assertReviewGateResult(t, enabled.Bytes(), reviewtransaction.GateAllow)

	disableReviewForClone(t, repo)

	// While disabled: the same governing receipt is never consulted. The
	// report is the identical generic disabled/unmanaged shape every other
	// disabled scenario produces, not the allow the receipt would have
	// earned.
	var disabled bytes.Buffer
	disabledErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &disabled)
	if disabledErr != nil {
		t.Fatalf("disabled gate vetoed delivery instead of reporting it: %v\n%s", disabledErr, disabled.String())
	}
	assertDisabledUnmanagedGate(t, disabledErr, disabled.Bytes())

	// Re-enabling rediscovers the SAME receipt, unmutated, and it governs
	// again exactly as before -- proving the switch never touched it.
	enableReviewForClone(t, repo)
	var reEnabled bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &reEnabled); err != nil {
		t.Fatalf("receipt-governed gate after re-enabling: %v\n%s", err, reEnabled.String())
	}
	if !bytes.Equal(reEnabled.Bytes(), enabled.Bytes()) {
		t.Fatalf("the receipt-governed projection changed across a disable/re-enable cycle:\nbefore:\n%s\nafter:\n%s", enabled.String(), reEnabled.String())
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
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &output)
	var denied ReviewGateDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("enabled receiptless delivery error = %T %v", err, err)
	}
	if fields := strictReviewJSONFields(t, output.Bytes()); !reflect.DeepEqual(fields, wantEnabledReviewGateFields) {
		t.Fatalf("enabled gate fields = %v, want %v", fields, wantEnabledReviewGateFields)
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Delivery != "" {
		t.Fatalf("an enabled switch reported a delivery disposition: %#v", result)
	}
	if result.Allowed || result.Context.Denial == nil || result.Context.Denial.Stage != "receipt-discovery" ||
		result.Context.Denial.Code != string(ReviewReceiptMissing) {
		t.Fatalf("enabled receiptless denial = %#v", result)
	}
}

// TestReviewValidateReportsDisabledUnmanagedDeliveryWithPriorReceipt closes the
// community-reported gap (Andiveli, PR #1801): a repository that already holds
// receipts from earlier reviewed flows must still report `disabled/unmanaged`
// for work authored after the switch went off. A stale receipt that does not
// govern the current candidate is the expected state of a disabled clone — no
// new receipt could have been created while disabled — so it must not turn
// "unmanaged by choice" into a mismatch denial.
//
// Wave 5 Slice 2 (design decision 4): each shape also asserts the SAME drift
// still denies with its named code while reviews are ON, checked BEFORE
// disabling -- the discovery-kind visibility this test's disabled half used
// to assert moved here, since discovery genuinely still runs while enabled.
func TestReviewValidateReportsDisabledUnmanagedDeliveryWithPriorReceipt(t *testing.T) {
	shapes := []struct {
		name string
		// review earns the terminal receipt for the earlier candidate and
		// leaves the repository exactly as the reviewed flow delivered it.
		review func(t *testing.T, repo string)
		gate   reviewtransaction.GateKind
		// wantDenialCode is the exact receipt-discovery outcome today's gate
		// turns into a fail-closed denial and the fix must keep discoverable.
		wantDenialCode string
	}{
		{
			// Andiveli's shape: a committed candidate reviewed against its
			// base, delivered, then a new commit on top while disabled. The
			// stale receipt binds a different candidate tree, so pre-push
			// discovery classifies it receipt_scope_changed and denies with
			// candidate-or-paths-mismatch.
			name: "scope-changed receipt at pre-push",
			review: func(t *testing.T, repo string) {
				base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
				if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("reviewed candidate behavior\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				runReviewCLIGit(t, repo, "add", "tracked.txt")
				runReviewCLIGit(t, repo, "commit", "-qm", "reviewed candidate")
				finalizeFacadeReviewForRepo(t, repo, "--base-ref", base, "--committed-only")
			},
			gate:           reviewtransaction.GatePrePush,
			wantDenialCode: "candidate-or-paths-mismatch",
		},
		{
			// The sibling shape: a workspace review delivered exactly as
			// reviewed, then a new commit while disabled. Discovery classifies
			// the stale receipt receipt_unrelated at pre-commit.
			name: "unrelated receipt at pre-commit",
			review: func(t *testing.T, repo string) {
				if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("reviewed candidate behavior\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				finalizeFacadeReviewForRepo(t, repo)
				runReviewCLIGit(t, repo, "add", "tracked.txt")
				runReviewCLIGit(t, repo, "commit", "-qm", "reviewed candidate")
			},
			gate:           reviewtransaction.GatePreCommit,
			wantDenialCode: string(ReviewReceiptUnrelated),
		},
	}
	for _, shape := range shapes {
		t.Run(shape.name, func(t *testing.T) {
			reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
			shape.review(t, repo)
			configureCLIReviewPublicationRemote(t, repo, branch)

			// New work authored and committed: no receipt can exist for it, so
			// the stale receipt must not turn "unmanaged by choice" into a
			// fault while disabled -- but while ENABLED it must still fail
			// closed and name this exact discovery kind.
			if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("work authored after the reviewed candidate\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runReviewCLIGit(t, repo, "add", "tracked.txt")
			runReviewCLIGit(t, repo, "commit", "-qm", "work authored after the reviewed candidate")

			var enabledOutput bytes.Buffer
			enabledErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(shape.gate)}, &enabledOutput)
			var enabledDenied ReviewGateDeniedError
			if !errors.As(enabledErr, &enabledDenied) {
				t.Fatalf("enabled drift over a prior receipt did not fail closed: %T %v\n%s", enabledErr, enabledErr, enabledOutput.String())
			}
			var enabledResult ReviewValidateResult
			decodeStrictReviewJSON(t, enabledOutput.Bytes(), &enabledResult)
			if enabledResult.Delivery != "" {
				t.Fatalf("an enabled switch reported a delivery disposition: %#v", enabledResult)
			}
			if enabledResult.Allowed || enabledResult.Context.Denial == nil || enabledResult.Context.Denial.Code != shape.wantDenialCode {
				t.Fatalf("enabled drift denial = %#v, want code %q", enabledResult.Context.Denial, shape.wantDenialCode)
			}

			disableReviewForClone(t, repo)

			var output bytes.Buffer
			err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(shape.gate)}, &output)
			// The gate reports; it does not veto: ordinary repository policy
			// governs delivery once receipt-driven development is off, and the
			// prior receipt governs only the bytes it approved.
			if err != nil {
				t.Fatalf("disabled delivery with a prior receipt was denied instead of reported: %v\n%s", err, output.String())
			}
			assertDisabledUnmanagedGate(t, err, output.Bytes())

			// The report is an observation: replaying returns the same bytes.
			var replay bytes.Buffer
			if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(shape.gate)}, &replay); err != nil {
				t.Fatalf("replayed disabled delivery gate: %v\n%s", err, replay.String())
			}
			if !bytes.Equal(replay.Bytes(), output.Bytes()) {
				t.Fatalf("disabled delivery report is not replay stable:\nfirst:\n%s\nreplay:\n%s", output.String(), replay.String())
			}
		})
	}
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

// TestReviewValidateDeniesDeliveredWorkspaceReceiptPrePushAsScopeMismatchWhileEnabled
// is the enabled half of the same forensic finding: the user's authority store
// is provably healthy, so the denial must say what is actually true — the
// candidate's delivered shape no longer matches the reviewed receipt — and
// never claim `authority_corrupted`.
func TestReviewValidateDeniesDeliveredWorkspaceReceiptPrePushAsScopeMismatchWhileEnabled(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("unreviewed second commit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "unreviewed second commit")

	var output bytes.Buffer
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePrePush)}, &output)
	var denied ReviewGateDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("enabled two-commit delivery over a workspace receipt error = %T %v", err, err)
	}
	if fields := strictReviewJSONFields(t, output.Bytes()); !reflect.DeepEqual(fields, wantEnabledReviewGateFields) {
		t.Fatalf("enabled gate fields = %v, want %v", fields, wantEnabledReviewGateFields)
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Delivery != "" {
		t.Fatalf("an enabled switch reported a delivery disposition: %#v", result)
	}
	if result.Allowed || result.Result != reviewtransaction.GateScopeChanged {
		t.Fatalf("enabled delivery-shape denial = %#v, want result %q", result, reviewtransaction.GateScopeChanged)
	}
	// The store is healthy: the denial must name the shape mismatch, not
	// corruption.
	if result.Context.Denial == nil || result.Context.Denial.Code != "delivery-shape-mismatch" {
		t.Fatalf("enabled delivery-shape denial code = %#v, want %q", result.Context.Denial, "delivery-shape-mismatch")
	}
	if result.Context.Denial.Code == string(ReviewAuthorityCorrupted) {
		t.Fatalf("a healthy authority was reported as corrupted: %#v", result.Context.Denial)
	}
	if !strings.Contains(result.Reason, "reviewed delivery is not exactly one commit from its reviewed base") {
		t.Fatalf("enabled delivery-shape denial reason = %q, want the one-commit delivery rule", result.Reason)
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
// re-enabling rediscovers it and blocks again (see
// TestReviewValidateKeepsFailingClosedOnCorruptedAuthorityWhileEnabled).
//
// Wave 5 Slice 2 supersession (design decision 4): the switch is consulted
// BEFORE any authority read regardless of gate, so this pre-push disabled
// report no longer discovers the corruption either -- the "unavailable or
// corrupted" visibility property is gate-independent (corruption is about
// the authority inventory, not the boundary) and is already the sole
// responsibility of TestReviewValidateKeepsFailingClosedOnCorruptedAuthorityWhileEnabled
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

// TestReviewValidateDeniesNoUpstreamTargetResolutionWhileEnabled pins the
// unchanged half of issue-1832's fix: with reviews ENABLED, a repository
// with no upstream must still produce exactly today's typed
// target-resolution denial (exit 1, Stage "target-resolution", Code
// "target_resolution_failed"), naming --base-ref <remote>/<branch> as the
// escape. Only the disabled path changes.
func TestReviewValidateDeniesNoUpstreamTargetResolutionWhileEnabled(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)

	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("reviewed candidate behavior\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finalizeFacadeReviewForRepo(t, repo)
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "reviewed candidate")

	var output bytes.Buffer
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePrePush)}, &output)
	var denied ReviewGateDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("enabled no-upstream pre-push error = %T %v\n%s", err, err, output.String())
	}
	if fields := strictReviewJSONFields(t, output.Bytes()); !reflect.DeepEqual(fields, wantEnabledReviewGateFields) {
		t.Fatalf("enabled gate fields = %v, want %v", fields, wantEnabledReviewGateFields)
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Delivery != "" {
		t.Fatalf("an enabled switch reported a delivery disposition: %#v", result)
	}
	if result.Allowed {
		t.Fatalf("enabled no-upstream target resolution fabricated an approval: %#v", result)
	}
	if result.Context.Denial == nil || result.Context.Denial.Stage != "target-resolution" || result.Context.Denial.Code != "target_resolution_failed" {
		t.Fatalf("enabled no-upstream denial = %#v, want stage %q code %q", result.Context.Denial, "target-resolution", "target_resolution_failed")
	}
	if !strings.Contains(result.Reason, "--base-ref <remote>/<branch>") {
		t.Fatalf("enabled no-upstream denial reason = %q, want it to name --base-ref <remote>/<branch>", result.Reason)
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
// TestReviewValidateKeepsFailingClosedOnCorruptedAuthorityWhileEnabled.
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

// TestReviewValidatePluralStaleReceiptsReportDisabledUnmanagedDelivery closes
// the community-reported blocker (decode2, PR #1801): plural terminal
// receipts are the NORM for any active receipt-driven-development user --
// nothing prunes them, and overlapping genesis paths classify scope-changed
// -- so this is the cross-product fixture that was missing from the disabled
// coverage above (which only ever exercised {none, one governing, one stale,
// corrupted, no-upstream}, never two). Before the fix: each stale receipt
// alone reported disabled/unmanaged (exit 0), but two together classified
// receipt_ambiguous, which was never in the unmanaged-while-disabled set, so
// the gate failed closed (exit 1) and blocked an ordinary commit with reviews
// OFF -- exactly the pre-commit-hook shape decode2 reported.
//
// Wave 5 Slice 2 supersession (design decision 4): the specific
// "candidate-or-paths-mismatch" (alone) and "receipt_ambiguous"
// (deterministically-stale, plural) codes this disabled report used to carry
// moved to TestReviewValidateStaleAndPluralStaleReceiptsFailClosedWhileEnabled
// below, which builds the identical alone-then-plural fixture progression
// with reviews ON throughout.
func TestReviewValidatePluralStaleReceiptsReportDisabledUnmanagedDelivery(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	logicalPath := "docs/plural-stale.md"
	lineageA := "review-disabled-plural-stale-a"
	lineageB := "review-disabled-plural-stale-b"

	_, storeA := approveDiscoveryMarkdownProjection(t, repo, lineageA, logicalPath, "reviewed\n", reviewtransaction.ProjectionWorkspace)
	if err := os.WriteFile(filepath.Join(repo, filepath.FromSlash(logicalPath)), []byte("drifted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	disableReviewForClone(t, repo)

	// Proof that each receipt alone yields exit 0 / disabled-unmanaged: only
	// lineage A's now-stale (scope-changed) receipt exists at this point.
	var alone bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &alone); err != nil {
		t.Fatalf("one stale receipt while disabled was denied instead of reported: %v\n%s", err, alone.String())
	}
	assertDisabledUnmanagedGate(t, nil, alone.Bytes())

	// cloneApprovedDiscoveryAuthority operates directly on the compact store,
	// bypassing the CLI's own disabled-mode gate on review/start -- exactly
	// as a second review earned BEFORE the switch was disabled would already
	// sit on disk, which is the realistic shape of the community report.
	cloneApprovedDiscoveryAuthority(t, repo, storeA, lineageB)

	gateInput := reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePostApply}
	_, _, discoveryErr := discoverCompactFacadeGateReview(context.Background(), repo, "", gateInput)
	var discovery *ReviewReceiptDiscoveryError
	if !errors.As(discoveryErr, &discovery) || discovery.Kind != ReviewReceiptAmbiguous || !discovery.DeterministicallyStaleOnly ||
		!reflect.DeepEqual(discovery.Candidates, []string{lineageA, lineageB}) {
		t.Fatalf("plural stale discovery = %#v, %v", discovery, discoveryErr)
	}

	var plural bytes.Buffer
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &plural)
	if err != nil {
		t.Fatalf("plural stale receipts while disabled were denied instead of reported: %v\n%s", err, plural.String())
	}
	assertDisabledUnmanagedGate(t, nil, plural.Bytes())

	// The report is an observation, so replaying the same request must return
	// the same bytes.
	var replay bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &replay); err != nil {
		t.Fatalf("replayed plural stale receipts while disabled: %v\n%s", err, replay.String())
	}
	if !bytes.Equal(replay.Bytes(), plural.Bytes()) {
		t.Fatalf("plural stale receipts while disabled is not replay stable:\nfirst:\n%s\nreplay:\n%s", plural.String(), replay.String())
	}
}

// TestReviewValidateStaleAndPluralStaleReceiptsFailClosedWhileEnabled is the
// enabled half of TestReviewValidatePluralStaleReceiptsReportDisabledUnmanagedDelivery
// above: the identical alone-then-plural fixture progression, reviews ON
// throughout, pinning the discovery-kind visibility the disabled half no
// longer carries (Wave 5 Slice 2, design decision 4).
func TestReviewValidateStaleAndPluralStaleReceiptsFailClosedWhileEnabled(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	logicalPath := "docs/plural-stale-enabled.md"
	lineageA := "review-enabled-plural-stale-a"
	lineageB := "review-enabled-plural-stale-b"

	_, storeA := approveDiscoveryMarkdownProjection(t, repo, lineageA, logicalPath, "reviewed\n", reviewtransaction.ProjectionWorkspace)
	if err := os.WriteFile(filepath.Join(repo, filepath.FromSlash(logicalPath)), []byte("drifted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// One stale receipt: candidate-or-paths-mismatch, still fails closed.
	var alone bytes.Buffer
	aloneErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &alone)
	var aloneDenied ReviewGateDeniedError
	if !errors.As(aloneErr, &aloneDenied) {
		t.Fatalf("one stale receipt while enabled did not fail closed: %T %v\n%s", aloneErr, aloneErr, alone.String())
	}
	var aloneResult ReviewValidateResult
	decodeStrictReviewJSON(t, alone.Bytes(), &aloneResult)
	if aloneResult.Allowed || aloneResult.Context.Denial == nil || aloneResult.Context.Denial.Code != "candidate-or-paths-mismatch" {
		t.Fatalf("one stale receipt while enabled denial = %#v", aloneResult.Context.Denial)
	}

	// A second, independently-reviewed stale receipt: the pair classifies
	// deterministically-stale-only ambiguous, and still fails closed.
	cloneApprovedDiscoveryAuthority(t, repo, storeA, lineageB)
	var plural bytes.Buffer
	pluralErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &plural)
	var pluralDenied ReviewGateDeniedError
	if !errors.As(pluralErr, &pluralDenied) {
		t.Fatalf("plural stale receipts while enabled did not fail closed: %T %v\n%s", pluralErr, pluralErr, plural.String())
	}
	var pluralResult ReviewValidateResult
	decodeStrictReviewJSON(t, plural.Bytes(), &pluralResult)
	if pluralResult.Allowed || pluralResult.Context.Denial == nil || pluralResult.Context.Denial.Code != string(ReviewReceiptAmbiguous) {
		t.Fatalf("plural stale receipts while enabled denial = %#v", pluralResult.Context.Denial)
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
	finalizeFacadeReviewForRepo(t, repo)
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
