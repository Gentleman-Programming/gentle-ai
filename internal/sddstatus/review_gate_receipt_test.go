//go:build legacy_compact_receipt

package sddstatus

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// review_gate_receipt_test.go is Wave 4 S5c' (design.md's decision-1
// amendment, 2026-08-03): the archive-gate evaluation path (status.go's two
// explicit-governance branches plus resolveCompactRemediationAuthority)
// reroutes from validateBoundReview/validateRuntimeBoundCandidate's stored
// reviewtransaction.GateContext comparison onto a fresh
// reviewtransaction.ValidateSDDReceiptRef re-derivation. Binding/
// BindingRevision, `review bind-sdd`, and the remediation-successor CAS stay
// untouched (Wave 7 territory) — only the archive-gate's SOURCE of validity
// changes, not who populates the underlying binding data.

// TestResolveGoverningReceiptRefRequiresAParsedNativeBinding ports
// TestBindingExistsRequiresAParsedNativeBinding (deleted in S7's re-scoped
// 8.1 alongside bindingExists itself, now genuinely orphaned by this
// slice's rerouting) onto resolveGoverningReceiptRef, which reuses the same
// underlying native-store read and must preserve the same two properties:
// an attempt-only runtime HEAD (no binding at all) is not treated as
// governance, and a corrupt native runtime HEAD fails closed with an error
// rather than silently reporting absence.
func TestResolveGoverningReceiptRefRequiresAParsedNativeBinding(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "attempts-only")
	if _, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "begin-attempts-only", WorkUnit: "apply",
		EvidenceGoal: "prove attempts do not imply review authority", MaxAttempts: 2, MaxChangedLines: 20,
	}); err != nil {
		t.Fatal(err)
	}
	ref, err := resolveGoverningReceiptRef(context.Background(), repo, "attempts-only")
	if err != nil {
		t.Fatal(err)
	}
	if ref != nil {
		t.Fatalf("attempt-only runtime HEAD was treated as an explicit review binding: %#v", ref)
	}

	if err := os.WriteFile(filepath.Join(store.Dir, "HEAD"), []byte("corrupt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveGoverningReceiptRef(context.Background(), repo, "attempts-only"); err == nil {
		t.Fatal("corrupt native runtime HEAD was accepted as a review binding")
	}
}

func TestResolveGoverningReceiptRefPresenceAndAbsence(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")

	absent, err := resolveGoverningReceiptRef(context.Background(), root, "thin")
	if err != nil || absent != nil {
		t.Fatalf("resolveGoverningReceiptRef() before bind = %#v, %v, want nil, nil", absent, err)
	}

	binding, err := BindApprovedReview(context.Background(), root, "thin", "approved-thin", "")
	if err != nil {
		t.Fatal(err)
	}
	present, err := resolveGoverningReceiptRef(context.Background(), root, "thin")
	if err != nil || present == nil || present.Lineage != binding.Lineage || present.ReceiptHash != binding.ReceiptHash {
		t.Fatalf("resolveGoverningReceiptRef() after bind = %#v, %v, want {Lineage:%q ReceiptHash:%q}", present, err, binding.Lineage, binding.ReceiptHash)
	}
}

