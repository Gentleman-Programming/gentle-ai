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

// TestBoundReviewArchiveGateAllowsAttestedPostReviewVerifyReportDelta proves
// the one archive-status exception: final independent verification replaces
// the canonical passing report after review, and its native settlement binds
// those exact bytes and the resulting candidate tree. Generic review gates
// remain unchanged; this test exercises only the bound SDD archive projection.
func TestBoundReviewArchiveGateAllowsAttestedPostReviewVerifyReportDelta(t *testing.T) {
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
	if beforeSettlement.Dependencies.Archive != DependencyBlocked || beforeSettlement.NextRecommended != "resolve-review" {
		t.Fatalf("unattested post-review report status = %#v, want scope-changed archive block", beforeSettlement)
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
	if settled.ReviewGate == nil || settled.ReviewGate.Result != reviewtransaction.GateAllow ||
		settled.ReviewGate.Reason != "bound receipt differs only by the native-settlement-attested canonical passing verify report" ||
		settled.Dependencies.Archive != DependencyReady || settled.NextRecommended != "archive" {
		t.Fatalf("attested post-review report status = %#v, want report-delta allow and archive-ready", settled)
	}
	if result, _, err := reviewtransaction.ValidateSDDReceiptRef(context.Background(), root, *ref); err != nil || result != reviewtransaction.GateScopeChanged {
		t.Fatalf("generic bound receipt validation after settlement = %q, %v, want unchanged scope-changed", result, err)
	}
	runtime, err = store.Status()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		mutate func(*RuntimeStatus)
		counts SpecCounts
	}{
		{name: "missing attestation", mutate: func(status *RuntimeStatus) { status.Attempts[len(status.Attempts)-1].AttestedVerifyReportDigest = "" }, counts: SpecCounts{Requirements: 1, Scenarios: 1}},
		{name: "digest mismatch", mutate: func(status *RuntimeStatus) {
			status.Attempts[len(status.Attempts)-1].AttestedVerifyReportDigest = shaID("c")
		}, counts: SpecCounts{Requirements: 1, Scenarios: 1}},
		{name: "arbitrary work unit cannot own attestation", mutate: func(status *RuntimeStatus) {
			status.Attempts[len(status.Attempts)-1].WorkUnit = "post-correction-final-verification"
		}, counts: SpecCounts{Requirements: 1, Scenarios: 1}},
		{name: "report admission mismatch", mutate: func(*RuntimeStatus) {}, counts: SpecCounts{Requirements: 2, Scenarios: 1}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			forged := runtime
			forged.Attempts = append([]RuntimeAttempt(nil), runtime.Attempts...)
			tt.mutate(&forged)
			if classifyPostReviewVerifyReportAttestation(context.Background(), root, root, change, *ref, &forged, tt.counts) == postReviewVerifyReportBound {
				t.Fatal("invalid post-review verify-report attestation allowed archive")
			}
		})
	}

	// R4: a status poll (drifted, bound, or digest-less legacy) never writes a
	// Git object. The legacy case drifts the worktree report so any snapshot
	// build reached past the byte gate would hash the novel bytes into the ODB.
	legacy := runtime
	legacy.Attempts = append([]RuntimeAttempt(nil), runtime.Attempts...)
	legacy.Attempts[len(legacy.Attempts)-1].AttestedVerifyReportDigest = ""
	for _, tc := range []struct {
		report string
		status *RuntimeStatus
		want   postReviewVerifyReportAttestation
	}{
		{finalReport + "\n# drifted in the worktree only\n", &runtime, postReviewVerifyReportUnproven},
		{finalReport, &runtime, postReviewVerifyReportBound},
		{finalReport + "\n# drifted in the worktree only\n", &legacy, postReviewVerifyReportRequired},
	} {
		write(t, verifyPath, tc.report)
		before := countLooseGitObjects(t, root)
		if got := classifyPostReviewVerifyReportAttestation(context.Background(), root, root, change, *ref, tc.status, SpecCounts{Requirements: 1, Scenarios: 1}); got != tc.want {
			t.Fatalf("status classification = %v, want %v", got, tc.want)
		}
		if after := countLooseGitObjects(t, root); after != before {
			t.Fatalf("status classification wrote %d Git object(s)", after-before)
		}
	}

	// A digest-less settlement whose receipt-to-current delta also touches a
	// non-report path has nothing a verify-attestation work unit could repair:
	// it must keep the pre-existing scope-changed routing, never Required, and
	// proving that delta must still write no Git objects.
	tasksPath := filepath.Join(changeRoot, "tasks.md")
	originalTasks, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatal(err)
	}
	write(t, verifyPath, finalReport+"\n# drifted in the worktree only\n")
	write(t, tasksPath, string(originalTasks)+"- [ ] drifted unrelated edit\n")
	before := countLooseGitObjects(t, root)
	if got := classifyPostReviewVerifyReportAttestation(context.Background(), root, root, change, *ref, &legacy, SpecCounts{Requirements: 1, Scenarios: 1}); got != postReviewVerifyReportUnproven {
		t.Fatalf("legacy non-report delta classification = %v, want unproven", got)
	}
	if after := countLooseGitObjects(t, root); after != before {
		t.Fatalf("legacy non-report delta classification wrote %d Git object(s)", after-before)
	}
	write(t, tasksPath, string(originalTasks))

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

