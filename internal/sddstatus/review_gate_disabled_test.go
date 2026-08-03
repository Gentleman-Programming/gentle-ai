package sddstatus

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// The maintainer's rule for this file is the same one RuntimeStore.ReviewDisabled
// already applies to the runtime ledger: while the kill switch is off,
// receipt-driven development does not exist, so it must have no implications.
//
// The deadlock it removes is real and closed. An SDD cycle reaches its archive
// decision, the archive gate demands a terminal review receipt, and `review
// start` is refused from producing one while the switch is off. An orchestrator
// reading `nextRecommended: "resolve-review"` loops forever between a gate that
// demands a review and a review system that refuses to run.
//
// Nothing is fabricated here: no receipt is invented, the gate result stays the
// honest evaluation, and `allow` is never manufactured. It removes only the
// IMPLICIT demand — an explicit review artifact is still validated in full.

// seedReviewlessArchiveReadyChange stages the exact end-of-cycle fixture the
// rule is about: a real repository, every task complete, passing verification
// evidence, and no review authority of any kind — because the switch was off
// for the whole cycle, so no lineage was ever allowed to start.
func seedReviewlessArchiveReadyChange(t *testing.T, root string) string {
	t.Helper()
	changeRoot := seedBoundedReadyChange(t, root)
	runSDDStatusGit(t, root, "init", "-q")
	runSDDStatusGit(t, root, "config", "user.email", "status@example.com")
	runSDDStatusGit(t, root, "config", "user.name", "Status Test")
	runSDDStatusGit(t, root, "add", ".")
	runSDDStatusGit(t, root, "commit", "-qm", "base")
	return changeRoot
}

// TestArchiveGateStillDemandsAReviewWhileReviewIsEnabled is the regression that
// matters most, and it is deliberately the first test in this file: the
// enabled path must not move one byte. It pins today's behaviour for the exact
// fixture the disabled tests below relax.
func TestArchiveGateStillDemandsAReviewWhileReviewIsEnabled(t *testing.T) {
	root := t.TempDir()
	seedReviewlessArchiveReadyChange(t, root)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: false})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateInvalidated {
		t.Fatalf("enabled ReviewGate = %#v, want invalidated", status.ReviewGate)
	}
	if status.ReviewGate.Delivery != "" {
		t.Fatalf("enabled ReviewGate.Delivery = %q, want empty so the enabled wire shape is unchanged", status.ReviewGate.Delivery)
	}
	if status.Dependencies.Archive != DependencyBlocked || status.NextRecommended != "resolve-review" {
		t.Fatalf("enabled archive=%q next=%q, want blocked/resolve-review", status.Dependencies.Archive, status.NextRecommended)
	}
	if !strings.Contains(strings.Join(status.BlockedReasons, "\n"), "terminal review receipt is missing") {
		t.Fatalf("enabled BlockedReasons = %v, want the missing-receipt reason", status.BlockedReasons)
	}
}

// TestArchiveGateEnforcesForACallerThatNeverResolvesTheSwitch holds the zero
// value. Any call site that forgets to resolve the switch keeps today's
// behaviour, so a missed seam fails safe instead of silently dropping review
// obligations.
func TestArchiveGateEnforcesForACallerThatNeverResolvesTheSwitch(t *testing.T) {
	root := t.TempDir()
	seedReviewlessArchiveReadyChange(t, root)

	options := ResolveOptions{CWD: root, ChangeName: "thin"}
	if options.ReviewDisabled {
		t.Fatal("the zero-value ResolveOptions must enforce review obligations")
	}
	status, err := Resolve(options)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateInvalidated ||
		status.Dependencies.Archive != DependencyBlocked || status.NextRecommended != "resolve-review" {
		t.Fatalf("zero-value gate=%#v archive=%q next=%q, want the enforcing shape",
			status.ReviewGate, status.Dependencies.Archive, status.NextRecommended)
	}
}