// TestBoundReviewArchiveGateIgnoresStaleGateContextViaReDerivation proves the
// re-derivation this slice wires: a governing receipt whose persisted
// GateContext no longer matches the live post-apply evaluation (the exact
// shape validateBoundReview's boundGateContextMatches used to fail closed on)
// is now genuinely irrelevant, because ValidateSDDReceiptRef never reads or
// compares GateContext at all — the receipt hash plus a fresh gate
// evaluation is the whole re-derivation (design.md decision 1: "GateContext
// on the ref IS the re-derivation").
func TestBoundReviewArchiveGateIgnoresStaleGateContextViaReDerivation(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, "thin", "- [x] 1.1 Done\n")
	writeApprovedCompactAuthorityForChange(t, root, changeRoot, "approved-thin")
	write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("a"), "pass"))

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), root, "approved-thin")
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	receiptPayload, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := reviewtransaction.ParseCompactReceipt(receiptPayload)
	if err != nil {
		t.Fatal(err)
	}
	gate := reviewtransaction.EvaluateCompactGate(context.Background(), root, receipt, reviewtransaction.NativeGateRequestInput{
		Gate: reviewtransaction.GatePostApply, LineageID: "approved-thin",
	})
	if gate.Result != reviewtransaction.GateAllow {
		t.Fatalf("fixture live gate = %#v, want allow", gate)
	}

	binding := ReviewBinding{
		Schema: reviewBindingSchema, Change: "thin", Lineage: "approved-thin",
		AuthorityRevision: record.Revision, ReceiptHash: bindingHash(receiptPayload),
		GateContext: gate.Context,
	}
	// Corrupt only the persisted GateContext's LedgerHash — a well-formed but
	// wrong value, deliberately not reviewtransaction.EmptyFixDeltaHash, so
	// boundGateContextMatches's own empty-hash exemption cannot mask this.
	binding.GateContext.LedgerHash = "sha256:" + strings.Repeat("9", 64)
	if binding.GateContext.LedgerHash == gate.Context.LedgerHash {
		t.Fatal("fixture bug: corrupted LedgerHash accidentally matches the live value")
	}
	binding.Revision = bindingDigest(binding)
	writeRuntimeLegacyBinding(t, root, binding)

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "thin"})
	if err != nil {
		t.Fatal(err)
	}
	if status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateAllow {
		t.Fatalf("ReviewGate = %#v, want allow via re-derivation despite stale persisted GateContext", status.ReviewGate)
	}
	if status.Dependencies.Archive == DependencyBlocked && status.NextRecommended == "resolve-review" {
		t.Fatalf("status still routed through the retired stored-GateContext-comparison rejection: %#v", status)
	}
}

func TestCompactAuthorityPathsBoundAllowsExactFreshOpenSpecInfrastructure(t *testing.T) {
	const change = "add-webhook-signature-verifier"
	paths := []string{
		".gitignore",
		"internal/webhook/signature.go",
		"openspec/config.yaml",
		"openspec/changes/archive/.gitkeep",
		"openspec/specs/.gitkeep",
		"openspec/changes/" + change + "/proposal.md",
		"openspec/changes/" + change + "/design.md",
		"openspec/changes/" + change + "/tasks.md",
		"openspec/changes/" + change + "/specs/auth/spec.md",
	}
	state := reviewtransaction.CompactState{
		GenesisPaths:    append([]string(nil), paths...),
		InitialSnapshot: reviewtransaction.Snapshot{Paths: append([]string(nil), paths...)},
	}
	if bound, reason := compactAuthorityPathsBound(state, change); !bound || reason != "" {
		t.Fatalf("fresh shared OpenSpec infrastructure bound=%t reason=%q, want true, empty", bound, reason)
	}

	for _, test := range []struct {
		name string
		path string
	}{
		{name: "other active change", path: "openspec/changes/other-change/tasks.md"},
		{name: "archived change payload", path: "openspec/changes/archive/previous-change/proposal.md"},
		{name: "canonical capability spec", path: "openspec/specs/webhooks/spec.md"},
		{name: "unexpected config", path: "openspec/config.yml"},
		{name: "traversal", path: "openspec/changes/" + change + "/../other-change/tasks.md"},
		{name: "ambiguous separator", path: "openspec\\changes\\other-change\\tasks.md"},
	} {
		t.Run(test.name, func(t *testing.T) {
			forged := state
			forged.GenesisPaths = append(append([]string(nil), paths...), test.path)
			forged.InitialSnapshot.Paths = append([]string(nil), forged.GenesisPaths...)
			if bound, reason := compactAuthorityPathsBound(forged, change); bound || reason == "" {
				t.Fatalf("foreign path %q bound=%t reason=%q, want false with refusal", test.path, bound, reason)
			}
		})
	}
}

