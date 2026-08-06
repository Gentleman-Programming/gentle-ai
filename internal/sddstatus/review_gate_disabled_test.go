package sddstatus

import (
	"context"
	"encoding/json"
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

// TestArchiveGateOffersButDoesNotBlockWhileReviewIsEnabled is the regression
// that matters most, and it is deliberately the first test in this file: the
// enabled path pins today's behaviour for the exact fixture (no review ever
// started for this candidate) the disabled tests below relax further.
//
// Corrective verify cycle 4, BLOCKER-1 (rdd-post-verify-review-offer's
// "Decline Proceeds to Unmanaged Ordinary Archive"): a genuinely missing
// receipt is decline-by-absence-of-action, not a blocker, on EITHER side of
// the switch. Superseded expectation (documented, not silently dropped):
// this test previously required GateInvalidated / blocked / resolve-review
// naming "terminal review receipt is missing" while enabled -- treating the
// unacted-on offer as a hard gate. The ratified "invitation, never a gate"
// reading supersedes that: the enabled path still surfaces the offer
// (proven at the wire level in review_offer_routing_test.go), but archive
// proceeds regardless.
func TestArchiveGateOffersButDoesNotBlockWhileReviewIsEnabled(t *testing.T) {
	root := t.TempDir()
	seedReviewlessArchiveReadyChange(t, root)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: false})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.ReviewGate != nil {
		t.Fatalf("enabled ReviewGate = %#v, want structural absence (decline, no review ever started)", status.ReviewGate)
	}
	if status.Dependencies.Archive != DependencyReady || status.NextRecommended != "archive" {
		t.Fatalf("enabled archive=%q next=%q, want ready/archive", status.Dependencies.Archive, status.NextRecommended)
	}
	if len(status.BlockedReasons) != 0 {
		t.Fatalf("enabled BlockedReasons = %v, want none", status.BlockedReasons)
	}
}

