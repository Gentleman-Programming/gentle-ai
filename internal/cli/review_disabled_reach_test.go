package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// The maintainer's rule for this file: while the kill switch is off,
// receipt-driven development does not exist, so it has no implications. The gate
// never blocks, never vetoes, and never gates; ordinary repository policy —
// hooks, tests, CI — decides. Turning reviews back on re-validates from the
// current state instead of resuming a stale obligation.
//
// Nothing here weakens the two invariants that make that safe: a disabled
// switch never fabricates approval (`allowed` stays false and no receipt, PASS,
// or authority is invented), and an unreadable *switch* still fails closed to
// the managed disposition, so a damaged mode record can never manufacture the
// disabled disposition.

// enableReviewForClone clears this clone's off-only override, turning
// receipt-driven development back on for the next candidate.
func enableReviewForClone(t *testing.T, repo string) {
	t.Helper()
	var output bytes.Buffer
	if err := RunReviewMode([]string{"enable", "--cwd", repo, "--scope", "clone", "--json"}, &output); err != nil {
		t.Fatalf("re-enable receipt-driven development: %v\n%s", err, output.String())
	}
	if status := decodeReviewModeResult(t, output.Bytes()).Status; status.Effective != reviewtransaction.RDDModeOn {
		t.Fatalf("kill switch did not come back on: %#v", status)
	}
}

// approveTwoExactlyGoverningReceipts stages the composition discovery reports
// as ReviewReceiptAmbiguous with DeterministicallyStaleOnly false: two terminal
// receipts that each EXACTLY govern the identical live candidate.
func approveTwoExactlyGoverningReceipts(t *testing.T, repo string) []string {
	t.Helper()
	lineageA := "review-disabled-reach-exact-a"
	lineageB := "review-disabled-reach-exact-b"
	_, storeA := approveDiscoveryMarkdownProjection(t, repo, lineageA, "docs/exact.md", "exact\n", reviewtransaction.ProjectionWorkspace)
	cloneApprovedDiscoveryAuthority(t, repo, storeA, lineageB)
	return []string{lineageA, lineageB}
}

// corruptReviewAuthorityInventory writes a truncated compact record, which is
// genuine damage to the review authority store rather than a stale receipt.
func corruptReviewAuthorityInventory(t *testing.T, repo string) {
	t.Helper()
	broken := filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "v2", "corrupt-reach")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "review-state.json"), []byte("{\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// assertDisabledUnmanagedGate is the single shape EVERY reclassified discovery
// outcome must produce while disabled (Wave 5 Slice 2, design decision 4):
// exit 0, no denial error, no approval, no invented receipt, the fixed
// generic disabled reason (reviewDisabledUnmanagedReason, byte-identical
// regardless of what authority-store damage or ambiguity exists — discovery
// never runs while disabled, so there is nothing scenario-specific to carry),
// and no Denial/discovery-kind detail at all. The discovery-kind-specific
// visibility this helper used to assert (a `wantCode` parameter) moved to
// each scenario's "...WhileEnabled" sibling test, where discovery genuinely
// still runs and the property is still true.
func assertDisabledUnmanagedGate(t *testing.T, runErr error, payload []byte) ReviewValidateResult {
	t.Helper()
	if runErr != nil {
		t.Fatalf("disabled gate vetoed delivery instead of reporting it: %v\n%s", runErr, string(payload))
	}
	var denied ReviewGateDeniedError
	if errors.As(runErr, &denied) {
		t.Fatalf("disabled gate reported a denial: %#v", denied)
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, payload, &result)
	if result.Delivery != reviewtransaction.RDDDeliveryDisabledUnmanaged {
		t.Fatalf("disabled gate delivery = %q, want %q\n%s", result.Delivery, reviewtransaction.RDDDeliveryDisabledUnmanaged, string(payload))
	}
	if result.Allowed || result.Result == reviewtransaction.GateAllow {
		t.Fatalf("disabled gate fabricated an approval: %#v", result)
	}
	if result.Context.Denial != nil {
		t.Fatalf("disabled gate leaked discovery-kind detail: %#v (design decision 4: no discovery kind while disabled)", result.Context.Denial)
	}
	if result.Reason != reviewDisabledUnmanagedReason {
		t.Fatalf("disabled gate reason = %q, want the fixed generic sentence %q", result.Reason, reviewDisabledUnmanagedReason)
	}
	return result
}

// TestReviewValidateReportsDisabledUnmanagedDeliveryOverMultipleExactReceipts
// closes the first hole the maintainer's rule leaves open. Two receipts that
// each exactly govern the candidate used to fail closed even with reviews off,
// on the argument that reclassifying "would risk silently choosing" between
// them. Nothing is chosen here: the gate declines to manage, emits no lineage
// and no receipt, and ordinary repository policy decides.
func TestReviewValidateReportsDisabledUnmanagedDeliveryOverMultipleExactReceipts(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	_ = approveTwoExactlyGoverningReceipts(t, repo)

	disableReviewForClone(t, repo)

	gateInput := reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePostApply}
	_, _, discoveryErr := discoverCompactFacadeGateReview(context.Background(), repo, "", gateInput)
	var discovery *ReviewReceiptDiscoveryError
	if !errors.As(discoveryErr, &discovery) || discovery.Kind != ReviewReceiptAmbiguous || discovery.DeterministicallyStaleOnly {
		t.Fatalf("multiple exact receipts discovery = %#v, %v, want ambiguous and NOT deterministically stale", discovery, discoveryErr)
	}

	var output bytes.Buffer
	runErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &output)
	// Wave 5 Slice 2 (design decision 4): the competing-lineage detail this
	// disabled report used to carry moved to the enabled-mode sibling
	// TestReviewValidateKeepsFailingClosedOnMultipleExactReceiptsWhileEnabled
	// below, where discovery genuinely still runs. While disabled, discovery
	// never runs at all, so there is nothing scenario-specific left to name.
	assertDisabledUnmanagedGate(t, runErr, output.Bytes())

	var replay bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &replay); err != nil {
		t.Fatalf("replayed disabled gate: %v\n%s", err, replay.String())
	}
	if !bytes.Equal(replay.Bytes(), output.Bytes()) {
		t.Fatalf("disabled gate is not replay stable:\nfirst:\n%s\nreplay:\n%s", output.String(), replay.String())
	}
}