func TestResolveAllowsApprovedFreshOpenSpecInfrastructureForActiveChange(t *testing.T) {
	const change = "add-webhook-signature-verifier"
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, change, "- [x] 1.1 Done\n")
	writeApprovedCompactAuthorityForChangeWithCandidate(t, root, changeRoot, "approved-webhook", "- [x] 1.1 Done\n", func() {
		write(t, filepath.Join(root, ".gitignore"), ".env\n")
		write(t, filepath.Join(root, "internal", "webhook", "signature.go"), "package webhook\n")
		write(t, filepath.Join(root, "openspec", "config.yaml"), "schema: specification\n")
		write(t, filepath.Join(root, "openspec", "changes", "archive", ".gitkeep"), "")
		write(t, filepath.Join(root, "openspec", "specs", ".gitkeep"), "")
		write(t, filepath.Join(changeRoot, "proposal.md"), "# Proposal\nFresh infrastructure included\n")
		write(t, filepath.Join(changeRoot, "design.md"), "# Design\nFresh infrastructure included\n")
		write(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), "### Requirement: Auth\n#### Scenario: Fresh infrastructure\n")
		write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("a"), "pass"))
	})

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: change})
	if err != nil {
		t.Fatal(err)
	}
	if status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateAllow ||
		status.Dependencies.Archive == DependencyBlocked || status.NextRecommended == "resolve-review" {
		t.Fatalf("fresh OpenSpec infrastructure status = %#v", status)
	}
	projected, err := ProjectStatusV1(status)
	if err != nil {
		t.Fatal(err)
	}
	if projected.ReviewGate == nil || projected.ReviewGate.Result != reviewtransaction.GateAllow {
		t.Fatalf("fresh OpenSpec infrastructure projected review gate = %#v", projected.ReviewGate)
	}
}