// TestArchiveGateEnforcesForACallerThatNeverResolvesTheSwitch holds the zero
// value against a fixture where enabled and disabled still visibly differ
// after BLOCKER-1 (a scope-changed EXPLICIT receipt -- real review activity
// gone wrong, not a decline, so it still blocks while enabled and only stops
// blocking when the switch is truly off). Any call site that forgets to
// resolve the switch keeps today's (enabled) behaviour, so a missed seam
// fails safe instead of silently dropping review obligations. Superseded:
// this test previously reused the "no review ever started" fixture, which
// BLOCKER-1 makes a decline on both sides of the switch and so can no longer
// distinguish "enforces" from "forgot to resolve".
func TestArchiveGateEnforcesForACallerThatNeverResolvesTheSwitch(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedBoundedReadyChange(t, root)
	writeApprovedReviewArtifacts(t, changeRoot)
	write(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Work\n- [x] scope changed\n")

	options := ResolveOptions{CWD: root, ChangeName: "thin"}
	if options.ReviewDisabled {
		t.Fatal("the zero-value ResolveOptions must enforce review obligations")
	}
	status, err := Resolve(options)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateScopeChanged ||
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

// TestDisabledArchiveGateProducesStructuralAbsenceNotADisposition is the
// corrective verify cycle's CRITICAL-1 fix (rdd-post-verify-review-offer's
// "Kill-Switch-Off Is Structural Absence" requirement, ratified text: "zero
// review code MUST execute on any SDD path: no offer, no status
// consultation, no disabled/unmanaged ceremony capable of failing or
// blocking" and "archive consults no reviewGate structured status"). This
// supersedes the disabled/unmanaged DISPOSITION this file previously
// required (a populated ReviewGate naming why nothing governed): the
// ratified requirement demands the field itself be absent, not populated
// with a reason. applyReviewGate now returns before any authority read while
// disabled, so status.ReviewGate stays nil and never reaches the wire.
func TestDisabledArchiveGateProducesStructuralAbsenceNotADisposition(t *testing.T) {
	root := t.TempDir()
	seedReviewlessArchiveReadyChange(t, root)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.ReviewGate != nil {
		t.Fatalf("disabled archive produced a review gate disposition instead of structural absence: %#v", status.ReviewGate)
	}
	if status.Dependencies.Archive == DependencyBlocked {
		t.Fatalf("disabled archive = %q, want unblocked", status.Dependencies.Archive)
	}

	projected, err := ProjectStatusV1(status)
	if err != nil {
		t.Fatalf("ProjectStatusV1() error = %v", err)
	}
	if projected.ReviewGate != nil {
		t.Fatalf("v1 projection carried a reviewGate the internal status never set: %#v", projected.ReviewGate)
	}
	payload, err := json.Marshal(projected)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(string(payload), "reviewGate") {
		t.Fatalf("disabled v1 wire payload leaked a reviewGate key: %s", payload)
	}
}

// TestDisabledArchiveGateNeverFabricatesReviewAuthority is the invariant that
// must hold no matter how absence is shaped: a disabled switch never
// approves, and it never invents a review transaction either.
func TestDisabledArchiveGateNeverFabricatesReviewAuthority(t *testing.T) {
	root := t.TempDir()
	seedReviewlessArchiveReadyChange(t, root)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.ReviewGate != nil && status.ReviewGate.Result == reviewtransaction.GateAllow {
		t.Fatalf("disabled gate fabricated an approval: %#v", status.ReviewGate)
	}
	if status.ReviewTransaction != nil {
		t.Fatalf("disabled gate invented a review transaction: %#v", status.ReviewTransaction)
	}
}

// TestDisabledArchiveGateIgnoresAnExplicitReviewReceiptToo holds the
// corrective verify cycle's stricter line: the ratified "zero review code"
// requirement carries no carve-out for an explicit review artifact. This
// supersedes the earlier RuntimeStore.ReviewDisabled doc comment, which drew
// the line at only the IMPLICIT demand — that narrower contract predates
// this wave's ratified requirement and no longer holds. An explicit receipt
// on disk, even one whose scope has since changed, is never read while the
// switch is off.
func TestDisabledArchiveGateIgnoresAnExplicitReviewReceiptToo(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedBoundedReadyChange(t, root)
	writeApprovedReviewArtifacts(t, changeRoot)
	// The candidate moves after approval: an enabled run would see this as
	// GateScopeChanged, but the disabled run must never read the receipt at
	// all, so this move is irrelevant to what it observes.
	write(t, filepath.Join(changeRoot, "tasks.md"), "- [x] 1.1 Work\n- [x] scope changed\n")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.ReviewGate != nil {
		t.Fatalf("explicit receipt while disabled = %#v, want structural absence", status.ReviewGate)
	}
	if status.Dependencies.Archive == DependencyBlocked {
		t.Fatalf("explicit receipt while disabled archive = %q, want unblocked", status.Dependencies.Archive)
	}
}

func TestDiscoveredTerminalBlockerRespectsReviewModeProvenance(t *testing.T) {
	tests := []struct {
		name   string
		result reviewtransaction.GateResult
	}{
		{name: "invalidated", result: reviewtransaction.GateInvalidated},
		{name: "escalated", result: reviewtransaction.GateEscalated},
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

			// Corrective verify cycle CRITICAL-1: while disabled, applyReviewGate
			// never runs at all, so the discovered blocker is structurally
			// absent rather than surfaced as a disabled/unmanaged disposition —
			// this supersedes the disposition this sub-test previously required.
			disabled, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
			if err != nil {
				t.Fatal(err)
			}
			if disabled.ReviewGate != nil {
				t.Fatalf("disabled discovered gate = %#v, want structural absence", disabled.ReviewGate)
			}
			if disabled.Dependencies.Archive == DependencyBlocked || disabled.NextRecommended == "resolve-review" {
				t.Fatalf("disabled discovered archive=%q next=%q, want unmanaged archive route", disabled.Dependencies.Archive, disabled.NextRecommended)
			}

			reenabled, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
			if err != nil {
				t.Fatal(err)
			}
			assertBlocked("re-enabled discovered", reenabled)

			// The ratified "zero review code" requirement carries no carve-out
			// for an explicit receipt either: even with it restored on disk,
			// the disabled run never reads it.
			if err := os.WriteFile(receiptPath, receipt, 0o644); err != nil {
				t.Fatal(err)
			}
			explicit, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
			if err != nil {
				t.Fatal(err)
			}
			if explicit.ReviewGate != nil {
				t.Fatalf("disabled explicit gate = %#v, want structural absence", explicit.ReviewGate)
			}
			if explicit.Dependencies.Archive == DependencyBlocked {
				t.Fatalf("disabled explicit archive = %q, want unblocked", explicit.Dependencies.Archive)
			}

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

// TestDisabledArchiveGateNeverConsultsAnApprovedReceiptEither is the last
// corner the corrective verify cycle's CRITICAL-1 fix closes: even a receipt
// that WOULD approve while enabled is never consulted while the switch is
// off. Superseded expectation (documented, not silently dropped): this file
// previously required the disabled run to stay byte-identical to the enabled
// approved gate ("disabling freezes authority read-only"). The ratified
// "zero review code MUST execute on any SDD path" requirement is
// unconditional and leaves no such carve-out — while off, archive proceeds
// unmanaged regardless of what any receipt on disk would have said.
func TestDisabledArchiveGateNeverConsultsAnApprovedReceiptEither(t *testing.T) {
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
	if disabled.ReviewGate != nil {
		t.Fatalf("disabled approved gate = %#v, want structural absence even though an approving receipt exists", disabled.ReviewGate)
	}
	if disabled.Dependencies.Archive == DependencyBlocked {
		t.Fatalf("disabled approved archive = %q, want unblocked", disabled.Dependencies.Archive)
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

// The tests below close corrective verify cycle 3's CRITICAL-B (residual
// C1): three sibling paths inside Resolve()/resolveEngramStatus() --
// staleAllowAuthority's discovery walk, resolveCompactRemediationAuthority's
// fallback into the same discovery path, and the governingRef!=nil
// validation/blocking branch -- were still ungated by reviewDisabled, so a
// stale-verify-totals fixture (evidence report claims more requirements
// than the spec actually has) reached resolveReviewAuthority's discovery
// walk and appended a blocked reason naming `gentle-ai review start`, a
// command the kill switch itself refuses, while the switch was OFF.

// staleTotalsVerifyEnvelope claims one more requirement/scenario than
// seedBoundedReadyChange's single-spec fixture actually has, reproducing the
// exact "stale evidence" shape (internally complete, non-failing report,
// totals mismatch against the current spec) the report's repro used.
func staleTotalsVerifyEnvelope(revision string) string {
	return strings.ReplaceAll(strings.ReplaceAll(boundedVerifyEnvelope(revision, "pass"), "requirements: 1/1", "requirements: 2/2"), "scenarios: 1/1", "scenarios: 2/2")
}

func seedStaleEvidenceReadyChange(t *testing.T, root string) string {
	t.Helper()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Work\n")
	write(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), "### Requirement: Auth\n#### Scenario: Valid login\n")
	write(t, filepath.Join(changeRoot, "verify-report.md"), staleTotalsVerifyEnvelope(shaID("1")))
	runSDDStatusGit(t, root, "init", "-q")
	runSDDStatusGit(t, root, "config", "user.email", "status@example.com")
	runSDDStatusGit(t, root, "config", "user.name", "Status Test")
	runSDDStatusGit(t, root, "add", ".")
	runSDDStatusGit(t, root, "commit", "-qm", "base")
	return changeRoot
}

// TestEnabledStaleEvidenceWithNoReceiptRestartsVerification covers the
// enabled behaviour for the exact fixture the disabled test below relaxes:
// with no governing receipt anywhere, the discovery walk finds nothing, and
// absent authority is decline-by-absence (issue #2137): stale non-FAIL
// evidence re-enters verification instead of blocking at resolve-review
// demanding a bounded review transaction that never existed.
func TestEnabledStaleEvidenceWithNoReceiptRestartsVerification(t *testing.T) {
	root := t.TempDir()
	seedStaleEvidenceReadyChange(t, root)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.Dependencies.Verify != DependencyReady || status.Dependencies.Archive != DependencyBlocked || status.NextRecommended != "verify" {
		t.Fatalf("enabled stale-evidence verify=%q archive=%q next=%q, want ready/blocked/verify", status.Dependencies.Verify, status.Dependencies.Archive, status.NextRecommended)
	}
	if reasons := strings.Join(status.BlockedReasons, "\n"); strings.Contains(reasons, "gentle-ai review start") || strings.Contains(reasons, "bounded review transaction is missing") {
		t.Fatalf("enabled stale-evidence BlockedReasons = %v, want no review-start or missing-transaction demand", status.BlockedReasons)
	}
}

// TestDisabledStaleEvidenceNeverConsultsReviewAuthority is the fix: the same
// fixture, with the switch off, grants the same "please re-verify" leniency
// with zero review consultation and zero blocked reasons -- never naming
// gentle-ai review start, a command the switch itself refuses to run.
func TestDisabledStaleEvidenceNeverConsultsReviewAuthority(t *testing.T) {
	root := t.TempDir()
	seedStaleEvidenceReadyChange(t, root)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if status.ReviewGate != nil {
		t.Fatalf("disabled stale-evidence reviewGate = %#v, want structural absence", status.ReviewGate)
	}
	if len(status.BlockedReasons) != 0 {
		t.Fatalf("disabled stale-evidence BlockedReasons = %v, want none", status.BlockedReasons)
	}
	if status.Dependencies.Verify != DependencyReady {
		t.Fatalf("disabled stale-evidence Verify = %q, want ready (please re-verify)", status.Dependencies.Verify)
	}
	if status.NextRecommended != "verify" {
		t.Fatalf("disabled stale-evidence NextRecommended = %q, want verify (not resolve-review)", status.NextRecommended)
	}
}

// The two tests below close corrective verify cycle 4's W-a: cycle 3's
// CRITICAL-B fix gated three sibling review-authority paths behind
// !reviewDisabled, but only gate 1 (staleAllowAuthority) had a covering
// test -- stripping gate 2 (resolveCompactRemediationAuthority) or gate 3
// (the governingRef!=nil ValidateSDDReceiptRef branch) left the whole
// suite green. Both now have a dedicated regression test whose failure
// mode is observable behavior, not a mock call count.

// failingCompactVerifyEnvelope is boundedVerifyEnvelope's shape with one
// blocker, so verifyResult.Passing is false (remediationRequired's own
// precondition) without disturbing anything else parseVerifyResult checks.
func failingCompactVerifyEnvelope(revision string) string {
	return strings.Replace(boundedVerifyEnvelope(revision, "fail"), "blockers: 0", "blockers: 1", 1)
}

// TestDisabledRemediationNeverConsultsCompactAuthority closes gate 2. The
// fixture has NO governingRef (no bind-sdd) and a FAILING verify report, so
// remediationRequired is true; resolveCompactRemediationAuthority's own
// discovery fallback (ref == nil) would find the approved, change-bound
// compact authority and hand resolveBoundedRemediation a non-nil
// CompactState -- observable as Required=true, a populated LineageID, and a
// reason naming "bounded compact remediation". With the switch off, gate 2
// must never call resolveReviewAuthority/discoverNativeReceipts at all, so
// remediation falls through to the generic "no transaction, no compact
// authority" reason instead, with no LineageID.
func TestDisabledRemediationNeverConsultsCompactAuthority(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	write(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), "### Requirement: Auth\n#### Scenario: Valid login\n")
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")
	write(t, filepath.Join(changeRoot, "verify-report.md"), failingCompactVerifyEnvelope(shaID("f")))

	enabled, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !enabled.RemediationState.Required || enabled.RemediationState.LineageID != "approved-thin" {
		t.Fatalf("fixture is wrong: enabled RemediationState = %#v, want required with LineageID approved-thin (the discoverable compact authority)", enabled.RemediationState)
	}
	if !strings.Contains(enabled.RemediationState.Reason, "bounded compact remediation") {
		t.Fatalf("fixture is wrong: enabled RemediationState.Reason = %q, want it to name compact remediation", enabled.RemediationState.Reason)
	}

	disabled, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if disabled.RemediationState.LineageID != "" {
		t.Fatalf("disabled RemediationState.LineageID = %q, want empty -- the compact authority was never consulted", disabled.RemediationState.LineageID)
	}
	if strings.Contains(disabled.RemediationState.Reason, "bounded compact remediation") {
		t.Fatalf("disabled RemediationState.Reason = %q, wrongly names compact remediation despite the switch being off", disabled.RemediationState.Reason)
	}
}

// TestDisabledArchiveGateNeverValidatesAnExplicitGoverningReceipt closes
// gate 3. The fixture has a genuine governingRef (an explicit `review
// bind-sdd` binding) that validates GateAllow when consulted -- with the
// switch off, gate 3 must never call ValidateSDDReceiptRef at all, so
// status.ReviewGate stays structurally absent instead of becoming a
// populated Allow disposition.
func TestDisabledArchiveGateNeverValidatesAnExplicitGoverningReceipt(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")
	if _, err := BindApprovedReview(context.Background(), root, "thin", "approved-thin", ""); err != nil {
		t.Fatal(err)
	}

	enabled, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if enabled.ReviewGate == nil || enabled.ReviewGate.Result != reviewtransaction.GateAllow {
		t.Fatalf("fixture is wrong: enabled ReviewGate = %#v, want a bound allow", enabled.ReviewGate)
	}

	disabled, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin", ReviewDisabled: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if disabled.ReviewGate != nil {
		t.Fatalf("disabled ReviewGate = %#v, want structural absence -- the bound governing receipt was never validated", disabled.ReviewGate)
	}
}