// TestReviewValidateKeepsFailingClosedOnMultipleExactReceiptsWhileEnabled is
// the regression that matters most: with reviews ON, two exactly-governing
// receipts still block, byte-for-byte as before, and the gate still refuses to
// pick for the caller.
//
// Wave 5 Slice 2 supersession: this is also now the sole home of the
// competing-lineage-names-visible property that
// TestReviewValidateReportsDisabledUnmanagedDeliveryOverMultipleExactReceipts
// above used to assert on the DISABLED half too (removed there — design
// decision 4's "no discovery kind while disabled" means that half no longer
// runs discovery at all, so it has nothing to name). The property survives
// here because discovery genuinely still runs while reviews are ON.
func TestReviewValidateKeepsFailingClosedOnMultipleExactReceiptsWhileEnabled(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	lineages := approveTwoExactlyGoverningReceipts(t, repo)

	var output bytes.Buffer
	runErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &output)
	var denied ReviewGateDeniedError
	if !errors.As(runErr, &denied) {
		t.Fatalf("multiple exact receipts while enabled did not fail closed: %T %v\n%s", runErr, runErr, output.String())
	}
	if fields := strictReviewJSONFields(t, output.Bytes()); !reflectDeepEqualStrings(fields, wantEnabledReviewGateFields) {
		t.Fatalf("enabled gate fields = %v, want %v", fields, wantEnabledReviewGateFields)
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Delivery != "" {
		t.Fatalf("an enabled switch reported a delivery disposition: %#v", result)
	}
	if result.Allowed || result.Context.Denial == nil || result.Context.Denial.Code != string(ReviewReceiptAmbiguous) {
		t.Fatalf("enabled multiple-exact denial = %#v", result)
	}
	if !strings.Contains(result.Reason, "require explicit target selection") {
		t.Fatalf("enabled multiple-exact reason = %q", result.Reason)
	}
	for _, lineage := range lineages {
		if !strings.Contains(result.Reason, lineage) {
			t.Fatalf("enabled multiple-exact reason dropped %q: %q", lineage, result.Reason)
		}
	}
}