// TestBoundReviewArchiveStatusKeepsPostReviewVerifyReportInformational proves
// that a post-review verification report and its settlement remain review
// context only. SDD archive readiness stays governed by its completed tasks and
// passing verification rather than the review gate's scope-changed observation.
func TestBoundReviewArchiveStatusKeepsPostReviewVerifyReportInformational(t *testing.T) {
	const change = "post-review-verify-report"
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, change, "- [x] 1.1 Done\n")
	verifyPath := filepath.Join(changeRoot, "verify-report.md")
	write(t, verifyPath, boundedVerifyEnvelope(shaID("a"), "pass"))
	writeApprovedCompactAuthorityForChangeWithCandidate(t, root, changeRoot, "approved-post-review-report", "- [x] 1.1 Approved\n", nil)

	if _, err := BindApprovedReview(context.Background(), root, change, "approved-post-review-report", ""); err != nil {
		t.Fatal(err)
	}
	initial, err := Resolve(ResolveOptions{CWD: root, ChangeName: change})
	if err != nil {
		t.Fatal(err)
	}
	if initial.ReviewGate == nil || initial.ReviewGate.Result != reviewtransaction.GateAllow ||
		initial.Dependencies.Archive != DependencyReady || initial.NextRecommended != "archive" {
		t.Fatalf("pre-verify bound status = %#v, want allow and archive-ready", initial)
	}

	finalReport := boundedVerifyEnvelope(shaID("b"), "pass")
	write(t, verifyPath, finalReport)
	runSDDStatusGit(t, root, "add", "openspec")
	ref, err := resolveGoverningReceiptRef(context.Background(), root, change)
	if err != nil || ref == nil {
		t.Fatalf("governing receipt ref = %#v, %v", ref, err)
	}
	if result, _, err := reviewtransaction.ValidateSDDReceiptRef(context.Background(), root, *ref); err != nil || result != reviewtransaction.GateScopeChanged {
		t.Fatalf("post-review receipt validation = %q, %v, want scope-changed", result, err)
	}
	beforeSettlement, err := Resolve(ResolveOptions{CWD: root, ChangeName: change})
	if err != nil {
		t.Fatal(err)
	}
	if beforeSettlement.ReviewGate == nil || beforeSettlement.ReviewGate.Result != reviewtransaction.GateScopeChanged ||
		beforeSettlement.Dependencies.Archive != DependencyReady || beforeSettlement.NextRecommended != "archive" {
		t.Fatalf("unattested post-review report status = %#v, want informational scope-changed ready/archive", beforeSettlement)
	}

	store := mustRuntimeStore(t, root, change)
	runtime, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: runtime.Revision, RequestID: "begin-final-verify", WorkUnit: "verify",
		EvidenceGoal: "run final independent verification", MaxAttempts: 1, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "settle-final-verify", Outcome: AttemptPassed,
		EvidenceRevision: shaID("b"), Diagnosis: "final verification passed", HarnessDisposition: HarnessReused,
		CleanupEvidence: "no cleanup required", ProcessEvidence: "focused verification passed",
	}); err != nil {
		t.Fatal(err)
	}

	settled, err := Resolve(ResolveOptions{CWD: root, ChangeName: change})
	if err != nil {
		t.Fatal(err)
	}
	if settled.ReviewGate == nil || settled.ReviewGate.Result != reviewtransaction.GateScopeChanged ||
		settled.Dependencies.Archive != DependencyReady || settled.NextRecommended != "archive" {
		t.Fatalf("attested post-review report status = %#v, want informational scope-changed archive-ready", settled)
	}
	if result, _, err := reviewtransaction.ValidateSDDReceiptRef(context.Background(), root, *ref); err != nil || result != reviewtransaction.GateScopeChanged {
		t.Fatalf("generic bound receipt validation after settlement = %q, %v, want unchanged scope-changed", result, err)
	}
	write(t, verifyPath, finalReport+"\n# Mutated after settlement\n")
	runSDDStatusGit(t, root, "add", "openspec")
	assertPostReviewVerifyReportScopeChanged(t, root, change, *ref)
	write(t, verifyPath, finalReport)
	runSDDStatusGit(t, root, "add", "openspec")

	otherPath := filepath.Join(root, "docs", "other.md")
	write(t, otherPath, "unrelated\n")
	runSDDStatusGit(t, root, "add", "docs/other.md")
	assertPostReviewVerifyReportScopeChanged(t, root, change, *ref)
	runSDDStatusGit(t, root, "rm", "-q", "--cached", "docs/other.md")
	if err := os.Remove(otherPath); err != nil {
		t.Fatal(err)
	}

	foreignPath := filepath.Join(root, "openspec", "changes", "foreign-change", "tasks.md")
	write(t, foreignPath, "- [x] foreign work\n")
	runSDDStatusGit(t, root, "add", "openspec")
	assertPostReviewVerifyReportScopeChanged(t, root, change, *ref)
}

func assertPostReviewVerifyReportScopeChanged(t *testing.T, root, change string, ref reviewtransaction.SDDReceiptRef) {
	t.Helper()
	if result, _, err := reviewtransaction.ValidateSDDReceiptRef(context.Background(), root, ref); err != nil || result != reviewtransaction.GateScopeChanged {
		t.Fatalf("changed bound receipt validation = %q, %v, want scope-changed", result, err)
	}
	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: change})
	if err != nil {
		t.Fatal(err)
	}
	if status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateScopeChanged ||
		status.Dependencies.Archive != DependencyReady || status.NextRecommended != "archive" {
		t.Fatalf("changed post-review status = %#v, want informational scope-changed ready/archive", status)
	}
}

