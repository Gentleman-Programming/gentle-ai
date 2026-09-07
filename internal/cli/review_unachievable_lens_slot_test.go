package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// reviewTransitionArgumentValue finds one argument's value by name inside a
// rendered collect input, exactly as a host reading the negotiated envelope
// would.
func reviewTransitionArgumentValue(t *testing.T, input ReviewTransitionInput, name string) string {
	t.Helper()
	for _, argument := range input.Arguments {
		if argument.Name == name {
			return argument.Value
		}
	}
	t.Fatalf("collect input %q carries no %q argument", input.Name, name)
	return ""
}

// TestReviewCaptureUnachievableLensSlotStopsStatusFromReofferingIt is the
// behavior-first regression for issue #3442. Reproduction: a host that
// discovers a bound selected lens slot cannot be completed (for example a
// reviewer prompt exceeding a host relay's transport bound) had no way to
// tell Go, so a restarted negotiated STATUS kept re-offering the identical
// slot as though nothing had happened. This proves the fix end to end: once
// the host reports the slot unachievable through `review capture-unachievable`,
// STATUS stops re-offering it, reports the distinct `unachievable_lens_slot`
// stop instead of contradicting the report with a fresh collect, and a
// restarted STATUS negotiation returns the same answer (restart-safety).
func TestReviewCaptureUnachievableLensSlotStopsStatusFromReofferingIt(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started := startHighRiskCLIReview(t, repo)

	statusNextTransitionRaw := func() ([]byte, ReviewTargetStatusResult) {
		var output bytes.Buffer
		if err := RunReview([]string{
			"status", "--cwd", repo, "--lineage", started.LineageID,
			"--contract", ReviewIntegrationContractV2, "--next-transition",
		}, &output); err != nil {
			t.Fatalf("STATUS --next-transition: %v\n%s", err, output.String())
		}
		var status ReviewTargetStatusResult
		decodeStrictReviewJSON(t, output.Bytes(), &status)
		return output.Bytes(), status
	}
	statusNextTransition := func() ReviewTargetStatusResult {
		_, status := statusNextTransitionRaw()
		return status
	}

	// Reproduce the gap first: the untouched slot is offered as an ordinary
	// reviewer_result collect input, exactly like every other outstanding
	// lens.
	before := statusNextTransition()
	if before.NextTransition == nil || before.NextTransition.Kind != reviewNextTransitionCollect ||
		before.NextTransition.ReasonCode != "reviewer_results_required" ||
		before.NextTransition.Collect == nil || len(before.NextTransition.Collect.Inputs) != len(started.SelectedLenses) {
		t.Fatalf("STATUS before any capture = %#v", before)
	}
	pending := before.NextTransition.Collect.Inputs[0]
	if pending.ArtifactSubject == nil || pending.ArtifactSubject.SubjectHash == "" {
		t.Fatalf("pending collect input carries no artifact subject: %#v", pending)
	}
	lineage := reviewTransitionArgumentValue(t, pending, "lineage")
	target := reviewTransitionArgumentValue(t, pending, "target")
	revision := reviewTransitionArgumentValue(t, pending, "expected-revision")
	subjectHash := pending.ArtifactSubject.SubjectHash

	// The host reports the exact offered slot unachievable, bound to the
	// same lineage/revision/target/subject the collect input just handed it
	// -- the same argument shape every other capture verb uses.
	var captureOutput bytes.Buffer
	if err := RunReview([]string{
		"capture-unachievable", "--cwd", repo, "--lineage", lineage, "--target", target,
		"--expected-revision", revision, "--request-hash", subjectHash,
		"--reason", "relay_transport_bound_exceeded", "--detail", "elapsed 42s against a 30s host relay bound",
	}, &captureOutput); err != nil {
		t.Fatalf("review capture-unachievable: %v\n%s", err, captureOutput.String())
	}
	var artifact reviewUnachievableLensCaptureArtifact
	decodeStrictReviewJSON(t, captureOutput.Bytes(), &artifact)
	if !artifact.Recorded || artifact.Lens != started.SelectedLenses[0] || artifact.SelectedOrder != 0 {
		t.Fatalf("capture-unachievable artifact = %#v", artifact)
	}

	// STATUS must never re-offer the identical slot after this: it stops
	// with a distinct, truthful reason instead of contradicting the host's
	// own report. The stop must also carry the exact recoverable withdraw
	// binding for the declared slot -- a restarted orchestrator that lost
	// the pre-stop collect offer above has no other native route back to
	// the SubjectHash its own retraction needs.
	afterRaw, after := statusNextTransitionRaw()
	if after.NextTransition == nil || after.NextTransition.Kind != reviewNextTransitionStop ||
		after.NextTransition.ReasonCode != "unachievable_lens_slot" {
		t.Fatalf("STATUS after capture-unachievable = %#v, want a stop reasoned unachievable_lens_slot", after.NextTransition)
	}
	if after.NextTransition.Collect != nil {
		t.Fatalf("a typed stop must never also carry a collect offer: %#v", after.NextTransition.Collect)
	}
	validatePublishedReviewSchema(t, compileWholeNativeStatusSchema(t, "status-v7.schema.json"), afterRaw)
	if after.NextTransition.UnachievableLensSlots == nil || len(*after.NextTransition.UnachievableLensSlots) != 1 {
		t.Fatalf("stop carries %#v, want exactly one recoverable declared slot", after.NextTransition.UnachievableLensSlots)
	}
	declaredSlot := (*after.NextTransition.UnachievableLensSlots)[0]
	if declaredSlot.Lens != started.SelectedLenses[0] || declaredSlot.SelectedOrder != 0 || declaredSlot.SubjectHash != subjectHash ||
		declaredSlot.Reason != "relay_transport_bound_exceeded" || declaredSlot.Withdraw.Operation != reviewCaptureUnachievableCaptureOperation ||
		!strings.HasPrefix(declaredSlot.Withdraw.Command, "gentle-ai review capture-unachievable ") {
		t.Fatalf("declared slot on the stop = %#v", declaredSlot)
	}

	// Restart-safety: a fresh negotiated STATUS call (simulating a restarted
	// orchestrator with no in-memory state) returns the identical answer,
	// because the declaration is durable compact authority, not a
	// process-local fact.
	restarted := statusNextTransition()
	if restarted.NextTransition == nil || restarted.NextTransition.Kind != reviewNextTransitionStop ||
		restarted.NextTransition.ReasonCode != "unachievable_lens_slot" {
		t.Fatalf("restarted STATUS = %#v, want the same unachievable_lens_slot stop", restarted.NextTransition)
	}

	// The compact authority itself durably records the declaration, bound to
	// the exact slot, so a maintainer inspecting the store sees the same
	// truthful answer STATUS reported.
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(record.State.UnachievableLensAttempts) != 1 {
		t.Fatalf("compact authority carries %d unachievable lens attempts, want 1", len(record.State.UnachievableLensAttempts))
	}
	recordedAttempt := record.State.UnachievableLensAttempts[0]
	if recordedAttempt.Lens != started.SelectedLenses[0] || recordedAttempt.SelectedOrder != 0 ||
		recordedAttempt.SubjectHash != subjectHash || recordedAttempt.Reason != "relay_transport_bound_exceeded" {
		t.Fatalf("recorded unachievable attempt = %#v", recordedAttempt)
	}

	// The declaration must not silently approve anything: the compact
	// authority is still reviewing, never approved, and no receipt exists.
	if record.State.State != reviewtransaction.StateReviewing {
		t.Fatalf("compact authority state = %q, want it to remain reviewing (never silently approved)", record.State.State)
	}

	// The resilience half of the fix: a host that mistook a transient
	// failure for a deterministic one is not stuck. The pre-stop lineage,
	// target, revision, and subjectHash variables are deliberately never
	// referenced again below: everything the withdrawal needs comes only
	// from declaredSlot, exactly as a restarted orchestrator that lost the
	// original collect offer would have to recover it -- purely from this
	// stop's own STATUS output.
	withdrawArgs := []string{"capture-unachievable", "--cwd", repo}
	for _, argument := range declaredSlot.Withdraw.Arguments {
		withdrawArgs = append(withdrawArgs, argument.Token)
	}
	var withdrawOutput bytes.Buffer
	if err := RunReview(withdrawArgs, &withdrawOutput); err != nil {
		t.Fatalf("review capture-unachievable --withdraw=true (recovered from the stop): %v\n%s", err, withdrawOutput.String())
	}
	var withdrawal reviewUnachievableLensWithdrawalArtifact
	decodeStrictReviewJSON(t, withdrawOutput.Bytes(), &withdrawal)
	if !withdrawal.Withdrawn {
		t.Fatalf("withdrawal artifact = %#v, want Withdrawn=true for a first withdrawal of a standing declaration", withdrawal)
	}

	restoredOffer := statusNextTransition()
	if restoredOffer.NextTransition == nil || restoredOffer.NextTransition.Kind != reviewNextTransitionCollect ||
		restoredOffer.NextTransition.ReasonCode != "reviewer_results_required" ||
		restoredOffer.NextTransition.Collect == nil || len(restoredOffer.NextTransition.Collect.Inputs) != len(started.SelectedLenses) {
		t.Fatalf("STATUS after withdraw = %#v, want the same collect offer restored", restoredOffer.NextTransition)
	}
	restoredSlot := restoredOffer.NextTransition.Collect.Inputs[0]
	if restoredSlot.ArtifactSubject == nil || restoredSlot.ArtifactSubject.SubjectHash != declaredSlot.SubjectHash {
		t.Fatalf("STATUS after withdraw restored a different slot: %#v, want subject hash %q", restoredSlot.ArtifactSubject, declaredSlot.SubjectHash)
	}

	afterWithdraw, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(afterWithdraw.State.UnachievableLensAttempts) != 0 {
		t.Fatalf("compact authority still carries %d unachievable lens attempts after withdraw, want 0", len(afterWithdraw.State.UnachievableLensAttempts))
	}
}