func countLooseGitObjects(t *testing.T, repo string) int {
	t.Helper()
	count := 0
	if err := filepath.WalkDir(filepath.Join(repo, ".git", "objects"), func(_ string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			count++
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return count
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
	if status.Dependencies.Archive != DependencyBlocked || status.NextRecommended != "resolve-review" {
		t.Fatalf("changed post-review status = %#v, want scope-changed archive block", status)
	}
}

func TestLegacyPostReviewVerifyReportWithArbitraryWorkUnitRequiresCurrentAttestation(t *testing.T) {
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
	if legacyStatus.ReviewGate != nil || legacyStatus.Dependencies.Verify != DependencyReady ||
		legacyStatus.Dependencies.Archive != DependencyBlocked || legacyStatus.NextRecommended != "verify" {
		t.Fatalf("legacy post-review status = %#v, want attestation-required verify routing without allow", legacyStatus)
	}
	if legacyStatus.PhaseInstructions == nil || !strings.Contains(strings.Join(legacyStatus.PhaseInstructions.Verify, "\n"), "verification attestation required") ||
		!strings.Contains(strings.Join(legacyStatus.PhaseInstructions.Verify, "\n"), "--work-unit \"verify-attestation\"") {
		t.Fatalf("legacy verification instructions = %#v, want distinct attestation continuation", legacyStatus.PhaseInstructions)
	}
	if result, _, err := reviewtransaction.ValidateSDDReceiptRef(context.Background(), root, *ref); err != nil || result != reviewtransaction.GateScopeChanged {
		t.Fatalf("generic legacy receipt validation = %q, %v, want unchanged scope-changed", result, err)
	}

	acquired, err := store.Acquire(context.Background(), CompactAcquireRequest{BeginAttemptRequest: BeginAttemptRequest{
		RequestID: "acquire-current-attestation", WorkUnit: "verify-attestation", EvidenceGoal: "attest final verification report",
		MaxAttempts: 1, MaxChangedLines: 20,
	}})
	if err != nil || acquired.State != CompactStateProceed || acquired.Token == "" {
		t.Fatalf("current attestation acquire = %#v, %v", acquired, err)
	}
	settled, err := store.Settle(context.Background(), CompactSettleRequest{
		Token: acquired.Token, RequestID: "settle-current-attestation", Outcome: AttemptPassed, EvidenceRevision: shaID("b"),
		Diagnosis: "current verification attested the report", HarnessDisposition: HarnessReused,
		CleanupEvidence: "no cleanup required", ProcessEvidence: "focused verification passed",
	})
	if err != nil || settled.State != CompactStateComplete {
		t.Fatalf("current attestation settle = %#v, %v", settled, err)
	}

	ready, err := Resolve(ResolveOptions{CWD: root, ChangeName: change})
	if err != nil {
		t.Fatal(err)
	}
	if ready.ReviewGate == nil || ready.ReviewGate.Result != reviewtransaction.GateAllow ||
		ready.Dependencies.Archive != DependencyReady || ready.NextRecommended != "archive" {
		t.Fatalf("current-attested legacy status = %#v, want archive-ready allow", ready)
	}
}

// TestFinalVerifySettlementDegradesUnattestableReportToEmptyAttestation proves
// attestation derivation never aborts settlement of a passing verify attempt:
// an unattestable report (here a non-passing verdict) settles with an empty
// attestation and the archive projection stays fail-closed downstream.
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

// TestBoundArchiveGateAttestsPostReviewReportWithWorkspaceBelowRepository
// mirrors TestFinalVerifySettlementAttestsReportAnchoredAtWorkspace on the
// status side: a workspace below its repository root still resolves the delta.
func TestBoundArchiveGateAttestsPostReviewReportWithWorkspaceBelowRepository(t *testing.T) {
	const change = "workspace-anchored-post-review-report"
	root := t.TempDir()
	workspace := filepath.Join(root, "service")
	changeRoot := seedReadyChange(t, workspace, change, "- [x] 1.1 Done\n")
	verifyPath := filepath.Join(changeRoot, "verify-report.md")
	write(t, verifyPath, boundedVerifyEnvelope(shaID("a"), "pass"))
	writeApprovedCompactAuthorityForChangeWithCandidate(t, root, changeRoot, "approved-workspace-post-review", "- [x] 1.1 Approved\n", nil)
	if _, err := BindApprovedReview(context.Background(), workspace, change, "approved-workspace-post-review", ""); err != nil {
		t.Fatal(err)
	}
	write(t, verifyPath, boundedVerifyEnvelope(shaID("b"), "pass"))
	runSDDStatusGit(t, root, "add", "-A")
	store := mustRuntimeStore(t, workspace, change)
	if store.Repo == store.Workspace {
		t.Fatalf("fixture must place the workspace below the repository root: repo=%q workspace=%q", store.Repo, store.Workspace)
	}
	runtime, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: runtime.Revision, RequestID: "begin-workspace-post-review", WorkUnit: "verify",
		EvidenceGoal: "run final independent verification", MaxAttempts: 1, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "settle-workspace-post-review", Outcome: AttemptPassed,
		EvidenceRevision: shaID("b"), Diagnosis: "final verification passed", HarnessDisposition: HarnessReused,
		CleanupEvidence: "no cleanup required", ProcessEvidence: "focused verification passed",
	}); err != nil {
		t.Fatal(err)
	}
	settled, err := Resolve(ResolveOptions{CWD: workspace, ChangeName: change})
	if err != nil {
		t.Fatal(err)
	}
	if settled.ReviewGate == nil || settled.ReviewGate.Result != reviewtransaction.GateAllow ||
		settled.Dependencies.Archive != DependencyReady || settled.NextRecommended != "archive" {
		t.Fatalf("workspace-below-repository attested status = %#v, want allow and archive-ready", settled)
	}
}