// TestArchiveGateDoesNotBlockArchiveWhileReviewIsDisabled is the fix: the same
// fixture, with the switch off, carries on as if receipt-driven development had
// never been part of the cycle.
func TestArchiveGateDoesNotBlockArchiveWhileReviewIsDisabled(t *testing.T) {
	root := t.TempDir()
	seedReviewlessArchiveReadyChange(t, root)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.Dependencies.Archive == DependencyBlocked {
		t.Fatalf("disabled archive = %q, want unblocked; blocked reasons = %v", status.Dependencies.Archive, status.BlockedReasons)
	}
	if status.NextRecommended == "resolve-review" {
		t.Fatalf("disabled next = %q, want a route that is not the resolve-review loop", status.NextRecommended)
	}
	for _, reason := range status.BlockedReasons {
		if strings.Contains(reason, "review receipt") || strings.Contains(reason, "review authority") {
			t.Fatalf("disabled BlockedReasons still carries a review blocker: %q", reason)
		}
	}
}

// TestDisabledArchiveGateStillRecordsThatNoReviewGovernedTheChange is the other
// half of the rule. Not blocking is not the same as pretending nothing
// happened: whoever reads this archive later must be able to tell it closed
// under ordinary repository policy rather than under a receipt.
func TestDisabledArchiveGateStillRecordsThatNoReviewGovernedTheChange(t *testing.T) {
	root := t.TempDir()
	seedReviewlessArchiveReadyChange(t, root)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.ReviewGate == nil {
		t.Fatal("disabled archive erased the review gate instead of recording that no review governed the change")
	}
	if status.ReviewGate.Delivery != reviewtransaction.RDDDeliveryDisabledUnmanaged {
		t.Fatalf("disabled ReviewGate.Delivery = %q, want %q", status.ReviewGate.Delivery, reviewtransaction.RDDDeliveryDisabledUnmanaged)
	}
	if !strings.Contains(status.ReviewGate.Reason, "receipt-driven development is disabled") {
		t.Fatalf("disabled ReviewGate.Reason = %q, want it to name the situation", status.ReviewGate.Reason)
	}
	// The mechanism that could not govern stays discoverable behind the
	// situation, exactly as the delivery gate keeps its denial reason.
	if !strings.Contains(status.ReviewGate.Reason, "terminal review receipt is missing") {
		t.Fatalf("disabled ReviewGate.Reason = %q, want the underlying reason preserved", status.ReviewGate.Reason)
	}

	projected, err := ProjectStatusV1(status)
	if err != nil {
		t.Fatalf("ProjectStatusV1() error = %v", err)
	}
	if projected.ReviewGate == nil || projected.ReviewGate.Delivery != reviewtransaction.RDDDeliveryDisabledUnmanaged {
		t.Fatalf("v1 projection lost the unmanaged disposition: %#v", projected.ReviewGate)
	}
}

// TestDisabledArchiveGateNeverReadsAsAnApproval is the invariant that must hold
// no matter how the disposition is shaped: a disabled switch may decline to
// manage, never approve.
func TestDisabledArchiveGateNeverReadsAsAnApproval(t *testing.T) {
	root := t.TempDir()
	seedReviewlessArchiveReadyChange(t, root)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.ReviewGate == nil || status.ReviewGate.Result == reviewtransaction.GateAllow {
		t.Fatalf("disabled gate fabricated an approval: %#v", status.ReviewGate)
	}
	if status.ReviewGate.Delivery == reviewtransaction.RDDDeliveryReceiptGoverned {
		t.Fatalf("disabled gate claimed a receipt governs: %#v", status.ReviewGate)
	}
	lowered := strings.ToLower(status.ReviewGate.Reason)
	for _, forbidden := range []string{"approved", "allow", "pass"} {
		if strings.Contains(lowered, forbidden) {
			t.Fatalf("disabled ReviewGate.Reason %q reads as an approval (contains %q)", status.ReviewGate.Reason, forbidden)
		}
	}
	if status.ReviewTransaction != nil {
		t.Fatalf("disabled gate invented a review transaction: %#v", status.ReviewTransaction)
	}
}