// TestReviewValidateReportsDisabledUnmanagedDeliveryOverCorruptedAuthority is
// the deliberate decision on damaged authority. Corrupted review authority is
// damage to a system the operator switched off; blocking an ordinary commit on
// it is exactly the implication the rule removes. So the gate defers.
//
// Wave 5 Slice 2 supersession (design decision 4): the switch is now
// consulted BEFORE any authority read, so this disabled report no longer even
// discovers the corruption — "visible, not silent: the damage is named" moved
// to TestReviewValidateKeepsFailingClosedOnCorruptedAuthorityWhileEnabled
// below, where discovery genuinely still runs. Re-enabling still rediscovers
// the same damage and blocks, so nothing is forgiven, only deferred; while
// disabled, the damage simply never gets read in the first place.
func TestReviewValidateReportsDisabledUnmanagedDeliveryOverCorruptedAuthority(t *testing.T) {
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
	corruptReviewAuthorityInventory(t, repo)

	var output bytes.Buffer
	runErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &output)
	assertDisabledUnmanagedGate(t, runErr, output.Bytes())
}

// TestReviewValidateKeepsFailingClosedOnCorruptedAuthorityWhileEnabled is the
// enabled half: with reviews on, damaged authority still fails closed exactly
// as before, and still names the damage in the reason -- visible, not silent
// (Wave 5 Slice 2: this is now the sole home of that visibility property; the
// disabled half above no longer discovers the corruption at all).
func TestReviewValidateKeepsFailingClosedOnCorruptedAuthorityWhileEnabled(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("reviewed candidate behavior\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finalizeFacadeReviewForRepo(t, repo)
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "reviewed candidate")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("second unreviewed commit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "second unreviewed commit")
	corruptReviewAuthorityInventory(t, repo)

	var output bytes.Buffer
	runErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &output)
	var denied ReviewGateDeniedError
	if !errors.As(runErr, &denied) {
		t.Fatalf("corrupted authority while enabled did not fail closed: %T %v\n%s", runErr, runErr, output.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Delivery != "" {
		t.Fatalf("an enabled switch reported a delivery disposition: %#v", result)
	}
	if result.Allowed || result.Context.Denial == nil {
		t.Fatalf("enabled denial over unreviewed commits = %#v", result)
	}
	// The denial is about THIS candidate, not about an unrelated damaged
	// entry sitting in the same shared store. Two unreviewed commits have no
	// receipt, and that is what the gate must say; borrowing a corruption
	// verdict from history would describe a repository the operator does not
	// have and name no action they can take.
	if result.Context.Denial.Code == string(ReviewAuthorityCorrupted) {
		t.Fatalf("an unrelated damaged entry was reported as this candidate's corruption: %#v", result)
	}
	// Visible, not silent: the damage is still named where it belongs.
	var inspection bytes.Buffer
	if err := RunReviewInspectAuthority([]string{"--cwd", repo}, &inspection); err != nil {
		t.Fatalf("inspect-authority over a damaged store: %v\n%s", err, inspection.String())
	}
	if !strings.Contains(inspection.String(), "corrupt-reach") {
		t.Fatalf("the damaged entry vanished from inspection:\n%s", inspection.String())
	}
}

// TestReviewValidateNegotiatedContractReportsDisabledUnmanagedDelivery covers
// the reach hole an agent hits: the whole disabled-unmanaged escape used to sit
// behind `if !negotiated`, so the identical repository that exits 0 for a human
// exited 1 for any caller driving the negotiated contract.
func TestReviewValidateNegotiatedContractReportsDisabledUnmanagedDelivery(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	logicalPath := "docs/negotiated-stale.md"
	_, storeA := approveDiscoveryMarkdownProjection(t, repo, "review-disabled-reach-negotiated-a", logicalPath, "reviewed\n", reviewtransaction.ProjectionWorkspace)
	cloneApprovedDiscoveryAuthority(t, repo, storeA, "review-disabled-reach-negotiated-b")
	if err := os.WriteFile(filepath.Join(repo, filepath.FromSlash(logicalPath)), []byte("drifted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	disableReviewForClone(t, repo)

	var output bytes.Buffer
	runErr := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &output)
	if runErr != nil {
		t.Fatalf("negotiated disabled gate vetoed delivery instead of reporting it: %v\n%s", runErr, output.String())
	}
	envelope := decodeReviewOperationEnvelope(t, output.Bytes())
	if err := envelope.Validate(); err != nil {
		t.Fatalf("negotiated disabled gate envelope: %v\n%s", err, output.String())
	}
	if envelope.Operation != ReviewIntegrationOperationValidate {
		t.Fatalf("negotiated disabled gate operation = %q", envelope.Operation)
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, envelope.Result, &result)
	if result.Delivery != reviewtransaction.RDDDeliveryDisabledUnmanaged {
		t.Fatalf("negotiated disabled gate delivery = %q, want %q\n%s", result.Delivery, reviewtransaction.RDDDeliveryDisabledUnmanaged, output.String())
	}
	if result.Allowed || result.Result == reviewtransaction.GateAllow {
		t.Fatalf("negotiated disabled gate fabricated an approval: %#v", result)
	}
}

// TestReviewValidateNegotiatedContractKeepsFailingClosedWhileEnabled is the
// negotiated regression: with reviews on, the same repository still produces
// the typed failure envelope and a non-zero exit.
func TestReviewValidateNegotiatedContractKeepsFailingClosedWhileEnabled(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	logicalPath := "docs/negotiated-stale.md"
	_, storeA := approveDiscoveryMarkdownProjection(t, repo, "review-enabled-reach-negotiated-a", logicalPath, "reviewed\n", reviewtransaction.ProjectionWorkspace)
	cloneApprovedDiscoveryAuthority(t, repo, storeA, "review-enabled-reach-negotiated-b")
	if err := os.WriteFile(filepath.Join(repo, filepath.FromSlash(logicalPath)), []byte("drifted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	runErr := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &output)
	if runErr == nil {
		t.Fatalf("negotiated enabled gate did not fail closed:\n%s", output.String())
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Code != string(ReviewReceiptAmbiguous) {
		t.Fatalf("negotiated enabled gate failure = %#v", failure)
	}
}

// TestReviewValidateReportsDisabledUnmanagedDeliveryOverMixedCompactAndLegacyAuthority
// covers the last untyped blocker: a compact v2 receipt that exactly governs,
// contested by a terminal legacy v1 chain that also exactly governs. It stayed
// outside the disposition machinery on the argument that reclassifying would
// mean "silently picking one authority system over the other". While disabled
// neither is picked — the gate reports that it is not governing at all.
//
// Wave 5 Slice 2 supersession (design decision 4): the switch is consulted
// BEFORE any authority read, so this disabled report no longer discovers the
// contest at all, and the "compact v2 and legacy v1" detail it used to name
// moved to review_receipt_discovery_test.go's
// TestUnqualifiedGateDiscoveryOnMixedCompactAndLegacyAuthorityHonorsTheKillSwitch,
// whose enabled half asserts the same message (errReviewMixedCompactLegacyAuthority
// is returned directly while enabled, since discovery genuinely still runs).
func TestReviewValidateReportsDisabledUnmanagedDeliveryOverMixedCompactAndLegacyAuthority(t *testing.T) {
	reviewEnabledHome(t)
	fixture := newLegacyCLIFixture(t, "review-disabled-reach-mixed-legacy")
	finalizeFacadeReviewForRepo(t, fixture.repo)

	disableReviewForClone(t, fixture.repo)

	var output bytes.Buffer
	runErr := RunReviewFacadeValidate([]string{
		"--cwd", fixture.repo, "--gate", string(reviewtransaction.GatePostApply),
	}, &output)
	assertDisabledUnmanagedGate(t, runErr, output.Bytes())
}

// TestReviewValidateReValidatesFromScratchAfterReEnabling proves the second
// half of the rule: nothing that happened while disabled may be treated as
// approved afterwards. The candidate changes during the disabled window, and
// once reviews are back on the gate blocks, and a fresh START re-freezes the
// CURRENT candidate instead of resuming the pre-disable obligation.
func TestReviewValidateReValidatesFromScratchAfterReEnabling(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("reviewed candidate behavior\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	beforeTarget := liveFacadeSnapshotIdentity(t, repo)
	finalizeApprovedFacadeReview(t, repo, "review-before-disabling")
	runReviewCLIGit(t, repo, "add", "tracked.txt")

	var approved bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &approved); err != nil {
		t.Fatalf("receipt-governed gate before disabling: %v\n%s", err, approved.String())
	}
	assertReviewGateResult(t, approved.Bytes(), reviewtransaction.GateAllow)

	disableReviewForClone(t, repo)

	// Work authored entirely inside the disabled window.
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("changed while reviews were off\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")

	var disabled bytes.Buffer
	disabledErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &disabled)
	if disabledErr != nil {
		t.Fatalf("disabled gate vetoed work authored while off: %v\n%s", disabledErr, disabled.String())
	}
	var disabledResult ReviewValidateResult
	decodeStrictReviewJSON(t, disabled.Bytes(), &disabledResult)
	if disabledResult.Delivery != reviewtransaction.RDDDeliveryDisabledUnmanaged || disabledResult.Allowed {
		t.Fatalf("disabled gate over changed work = %#v", disabledResult)
	}

	enableReviewForClone(t, repo)

	// Re-enabling must re-validate: the pre-disable receipt is content-bound to
	// bytes that no longer exist, and nothing from the disabled window carries
	// an approval forward.
	var reEnabled bytes.Buffer
	reEnabledErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &reEnabled)
	if reEnabledErr == nil {
		t.Fatalf("re-enabling inherited an approval for work done while disabled:\n%s", reEnabled.String())
	}
	var inherited ReviewValidateResult
	decodeStrictReviewJSON(t, reEnabled.Bytes(), &inherited)
	if inherited.Allowed || inherited.Result == reviewtransaction.GateAllow {
		t.Fatalf("re-enabled gate approved unreviewed work: %#v", inherited)
	}
	if inherited.Delivery != "" {
		t.Fatalf("re-enabled gate still reported a disabled disposition: %#v", inherited)
	}

	// The pre-disable authority is offered only as an explicit recovery the
	// operator must choose, never as an inherited approval.
	afterStart := startFacadeReviewResult(t, repo, "review-after-re-enabling")
	if afterStart.Action != "recover" || afterStart.LineageID != "review-before-disabling" {
		t.Fatalf("re-enabled START = %#v, want an explicit recovery of the pre-disable lineage", afterStart)
	}
	var stillBlocked bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &stillBlocked); err == nil {
		t.Fatalf("merely starting after re-enabling produced an approval:\n%s", stillBlocked.String())
	}

	// Completing that re-validation is what unblocks delivery, and it does so on
	// a NEW lineage frozen over the CURRENT candidate — never by reusing the
	// pre-disable receipt or its target identity.
	successor := recoverFacadeReview(t, repo, "review-before-disabling", "review-after-re-enabling")
	if successor.TargetIdentity == beforeTarget {
		t.Fatalf("re-validation reused the pre-disable target identity %q", beforeTarget)
	}
	finalizeApprovedFacadeReview(t, repo, successor.LineageID)

	var revalidated bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &revalidated); err != nil {
		t.Fatalf("gate after re-validating from scratch: %v\n%s", err, revalidated.String())
	}
	assertReviewGateResult(t, revalidated.Bytes(), reviewtransaction.GateAllow)
	var fresh ReviewValidateResult
	decodeStrictReviewJSON(t, revalidated.Bytes(), &fresh)
	if fresh.Context.LineageID != successor.LineageID {
		t.Fatalf("re-validated gate bound lineage %q, want the fresh successor %q", fresh.Context.LineageID, successor.LineageID)
	}
}

// recoverFacadeReview drives the explicit maintainer recovery that re-freezes a
// superseded lineage over the current candidate.
func recoverFacadeReview(t *testing.T, repo, predecessor, successorLineage string) ReviewRecoverResult {
	t.Helper()
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, predecessor)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	intended, err := builder.DiscoverUnignoredUntracked(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	target, err := builder.Build(context.Background(), reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: intended})
	if err != nil {
		t.Fatal(err)
	}
	authorization := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + predecessor +
		"\npredecessor_revision=" + record.Revision + "\ntarget_identity=" + target.Identity +
		"\nactor=maintainer\nreason=candidate changed while reviews were off"
	var output bytes.Buffer
	if err := RunReview([]string{
		"recover", "--cwd", repo, "--predecessor-lineage", predecessor,
		"--expected-predecessor-revision", record.Revision, "--successor-lineage", successorLineage,
		"--disposition", "scope_changed", "--reason", "candidate changed while reviews were off",
		"--actor", "maintainer", "--maintainer-authorization", authorization,
	}, &output); err != nil {
		t.Fatalf("recover review after re-enabling: %v\n%s", err, output.String())
	}
	var recovered ReviewRecoverResult
	decodeStrictReviewJSON(t, output.Bytes(), &recovered)
	return recovered
}

// startFacadeReviewResult runs one START over the live candidate and returns
// the typed result.
func startFacadeReviewResult(t *testing.T, repo, lineage string) ReviewFacadeStartResult {
	t.Helper()
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, &output); err != nil {
		t.Fatalf("start facade review %q: %v\n%s", lineage, err, output.String())
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	return started
}

// liveFacadeSnapshotIdentity computes the current workspace candidate's
// snapshot identity WITHOUT persisting any authority (legacy or v3) for it.
// Some fixtures need this identity purely as a later comparison value
// (proving a successor candidate is NOT the same target), and calling it
// where a subsequent finalizeApprovedFacadeReview also creates LEGACY
// authority for the identical lineage id would otherwise leave both a v3
// record (from an ordinary `review start`) and a v2 one (from
// finalizeApprovedFacadeReview's direct construction) for the same lineage
// -- a collision this helper avoids by never persisting anything.
func liveFacadeSnapshotIdentity(t *testing.T, repo string) string {
	t.Helper()
	ctx := context.Background()
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	root, err := builder.ResolveRepositoryRoot(ctx)
	if err != nil {
		t.Fatalf("resolve live snapshot repository root: %v", err)
	}
	rootBuilder := reviewtransaction.SnapshotBuilder{Repo: root}
	snapshot, err := rootBuilder.Build(ctx, reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatalf("build live snapshot identity: %v", err)
	}
	return snapshot.Identity
}

// finalizeApprovedFacadeReview finalizes an already-started LEGACY
// (compact-v2) lineage to a terminal receipt. Every caller of this helper
// specifically needs genuine legacy authority (proven by
// reviewtransaction.CompactAuthoritativeStore/CompactAuthoritativeStore
// reads immediately after several call sites, and by the two
// legacy-vs-v3-precedence tests in review_governing_authority_test.go),
// never merely "some approved review" -- a v3 fixture would not exercise
// what any of these tests actually assert.
//
// Before Wave 7 S7 (WU18), this called the CLI's own `review start` with
// the (default, unset) activation switch off, which took the legacy
// compact-v2 branch. WU18 removed that branch entirely -- `review start` is
// now unconditionally v3, so there is no CLI-reachable way left to create a
// NEW legacy authority (matches gate_boundary_matrix_test.go's own
// disclosed-gap comment: a v1 legacy lineage already had no CLI-reachable
// creation path, and now neither does v2). This constructs the identical
// legacy compact-v2 authority directly through the same
// reviewtransaction API runReviewFacadeStart's now-deleted legacy branch
// used to call, then finalizes it through the unchanged CLI
// RunReviewFacadeFinalize (finalize's discovery-by-kind logic never
// depended on the switch and is untouched by WU18).
func finalizeApprovedFacadeReview(t *testing.T, repo, lineage string) {
	t.Helper()
	ctx := context.Background()
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	root, err := builder.ResolveRepositoryRoot(ctx)
	if err != nil {
		t.Fatalf("resolve legacy facade review repository root: %v", err)
	}
	rootBuilder := reviewtransaction.SnapshotBuilder{Repo: root}
	snapshot, err := rootBuilder.Build(ctx, reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatalf("build legacy facade review target %q: %v", lineage, err)
	}
	assessment, err := rootBuilder.AssessSnapshotRisk(ctx, snapshot)
	if err != nil {
		t.Fatalf("classify legacy facade review target %q: %v", lineage, err)
	}
	lenses, err := facadeSelectedLenses(assessment, "reliability")
	if err != nil {
		t.Fatalf("select legacy facade review lenses %q: %v", lineage, err)
	}
	policy, err := facadePolicyBytes("")
	if err != nil {
		t.Fatalf("read legacy facade review policy %q: %v", lineage, err)
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: facadePayloadHash(policy), RiskLevel: assessment.Level,
		SelectedLenses: lenses, OriginalChangedLines: &assessment.ChangedLines,
	})
	if err != nil {
		t.Fatalf("create legacy facade review state %q: %v", lineage, err)
	}
	compactStarted, err := reviewtransaction.StartCompactAuthority(ctx, root, reviewtransaction.CompactStartRequest{
		State: state, ExplicitLineage: true,
	})
	if err != nil {
		t.Fatalf("start legacy compact authority %q: %v", lineage, err)
	}
	// reviewFacadeStartResultFor is the same production helper
	// runReviewFacadeStart's own (now-deleted) legacy branch used to shape
	// this exact result from a StartCompactAuthority call -- reused here
	// rather than calling the CLI a second time, which would now hit the
	// unconditional start path's own read-only guard against an existing
	// legacy chain (by design: v3 must never be created over live legacy
	// authority for the same lineage id).
	started := reviewFacadeStartResultFor(compactStarted.Action, compactStarted.LensesRequired, compactStarted.Record.State)
	args := []string{"--cwd", repo, "--lineage", started.LineageID}
	if len(started.SelectedLenses) != 0 {
		evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
		if err := os.WriteFile(evidencePath, []byte("focused tests pass\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		args = append(args, facadeReviewerResultArgs(t, repo, started)...)
		args = append(args, "--evidence", evidencePath)
	}
	if err := RunReviewFacadeFinalize(args, io.Discard); err != nil {
		t.Fatalf("finalize facade review %q: %v", lineage, err)
	}
}

// TestDisabledGateNeverEmitsAllowOrCreatesReceipt sweeps every reclassified
// discovery outcome and holds the non-negotiable boundary: `disabled/unmanaged`
// exits 0 because it defers, never because it approved.
//
// Wave 5 Slice 2 (design decision 4): also the decoy-store zero-reads proof.
// These three cases stage wildly different, even damaged, authority-store
// content -- an ambiguous multi-receipt composition, a genuinely corrupted
// (truncated) compact record, and no receipt at all -- behind the SAME gate.
// Under the pre-Slice-2 contract these produced three DIFFERENT disabled
// reasons (each naming its own discovery kind). Under the new contract the
// switch is consulted before any of that content is ever read, so all three
// must produce BYTE-IDENTICAL disabled output: the store's content
// contributes nothing to what is reported, which is the portable,
// CI-safe equivalent of Wave 4's own strace-based "zero authority-store
// opens while disabled" verifier proof.
func TestDisabledGateNeverEmitsAllowOrCreatesReceipt(t *testing.T) {
	cases := []struct {
		name  string
		stage func(t *testing.T, repo string)
	}{
		{
			name:  "multiple exactly governing receipts",
			stage: func(t *testing.T, repo string) { approveTwoExactlyGoverningReceipts(t, repo) },
		},
		{
			name: "corrupted authority",
			stage: func(t *testing.T, repo string) {
				approveDiscoveryMarkdownProjection(t, repo, "review-disabled-sweep-corrupt", "docs/sweep.md", "reviewed\n", reviewtransaction.ProjectionWorkspace)
				corruptReviewAuthorityInventory(t, repo)
			},
		},
		{
			name:  "no receipt at all",
			stage: func(t *testing.T, repo string) {},
		},
	}
	outputs := make(map[string][]byte, len(cases))
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			testCase.stage(t, repo)
			disableReviewForClone(t, repo)
			before := reviewAuthorityFingerprint(t, repo)

			var output bytes.Buffer
			runErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &output)
			assertDisabledUnmanagedGate(t, runErr, output.Bytes())
			if bytes.Contains(output.Bytes(), []byte(`"allow"`)) {
				t.Fatalf("disabled gate emitted an allow: %s", output.String())
			}
			if after := reviewAuthorityFingerprint(t, repo); after != before {
				t.Fatalf("disabled gate mutated review authority:\nbefore: %s\nafter:  %s", before, after)
			}
			outputs[testCase.name] = append([]byte(nil), output.Bytes()...)
		})
	}
	first := outputs[cases[0].name]
	for _, testCase := range cases[1:] {
		if !bytes.Equal(outputs[testCase.name], first) {
			t.Fatalf("decoy-store zero-reads proof failed: disabled output differs by store content\n%s:\n%s\n%s:\n%s",
				cases[0].name, first, testCase.name, outputs[testCase.name])
		}
	}
}

