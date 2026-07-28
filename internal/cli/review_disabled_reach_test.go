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
// review-driven development does not exist, so it has no implications. The gate
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
// review-driven development back on for the next candidate.
func enableReviewForClone(t *testing.T, repo string) {
	t.Helper()
	var output bytes.Buffer
	if err := RunReviewMode([]string{"enable", "--cwd", repo, "--scope", "clone", "--json"}, &output); err != nil {
		t.Fatalf("re-enable review-driven development: %v\n%s", err, output.String())
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

// assertDisabledUnmanagedGate is the single shape every reclassified discovery
// outcome must produce while disabled: exit 0, no denial error, no approval, no
// invented receipt, and the reason the gate could not govern still discoverable.
func assertDisabledUnmanagedGate(t *testing.T, runErr error, payload []byte, wantCode string) ReviewValidateResult {
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
	if result.Context.Denial == nil || result.Context.Denial.Code != wantCode {
		t.Fatalf("disabled gate denial = %#v, want code %q", result.Context.Denial, wantCode)
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
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	lineages := approveTwoExactlyGoverningReceipts(t, repo)

	disableReviewForClone(t, repo)

	gateInput := reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePostApply}
	_, _, discoveryErr := discoverCompactFacadeGateReview(context.Background(), repo, "", gateInput)
	var discovery *ReviewReceiptDiscoveryError
	if !errors.As(discoveryErr, &discovery) || discovery.Kind != ReviewReceiptAmbiguous || discovery.DeterministicallyStaleOnly {
		t.Fatalf("multiple exact receipts discovery = %#v, %v, want ambiguous and NOT deterministically stale", discovery, discoveryErr)
	}

	var output bytes.Buffer
	runErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &output)
	result := assertDisabledUnmanagedGate(t, runErr, output.Bytes(), string(ReviewReceiptAmbiguous))

	// Information must not be destroyed: not blocking is not the same as
	// pretending nothing was ambiguous, so the competing lineages stay named.
	for _, lineage := range lineages {
		if !strings.Contains(result.Reason, lineage) {
			t.Fatalf("disabled gate dropped the competing lineage %q from its reason: %q", lineage, result.Reason)
		}
	}

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
func TestReviewValidateKeepsFailingClosedOnMultipleExactReceiptsWhileEnabled(t *testing.T) {
	reviewModeHome(t)
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
// it is exactly the implication the rule removes. So the gate defers — and says
// so loudly rather than silently: the denial code stays `authority_corrupted`
// and the reason names the damage, so the user is told without being stopped.
// Re-enabling rediscovers the same damage and blocks again, so nothing is
// forgiven, only deferred.
func TestReviewValidateReportsDisabledUnmanagedDeliveryOverCorruptedAuthority(t *testing.T) {
	reviewModeHome(t)
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
	result := assertDisabledUnmanagedGate(t, runErr, output.Bytes(), string(ReviewAuthorityCorrupted))
	// Visible, not silent: the damage is named in the reported reason.
	if !strings.Contains(result.Reason, "unavailable or corrupted") {
		t.Fatalf("disabled corrupted-authority reason hid the damage: %q", result.Reason)
	}
}

// TestReviewValidateKeepsFailingClosedOnCorruptedAuthorityWhileEnabled is the
// enabled half: with reviews on, damaged authority still fails closed exactly
// as before.
func TestReviewValidateKeepsFailingClosedOnCorruptedAuthorityWhileEnabled(t *testing.T) {
	reviewModeHome(t)
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
	if result.Allowed || result.Context.Denial == nil || result.Context.Denial.Code != string(ReviewAuthorityCorrupted) {
		t.Fatalf("enabled corrupted-authority denial = %#v", result)
	}
}

// TestReviewValidateNegotiatedContractReportsDisabledUnmanagedDelivery covers
// the reach hole an agent hits: the whole disabled-unmanaged escape used to sit
// behind `if !negotiated`, so the identical repository that exits 0 for a human
// exited 1 for any caller driving the negotiated contract.
func TestReviewValidateNegotiatedContractReportsDisabledUnmanagedDelivery(t *testing.T) {
	reviewModeHome(t)
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
	reviewModeHome(t)
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
func TestReviewValidateReportsDisabledUnmanagedDeliveryOverMixedCompactAndLegacyAuthority(t *testing.T) {
	fixture := newLegacyCLIFixture(t, "review-disabled-reach-mixed-legacy")
	finalizeFacadeReviewForRepo(t, fixture.repo)

	disableReviewForClone(t, fixture.repo)

	var output bytes.Buffer
	runErr := RunReviewFacadeValidate([]string{
		"--cwd", fixture.repo, "--gate", string(reviewtransaction.GatePostApply),
	}, &output)
	result := assertDisabledUnmanagedGate(t, runErr, output.Bytes(), string(ReviewReceiptAmbiguous))
	if !strings.Contains(result.Reason, "compact v2 and legacy v1") {
		t.Fatalf("disabled mixed-authority reason hid the competing stores: %q", result.Reason)
	}
}

// TestReviewValidateReValidatesFromScratchAfterReEnabling proves the second
// half of the rule: nothing that happened while disabled may be treated as
// approved afterwards. The candidate changes during the disabled window, and
// once reviews are back on the gate blocks, and a fresh START re-freezes the
// CURRENT candidate instead of resuming the pre-disable obligation.
func TestReviewValidateReValidatesFromScratchAfterReEnabling(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("reviewed candidate behavior\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	beforeTarget := startFacadeReviewTargetIdentity(t, repo, "review-before-disabling")
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
	intended, err := builder.DiscoverIntendedUntracked(context.Background())
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

func startFacadeReviewTargetIdentity(t *testing.T, repo, lineage string) string {
	t.Helper()
	return startFacadeReviewResult(t, repo, lineage).TargetIdentity
}

// finalizeApprovedFacadeReview finalizes an already-started lineage to a
// terminal receipt.
func finalizeApprovedFacadeReview(t *testing.T, repo, lineage string) {
	t.Helper()
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, &output); err != nil {
		t.Fatalf("resume facade review %q: %v\n%s", lineage, err, output.String())
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
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
func TestDisabledGateNeverEmitsAllowOrCreatesReceipt(t *testing.T) {
	cases := []struct {
		name  string
		stage func(t *testing.T, repo string)
		code  string
	}{
		{
			name:  "multiple exactly governing receipts",
			stage: func(t *testing.T, repo string) { approveTwoExactlyGoverningReceipts(t, repo) },
			code:  string(ReviewReceiptAmbiguous),
		},
		{
			name: "corrupted authority",
			stage: func(t *testing.T, repo string) {
				approveDiscoveryMarkdownProjection(t, repo, "review-disabled-sweep-corrupt", "docs/sweep.md", "reviewed\n", reviewtransaction.ProjectionWorkspace)
				corruptReviewAuthorityInventory(t, repo)
			},
			code: string(ReviewAuthorityCorrupted),
		},
		{
			name:  "no receipt at all",
			stage: func(t *testing.T, repo string) {},
			code:  string(ReviewReceiptMissing),
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			reviewModeHome(t)
			repo := initReviewCLIRepo(t)
			testCase.stage(t, repo)
			disableReviewForClone(t, repo)
			before := reviewAuthorityFingerprint(t, repo)

			var output bytes.Buffer
			runErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePostApply)}, &output)
			assertDisabledUnmanagedGate(t, runErr, output.Bytes(), testCase.code)
			if bytes.Contains(output.Bytes(), []byte(`"allow"`)) {
				t.Fatalf("disabled gate emitted an allow: %s", output.String())
			}
			if after := reviewAuthorityFingerprint(t, repo); after != before {
				t.Fatalf("disabled gate mutated review authority:\nbefore: %s\nafter:  %s", before, after)
			}
		})
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
func TestSDDAttemptFinishHonorsTheKillSwitchOverRemediationObligations(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		disable  bool
		wantFail bool
	}{
		{name: "reviews enabled still demand a successor", disable: false, wantFail: true},
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
			if testCase.wantFail {
				if !errors.Is(err, sddstatus.ErrRuntimeRemediationSuccessorRequired) {
					t.Fatalf("enabled bound finish = %T %v, want the successor demand\n%s", err, err, output.String())
				}
				return
			}
			if err != nil {
				t.Fatalf("disabled bound finish demanded a review obligation: %T %v\n%s", err, err, output.String())
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