// TestDisabledArchiveGateStillValidatesAnExplicitReviewReceipt holds the line
// the RuntimeStore.ReviewDisabled doc comment draws: the switch removes the
// IMPLICIT demand, never the checks on an explicit request. A change that
// carries a review receipt asked for receipt-driven development to act, so its
// receipt is still validated in full and a broken one still blocks.
func TestDisabledArchiveGateStillValidatesAnExplicitReviewReceipt(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedBoundedReadyChange(t, root)
	writeApprovedReviewArtifacts(t, changeRoot)
	// The candidate moves after approval: the explicit receipt no longer
	// governs these bytes.
	write(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Work\n- [x] scope changed\n")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateScopeChanged {
		t.Fatalf("explicit receipt while disabled = %#v, want scope-changed", status.ReviewGate)
	}
	if status.Dependencies.Archive != DependencyBlocked || status.NextRecommended != "resolve-review" {
		t.Fatalf("explicit receipt while disabled archive=%q next=%q, want blocked/resolve-review",
			status.Dependencies.Archive, status.NextRecommended)
	}
}

func TestDiscoveredTerminalBlockerRespectsReviewModeProvenance(t *testing.T) {
	tests := []struct {
		name       string
		result     reviewtransaction.GateResult
		wantReason string
	}{
		{name: "invalidated", result: reviewtransaction.GateInvalidated, wantReason: "review receipt was invalidated"},
		{name: "escalated", result: reviewtransaction.GateEscalated, wantReason: "escalated the receipt"},
	}

	original := evaluateNativeReviewGate
	t.Cleanup(func() { evaluateNativeReviewGate = original })
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			changeRoot := seedBoundedReadyChange(t, root)
			writeApprovedReviewArtifacts(t, changeRoot)
			receiptPath := filepath.Join(changeRoot, "reviews", "receipt.json")
			receipt, err := os.ReadFile(receiptPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(receiptPath); err != nil {
				t.Fatal(err)
			}
			store, err := reviewtransaction.AuthoritativeStore(context.Background(), root, "thin-lineage")
			if err != nil {
				t.Fatal(err)
			}
			before, err := store.LoadChain()
			if err != nil {
				t.Fatal(err)
			}
			evaluateNativeReviewGate = func(context.Context, string, reviewtransaction.Receipt, reviewtransaction.GateRequest) reviewtransaction.NativeGateEvaluation {
				return reviewtransaction.NativeGateEvaluation{Result: tt.result}
			}

			assertBlocked := func(label string, status Status) {
				t.Helper()
				if status.ReviewGate == nil || status.ReviewGate.Result != tt.result || status.ReviewGate.Delivery != "" {
					t.Fatalf("%s gate = %#v, want %q without delivery", label, status.ReviewGate, tt.result)
				}
				if status.Dependencies.Archive != DependencyBlocked || status.NextRecommended != "resolve-review" {
					t.Fatalf("%s archive=%q next=%q, want blocked/resolve-review", label, status.Dependencies.Archive, status.NextRecommended)
				}
			}

			enabled, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
			if err != nil {
				t.Fatal(err)
			}
			assertBlocked("enabled discovered", enabled)

			disabled, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
			if err != nil {
				t.Fatal(err)
			}
			if disabled.ReviewGate == nil || disabled.ReviewGate.Result != tt.result || disabled.ReviewGate.Delivery != reviewtransaction.RDDDeliveryDisabledUnmanaged {
				t.Fatalf("disabled discovered gate = %#v, want %q disabled/unmanaged", disabled.ReviewGate, tt.result)
			}
			if disabled.Dependencies.Archive == DependencyBlocked || disabled.NextRecommended == "resolve-review" {
				t.Fatalf("disabled discovered archive=%q next=%q, want unmanaged archive route", disabled.Dependencies.Archive, disabled.NextRecommended)
			}
			if !strings.Contains(disabled.ReviewGate.Reason, tt.wantReason) {
				t.Fatalf("disabled discovered reason = %q, want underlying %q", disabled.ReviewGate.Reason, tt.wantReason)
			}

			reenabled, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
			if err != nil {
				t.Fatal(err)
			}
			assertBlocked("re-enabled discovered", reenabled)

			if err := os.WriteFile(receiptPath, receipt, 0o644); err != nil {
				t.Fatal(err)
			}
			explicit, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
			if err != nil {
				t.Fatal(err)
			}
			assertBlocked("disabled explicit", explicit)

			after, err := store.LoadChain()
			if err != nil {
				t.Fatal(err)
			}
			if after.HeadRevision != before.HeadRevision {
				t.Fatalf("authority revision changed from %q to %q", before.HeadRevision, after.HeadRevision)
			}
		})
	}
}