// reviewAuthorityFingerprint lists every file under this clone's review
// authority root so a test can prove a disabled run created no receipt and
// mutated no state.
func reviewAuthorityFingerprint(t *testing.T, repo string) string {
	t.Helper()
	root := filepath.Join(repo, ".git", "gentle-ai", "review-transactions")
	var entries []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		payload, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		entries = append(entries, path+":"+facadePayloadHash(payload))
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return strings.Join(entries, "\n")
}

// reflectDeepEqualStrings keeps this file free of a reflect import for the one
// slice comparison it needs.
func reflectDeepEqualStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

// TestSDDAttemptFinishHonorsTheKillSwitchOverRemediationObligations is the
// end-to-end proof that the switch actually REACHES the SDD runtime ledger. The
// unit test in internal/sddstatus proves the ledger obeys the flag; this proves
// the CLI resolves the switch and sets it, which is the part that was missing
// entirely — internal/sddstatus never consulted the kill switch in any form.
//
// The shape is the reporter's: a clone holds a review binding, work continues,
// the attempt changes the candidate tree, and the passing attempt is closed
// without remediation flags. With reviews ON that still demands an approved
// recovery successor. With reviews OFF it closes, because the successor could
// only come from `review start`, which the same switch refuses.
func TestSDDAttemptFinishImposesNoRemediationObligationEitherWay(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		disable  bool
		wantFail bool
	}{
		{name: "reviews enabled impose no obligation", disable: false, wantFail: false},
		{name: "reviews disabled impose no obligation", disable: true, wantFail: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			reviewModeHome(t)
			repo := initReviewCLIRepo(t)
			change := "cli-kill-switch-remediation"
			changeRoot := filepath.Join(repo, "openspec", "changes", change)
			writeCLIAttemptFile(t, filepath.Join(changeRoot, "proposal.md"), "# Proposal\n")
			writeCLIAttemptFile(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Done\n")
			runReviewCLIGit(t, repo, "add", ".")
			runReviewCLIGit(t, repo, "commit", "-qm", "seed change")

			runSDDAttemptStatus(t, []string{
				"begin", "--cwd", repo, "--change", change, "--expected-revision=", "--request-id", "switch-begin-1",
				"--work-unit", "cli-kill-switch", "--evidence-goal", "close a bound attempt",
				"--max-attempts", "3", "--max-changed-lines", "40",
			})

			lineage := "cli-kill-switch-lineage"
			writeCLIApprovedCompactAuthority(t, repo, lineage)
			if _, err := sddstatus.BindApprovedReview(context.Background(), repo, change, lineage, ""); err != nil {
				t.Fatal(err)
			}
			bound := runSDDAttemptStatus(t, []string{"status", "--cwd", repo, "--change", change})
			if bound.Binding == nil {
				t.Fatalf("bound CLI status = %#v", bound)
			}

			// Work that changes the candidate tree during the attempt: this is
			// exactly what arms the implicit successor demand.
			writeCLIAttemptFile(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Done\n# more work\n")

			if testCase.disable {
				disableReviewForClone(t, repo)
			}

			var output bytes.Buffer
			err := RunSDDAttempt([]string{
				"finish", "--cwd", repo, "--change", change, "--expected-revision", bound.Revision,
				"--request-id", "switch-finish-1", "--outcome", "passed", "--evidence-revision", cliAttemptHash('c'),
				"--diagnosis", "attempt passed", "--harness-disposition", "reused",
				"--cleanup-evidence", "cleanup completed", "--process-evidence", "process scan found no descendants",
			}, &output)
			if err != nil {
				t.Fatalf("bound finish demanded a review obligation: %T %v\n%s", err, err, output.String())
			}
			var status sddstatus.RuntimeStatus
			decodeStrictReviewJSON(t, output.Bytes(), &status)
			if status.ActiveAttempt != nil {
				t.Fatalf("disabled bound finish left the attempt open: %#v", status.ActiveAttempt)
			}
			if status.Binding == nil || status.Binding.Revision != bound.Binding.Revision {
				t.Fatalf("disabled bound finish mutated the review binding: %#v", status.Binding)
			}
		})
	}
}