func TestLegacyPostReviewVerifyReportWithArbitraryWorkUnitIsInformational(t *testing.T) {
	const change = "legacy-post-review-verify-report"
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, change, "- [x] 1.1 Done\n")
	verifyPath := filepath.Join(changeRoot, "verify-report.md")
	write(t, verifyPath, boundedVerifyEnvelope(shaID("a"), "pass"))
	writeApprovedCompactAuthorityForChangeWithCandidate(t, root, changeRoot, "approved-legacy-post-review-report", "- [x] 1.1 Approved\n", nil)
	if _, err := BindApprovedReview(context.Background(), root, change, "approved-legacy-post-review-report", ""); err != nil {
		t.Fatal(err)
	}

	finalReport := boundedVerifyEnvelope(shaID("b"), "pass")
	write(t, verifyPath, finalReport)
	runSDDStatusGit(t, root, "add", "openspec")
	ref, err := resolveGoverningReceiptRef(context.Background(), root, change)
	if err != nil || ref == nil {
		t.Fatalf("governing receipt ref = %#v, %v", ref, err)
	}
	store := mustRuntimeStore(t, root, change)
	runtime, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: runtime.Revision, RequestID: "begin-legacy-final-verify", WorkUnit: "post-correction-final-verification",
		EvidenceGoal: "run final independent verification", MaxAttempts: 1, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	begin, err := store.loadRecord(started.Revision)
	if err != nil {
		t.Fatal(err)
	}
	legacy := runtimeRecord{Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: started.Revision, Operation: runtimeOperationFinish, RequestID: "settle-legacy-final-verify", Finish: &runtimeFinishEvent{
		Ordinal: 1, FinishCandidateIdentity: begin.Begin.BeginCandidateIdentity, FinishCandidateTree: begin.Begin.BeginCandidateTree,
		Outcome: AttemptPassed, ChangedLines: 0, EvidenceRevision: shaID("b"), Diagnosis: "legacy final verification passed",
		HarnessDisposition: HarnessReused, CleanupEvidence: "no cleanup required", ProcessEvidence: "focused verification passed",
	}}
	legacy.RequestDigest = runtimeValueHash("gentle-ai.sdd-runtime-finish-request/v1", FinishAttemptRequest{
		ExpectedRevision: legacy.PreviousRevision, RequestID: legacy.RequestID, Outcome: legacy.Finish.Outcome,
		EvidenceRevision: legacy.Finish.EvidenceRevision, Diagnosis: legacy.Finish.Diagnosis,
		HarnessDisposition: legacy.Finish.HarnessDisposition, CleanupEvidence: legacy.Finish.CleanupEvidence,
		ProcessEvidence: legacy.Finish.ProcessEvidence,
	})
	revision, payload, err := runtimeRecordRevision(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.publishRecord(revision, payload); err != nil {
		t.Fatal(err)
	}
	if err := store.publishHead(revision); err != nil {
		t.Fatal(err)
	}

	legacyStatus, err := Resolve(ResolveOptions{CWD: root, ChangeName: change, IncludeInstructions: true})
	if err != nil {
		t.Fatal(err)
	}
	if legacyStatus.ReviewGate == nil || legacyStatus.ReviewGate.Result != reviewtransaction.GateScopeChanged ||
		legacyStatus.Dependencies.Verify != DependencyAllDone || legacyStatus.Dependencies.Archive != DependencyReady || legacyStatus.NextRecommended != "archive" {
		t.Fatalf("legacy post-review status = %#v, want informational receipt state with ordinary archive routing", legacyStatus)
	}
	if result, _, err := reviewtransaction.ValidateSDDReceiptRef(context.Background(), root, *ref); err != nil || result != reviewtransaction.GateScopeChanged {
		t.Fatalf("generic legacy receipt validation = %q, %v, want unchanged scope-changed", result, err)
	}

}