// TestDisabledArchiveGateStillHonoursAnApprovedReceipt keeps the enabled path
// byte-identical where a receipt does govern: disabling freezes authority
// read-only, it does not unmake an approval bound to exactly these bytes.
func TestDisabledArchiveGateStillHonoursAnApprovedReceipt(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedBoundedReadyChange(t, root)
	writeApprovedReviewArtifacts(t, changeRoot)

	enabled, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	disabled, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if enabled.ReviewGate == nil || enabled.ReviewGate.Result != reviewtransaction.GateAllow {
		t.Fatalf("enabled approved gate = %#v, want allow", enabled.ReviewGate)
	}
	if *disabled.ReviewGate != *enabled.ReviewGate {
		t.Fatalf("disabled approved gate = %#v, want byte-identical to enabled %#v", disabled.ReviewGate, enabled.ReviewGate)
	}
	if disabled.Dependencies != enabled.Dependencies || disabled.NextRecommended != enabled.NextRecommended {
		t.Fatalf("disabled approved routing = %#v/%q, want %#v/%q",
			disabled.Dependencies, disabled.NextRecommended, enabled.Dependencies, enabled.NextRecommended)
	}
}

// TestDisabledSwitchDoesNotUnblockArchiveForNonReviewReasons keeps the scope
// honest. Blocking archive because the tasks are unfinished has nothing to do
// with receipt-driven development, so the kill switch must not touch it.
func TestDisabledSwitchDoesNotUnblockArchiveForNonReviewReasons(t *testing.T) {
	root := t.TempDir()
	seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n- [ ] 1.2 Unfinished\n")
	write(t, filepath.Join(root, "openspec", "changes", "thin", "verify-report.md"), boundedVerifyEnvelope(shaID("1"), "pass"))
	runSDDStatusGit(t, root, "init", "-q")
	runSDDStatusGit(t, root, "config", "user.email", "status@example.com")
	runSDDStatusGit(t, root, "config", "user.name", "Status Test")
	runSDDStatusGit(t, root, "add", ".")
	runSDDStatusGit(t, root, "commit", "-qm", "base")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.TaskProgress.AllComplete {
		t.Fatalf("fixture is wrong: tasks = %#v", status.TaskProgress)
	}
	if status.Dependencies.Archive != DependencyBlocked {
		t.Fatalf("disabled archive with unfinished tasks = %q, want blocked", status.Dependencies.Archive)
	}
	if status.NextRecommended == "archive" {
		t.Fatalf("disabled next = %q, want a route that is not archive", status.NextRecommended)
	}
	if status.ReviewGate != nil {
		t.Fatalf("archive gating never ran, so there is no gate to report: %#v", status.ReviewGate)
	}
}

// TestDisabledReviewModeDoesNotBlockPreVerifyRouting is the regression guard for
// issue-1932: when review mode is disabled, completing apply must leave verify
// ready without forcing an implicit review/start obligation before independent
// verification can run.
func TestDisabledReviewModeDoesNotBlockPreVerifyRouting(t *testing.T) {
	root := t.TempDir()
	seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
	// verifyReport is deliberately absent: apply is complete, verify is not run yet.
	runSDDStatusGit(t, root, "init", "-q")
	runSDDStatusGit(t, root, "config", "user.email", "status@example.com")
	runSDDStatusGit(t, root, "config", "user.name", "Status Test")
	runSDDStatusGit(t, root, "add", ".")
	runSDDStatusGit(t, root, "commit", "-qm", "base")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.ApplyState != ApplyAllDone {
		t.Fatalf("fixture ApplyState = %q, want %q", status.ApplyState, ApplyAllDone)
	}
	if status.Dependencies.Verify != DependencyReady {
		t.Fatalf("disabled Verify = %q, want ready", status.Dependencies.Verify)
	}
	if status.NextRecommended != "verify" {
		t.Fatalf("disabled NextRecommended = %q, want verify", status.NextRecommended)
	}
	for _, reason := range status.BlockedReasons {
		if strings.Contains(reason, "explicit bounded review/start(target) is required") {
			t.Fatalf("disabled BlockedReasons contains review/start requirement: %v", status.BlockedReasons)
		}
	}
}