// TestReviewCaptureUnachievableRejectsMismatchedRequestHash proves the
// declaration cannot be used to skip a lens or fabricate a result: a
// request-hash that does not match any outstanding selected slot is refused,
// exactly like a stale binding on every other capture verb.
func TestReviewCaptureUnachievableRejectsMismatchedRequestHash(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started := startHighRiskCLIReview(t, repo)

	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", started.LineageID,
		"--contract", ReviewIntegrationContractV2, "--next-transition",
	}, &output); err != nil {
		t.Fatalf("STATUS --next-transition: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	pending := status.NextTransition.Collect.Inputs[0]
	lineage := reviewTransitionArgumentValue(t, pending, "lineage")
	target := reviewTransitionArgumentValue(t, pending, "target")
	revision := reviewTransitionArgumentValue(t, pending, "expected-revision")

	var captureOutput bytes.Buffer
	err := RunReview([]string{
		"capture-unachievable", "--cwd", repo, "--lineage", lineage, "--target", target,
		"--expected-revision", revision, "--request-hash", "sha256:" + strings.Repeat("0", 64),
		"--reason", "relay_transport_bound_exceeded",
	}, &captureOutput)
	if err == nil {
		t.Fatal("a request-hash matching no outstanding slot was not refused")
	}

	store, storeErr := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	record, loadErr := store.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(record.State.UnachievableLensAttempts) != 0 {
		t.Fatalf("a refused capture-unachievable call mutated the authority: %d attempts recorded", len(record.State.UnachievableLensAttempts))
	}
}

// TestReviewCaptureUnachievableWithdrawIsIdempotentAndRefusesBadCombinations
// is the CLI-level regression for the withdraw path: replaying an exact
// withdrawal after the declaration is already gone succeeds without
// mutating the authority, withdrawing a request-hash that was never
// declared is the same safe no-op, and --withdraw=true refuses to be
// combined with --reason or --detail (a withdrawal carries no new
// evidence).
func TestReviewCaptureUnachievableWithdrawIsIdempotentAndRefusesBadCombinations(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started := startHighRiskCLIReview(t, repo)

	var statusOutput bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", started.LineageID,
		"--contract", ReviewIntegrationContractV2, "--next-transition",
	}, &statusOutput); err != nil {
		t.Fatalf("STATUS --next-transition: %v\n%s", err, statusOutput.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	pending := status.NextTransition.Collect.Inputs[0]
	lineage := reviewTransitionArgumentValue(t, pending, "lineage")
	target := reviewTransitionArgumentValue(t, pending, "target")
	revision := reviewTransitionArgumentValue(t, pending, "expected-revision")
	subjectHash := pending.ArtifactSubject.SubjectHash

	binding := []string{
		"--cwd", repo, "--lineage", lineage, "--target", target, "--expected-revision", revision,
	}

	// --withdraw=true cannot be combined with --reason or --detail.
	if err := RunReview(append(append([]string{"capture-unachievable"}, binding...),
		"--request-hash", subjectHash, "--withdraw=true", "--reason", "relay_transport_bound_exceeded"), io.Discard); err == nil {
		t.Fatal("--withdraw=true combined with --reason was not refused")
	}

	// Withdrawing a request-hash that was never declared is a safe no-op,
	// not an error: the guarantee it promises (nothing blocks this slot)
	// already holds.
	var neverDeclaredOutput bytes.Buffer
	if err := RunReview(append(append([]string{"capture-unachievable"}, binding...),
		"--request-hash", subjectHash, "--withdraw=true"), &neverDeclaredOutput); err != nil {
		t.Fatalf("withdraw of a never-declared slot: %v\n%s", err, neverDeclaredOutput.String())
	}
	var neverDeclared reviewUnachievableLensWithdrawalArtifact
	decodeStrictReviewJSON(t, neverDeclaredOutput.Bytes(), &neverDeclared)
	if neverDeclared.Withdrawn {
		t.Fatalf("withdraw of a never-declared slot reported Withdrawn=true: %#v", neverDeclared)
	}

	// Declare, then withdraw, then replay the exact same withdrawal: the
	// replay must succeed without mutating the authority.
	if err := RunReview(append(append([]string{"capture-unachievable"}, binding...),
		"--request-hash", subjectHash, "--reason", "relay_transport_bound_exceeded"), io.Discard); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if err := RunReview(append(append([]string{"capture-unachievable"}, binding...),
		"--request-hash", subjectHash, "--withdraw=true"), io.Discard); err != nil {
		t.Fatal(err)
	}
	beforeReplay, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	var replayOutput bytes.Buffer
	if err := RunReview(append(append([]string{"capture-unachievable"}, binding...),
		"--request-hash", subjectHash, "--withdraw=true"), &replayOutput); err != nil {
		t.Fatalf("exact replay of an already-withdrawn declaration: %v\n%s", err, replayOutput.String())
	}
	var replay reviewUnachievableLensWithdrawalArtifact
	decodeStrictReviewJSON(t, replayOutput.Bytes(), &replay)
	if replay.Withdrawn {
		t.Fatalf("replayed withdrawal reported Withdrawn=true: %#v", replay)
	}
	afterReplay, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if afterReplay.Revision != beforeReplay.Revision {
		t.Fatal("an idempotent replayed withdrawal must not mutate the authority")
	}
}