// TestFinalVerifySettlementDegradesUnattestableReportToEmptyAttestation proves
// attestation derivation never aborts settlement of a passing verify attempt:
// an unattestable report (here a non-passing verdict) settles with an empty
// traceability attestation while strict SDD verification keeps its own verdict.
func TestFinalVerifySettlementDegradesUnattestableReportToEmptyAttestation(t *testing.T) {
	const change = "strict-final-verify-report"
	root := t.TempDir()
	changeRoot := seedReadyChange(t, root, change, "- [x] 1.1 Done\n")
	write(t, filepath.Join(changeRoot, "verify-report.md"), boundedVerifyEnvelope(shaID("a"), "fail"))
	runSDDStatusGit(t, root, "init", "-q")
	runSDDStatusGit(t, root, "config", "user.email", "status@example.com")
	runSDDStatusGit(t, root, "config", "user.name", "Status Test")
	runSDDStatusGit(t, root, "add", ".")
	runSDDStatusGit(t, root, "commit", "-qm", "base")

	store := mustRuntimeStore(t, root, change)
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "begin-strict-final-verify", WorkUnit: "verify",
		EvidenceGoal: "run final independent verification", MaxAttempts: 1, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "settle-strict-final-verify", Outcome: AttemptPassed,
		EvidenceRevision: shaID("a"), Diagnosis: "final verification passed", HarnessDisposition: HarnessReused,
		CleanupEvidence: "no cleanup required", ProcessEvidence: "focused verification passed",
	}); err != nil {
		t.Fatalf("passing final verify settlement aborted on an unattestable canonical report: %v", err)
	}
	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.ActiveAttempt != nil || len(status.Attempts) != 1 || status.Attempts[0].Outcome != AttemptPassed ||
		status.Attempts[0].AttestedVerifyReportDigest != "" {
		t.Fatalf("degraded settlement = %#v, want a settled passing attempt with an empty attestation", status)
	}
}

// TestFinalVerifySettlementAttestsReportAnchoredAtWorkspace proves the
// canonical verify-report path is anchored at the planning workspace (--cwd),
// not at the Git repository root: a workspace that is a subdirectory of its
// repository must still settle a passing final verification and attest the
// exact report bytes.
func TestFinalVerifySettlementAttestsReportAnchoredAtWorkspace(t *testing.T) {
	const change = "workspace-anchored-final-verify-report"
	root := t.TempDir()
	workspace := filepath.Join(root, "service")
	changeRoot := seedReadyChange(t, workspace, change, "- [x] 1.1 Done\n")
	report := boundedVerifyEnvelope(shaID("a"), "pass")
	write(t, filepath.Join(changeRoot, "verify-report.md"), report)
	runSDDStatusGit(t, root, "init", "-q")
	runSDDStatusGit(t, root, "config", "user.email", "status@example.com")
	runSDDStatusGit(t, root, "config", "user.name", "Status Test")
	runSDDStatusGit(t, root, "add", ".")
	runSDDStatusGit(t, root, "commit", "-qm", "base")

	store, err := OpenRuntimeStore(context.Background(), workspace, change)
	if err != nil {
		t.Fatal(err)
	}
	if store.Repo == store.Workspace {
		t.Fatalf("fixture must place the workspace below the repository root: repo=%q workspace=%q", store.Repo, store.Workspace)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "begin-workspace-final-verify", WorkUnit: "verify",
		EvidenceGoal: "run final independent verification", MaxAttempts: 1, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "settle-workspace-final-verify", Outcome: AttemptPassed,
		EvidenceRevision: shaID("a"), Diagnosis: "final verification passed", HarnessDisposition: HarnessReused,
		CleanupEvidence: "no cleanup required", ProcessEvidence: "focused verification passed",
	}); err != nil {
		t.Fatalf("workspace-anchored final verify settlement failed: %v", err)
	}
	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Attempts) == 0 {
		t.Fatalf("workspace-anchored settlement recorded no attempts: %#v", status)
	}
	last := status.Attempts[len(status.Attempts)-1]
	if last.Outcome != AttemptPassed || last.AttestedVerifyReportDigest != verifyReportDigest([]byte(report)) {
		t.Fatalf("workspace-anchored settlement attempt = %#v, want passed with the exact report digest", last)
	}
}
