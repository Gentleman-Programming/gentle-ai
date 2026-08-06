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

func TestResolveEmbedsAndRoutesNativeRuntimeAuthority(t *testing.T) {
	t.Run("OpenSpec active attempt names its own continuation without stopping the caller", func(t *testing.T) {
		repo := initRuntimeLedgerRepo(t)
		seedReadyChange(t, repo, "active-runtime", "- [ ] 1.1 Work\n")
		store := mustRuntimeStore(t, repo, "active-runtime")
		active, err := store.Begin(context.Background(), BeginAttemptRequest{
			ExpectedRevision: "", RequestID: "begin-active-runtime", WorkUnit: "apply-auth",
			EvidenceGoal: "prove the auth runtime", MaxAttempts: 2, MaxChangedLines: 20,
		})
		if err != nil {
			t.Fatal(err)
		}

		status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "active-runtime", IncludeInstructions: true})
		if err != nil {
			t.Fatal(err)
		}
		assertRuntimeStatusRevision(t, status, active.Revision)
		if status.RuntimeStatus.ActiveAttempt == nil || status.RuntimeStatus.ActiveAttempt.Ordinal != 1 {
			t.Fatalf("runtime status = %#v, want active ordinal 1", status.RuntimeStatus)
		}
		assertRuntimeContinuationOffered(t, status, active.Revision)
		if instructions := strings.Join(status.PhaseInstructions.Apply, "\n"); !strings.Contains(instructions, "gentle-ai sdd-attempt acquire") ||
			!strings.Contains(instructions, "gentle-ai sdd-attempt settle") {
			t.Fatalf("apply instructions omit native runtime commands:\n%s", instructions)
		}
	})

	t.Run("decision required blocks execution until explicit reset", func(t *testing.T) {
		repo := initRuntimeLedgerRepo(t)
		seedReadyChange(t, repo, "exhausted-runtime", "- [ ] 1.1 Work\n")
		store := mustRuntimeStore(t, repo, "exhausted-runtime")
		active, err := store.Begin(context.Background(), BeginAttemptRequest{
			ExpectedRevision: "", RequestID: "begin-exhausted-runtime", WorkUnit: "apply-auth",
			EvidenceGoal: "prove the auth runtime", MaxAttempts: 1, MaxChangedLines: 20,
		})
		if err != nil {
			t.Fatal(err)
		}
		exhausted, err := store.Finish(context.Background(), FinishAttemptRequest{
			ExpectedRevision: active.Revision, RequestID: "finish-exhausted-runtime", Outcome: AttemptFailed,
			EvidenceRevision: runtimeTestHash('7'), Diagnosis: "bounded runtime reproduced the failure",
			HarnessDisposition: HarnessReused, CleanupEvidence: "runtime process group exited",
			ProcessEvidence: "post-run scan found no descendants",
		})
		if err != nil {
			t.Fatal(err)
		}
		if !exhausted.DecisionRequired {
			t.Fatalf("runtime status = %#v, want decision_required", exhausted)
		}

		status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "exhausted-runtime", IncludeInstructions: true})
		if err != nil {
			t.Fatal(err)
		}
		assertRuntimeStatusRevision(t, status, exhausted.Revision)
		assertRuntimeContinuationBlocked(t, status, "blocked(maintainer_decision)")
	})

	t.Run("completed objective remains visible without blocking phase progress", func(t *testing.T) {
		repo := initRuntimeLedgerRepo(t)
		seedReadyChange(t, repo, "complete-runtime", "- [ ] 1.1 Work\n")
		store := mustRuntimeStore(t, repo, "complete-runtime")
		active, err := store.Begin(context.Background(), BeginAttemptRequest{
			ExpectedRevision: "", RequestID: "begin-complete-runtime", WorkUnit: "apply-auth",
			EvidenceGoal: "prove the auth runtime", MaxAttempts: 2, MaxChangedLines: 20,
		})
		if err != nil {
			t.Fatal(err)
		}
		complete, err := store.Finish(context.Background(), FinishAttemptRequest{
			ExpectedRevision: active.Revision, RequestID: "finish-complete-runtime", Outcome: AttemptPassed,
			EvidenceRevision: runtimeTestHash('8'), Diagnosis: "bounded runtime passed",
			HarnessDisposition: HarnessReused, CleanupEvidence: "runtime process group exited",
			ProcessEvidence: "post-run scan found no descendants",
		})
		if err != nil {
			t.Fatal(err)
		}

		status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "complete-runtime"})
		if err != nil {
			t.Fatal(err)
		}
		assertRuntimeStatusRevision(t, status, complete.Revision)
		if !status.RuntimeStatus.Complete || status.NextRecommended != "apply" || status.Dependencies.Apply != DependencyReady {
			t.Fatalf("completed runtime routing = %#v", status)
		}
	})
}

func TestResolveEngramUsesTheSameNativeRuntimeAuthority(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".engram"), 0o755); err != nil {
		t.Fatal(err)
	}
	runRuntimeLedgerGit(t, repo, "remote", "add", "origin", "git@github.com:Gentleman-Programming/gentle-ai.git")
	restore := stubEngramExport(t, []engramObservation{
		{Title: "sdd/engram-runtime/proposal", Content: "## Proposal\nNative runtime", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/engram-runtime/spec", Content: "### Requirement: Runtime\n#### Scenario: Native authority\n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/engram-runtime/design", Content: "## Design\nUse the native ledger", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/engram-runtime/tasks", Content: "- [ ] 1.1 Work\n", Project: "gentle-ai", Scope: "project"},
	})
	defer restore()

	store := mustRuntimeStore(t, repo, "engram-runtime")
	active, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "begin-engram-runtime", WorkUnit: "engram-apply",
		EvidenceGoal: "prove artifact-agnostic authority", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "engram-runtime", IncludeInstructions: true})
	if err != nil {
		t.Fatal(err)
	}
	if status.ArtifactStore != ArtifactStoreEngram {
		t.Fatalf("artifact store = %q, want engram", status.ArtifactStore)
	}
	assertRuntimeStatusRevision(t, status, active.Revision)
	assertRuntimeContinuationOffered(t, status, active.Revision)
	payload, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"attemptLedger", "attempt-ledger", "gentle-ai.sdd-attempt-ledger/v1"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("Engram status leaked mutable runtime artifact %q: %s", forbidden, payload)
		}
	}
}

func TestResolveRoutesAtomicRuntimeRemediationSuccessorToFreshVerify(t *testing.T) {
	fixture := newRuntimeRemediationFixture(t, true)
	completed, err := fixture.store.Finish(context.Background(), fixture.finishRequest("finish-remediation-for-status"))
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(fixture.changeRoot, "verify-report.md"), boundedVerifyEnvelope(fixture.failedEvidence, "fail"))

	status, err := Resolve(ResolveOptions{CWD: fixture.repo, ChangeName: "runtime-remediation"})
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeStatusRevision(t, status, completed.Revision)
	if status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateAllow {
		t.Fatalf("review gate = %#v, want live successor allow", status.ReviewGate)
	}
	if status.Dependencies.Verify != DependencyReady || status.Dependencies.Archive != DependencyBlocked || status.NextRecommended != "verify" {
		t.Fatalf("atomic remediation routing: verify=%q archive=%q next=%q reasons=%v", status.Dependencies.Verify, status.Dependencies.Archive, status.NextRecommended, status.BlockedReasons)
	}
	if status.RemediationState != (RemediationState{}) {
		t.Fatalf("remediation state = %#v, want completed native successor", status.RemediationState)
	}
}

func TestResolveRoutesPureEngramRuntimeRemediationSuccessorToFreshVerify(t *testing.T) {
	fixture := newRuntimeEngramRemediationFixture(t)
	completed, err := fixture.store.Finish(context.Background(), fixture.finishRequest("finish-engram-remediation-for-status"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(fixture.repo, ".engram"), 0o755); err != nil {
		t.Fatal(err)
	}
	runRuntimeLedgerGit(t, fixture.repo, "remote", "add", "origin", "git@github.com:Gentleman-Programming/gentle-ai.git")
	restore := stubEngramExport(t, []engramObservation{
		{Title: "sdd/engram-runtime/proposal", Content: "## Proposal\nNative runtime", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/engram-runtime/spec", Content: "### Requirement: Runtime\n#### Scenario: Native authority\n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/engram-runtime/design", Content: "## Design\nUse the native ledger", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/engram-runtime/tasks", Content: "- [x] 1.1 Work\n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/engram-runtime/verify-report", Content: boundedVerifyEnvelope(fixture.failedEvidence, "fail"), Project: "gentle-ai", Scope: "project"},
	})
	defer restore()

	status, err := Resolve(ResolveOptions{CWD: fixture.repo, ChangeName: "engram-runtime"})
	if err != nil {
		t.Fatal(err)
	}
	if status.ArtifactStore != ArtifactStoreEngram {
		t.Fatalf("artifact store = %q, want Engram", status.ArtifactStore)
	}
	assertRuntimeStatusRevision(t, status, completed.Revision)
	if status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateAllow {
		t.Fatalf("Engram review gate = %#v, want live successor allow", status.ReviewGate)
	}
	if status.Dependencies.Verify != DependencyReady || status.Dependencies.Archive != DependencyBlocked || status.NextRecommended != "verify" {
		t.Fatalf("Engram remediation routing: verify=%q archive=%q next=%q reasons=%v", status.Dependencies.Verify, status.Dependencies.Archive, status.NextRecommended, status.BlockedReasons)
	}
	if status.RemediationState != (RemediationState{}) {
		t.Fatalf("Engram remediation state = %#v, want completed native successor", status.RemediationState)
	}
	if _, err := os.Stat(filepath.Join(fixture.repo, "openspec")); !os.IsNotExist(err) {
		t.Fatalf("pure Engram routing depended on an OpenSpec root: %v", err)
	}
}

func TestResolveDoesNotBypassMalformedFailedEvidenceWithACompletedRuntime(t *testing.T) {
	fixture := newRuntimeRemediationFixture(t, true)
	if _, err := fixture.store.Finish(context.Background(), fixture.finishRequest("finish-remediation-malformed-evidence")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(fixture.changeRoot, "verify-report.md"), strings.ReplaceAll(
		boundedVerifyEnvelope(fixture.failedEvidence, "fail"),
		"evidence_revision: "+fixture.failedEvidence+"\n", "",
	))

	status, err := Resolve(ResolveOptions{CWD: fixture.repo, ChangeName: "runtime-remediation"})
	if err != nil {
		t.Fatal(err)
	}
	if status.NextRecommended == "verify" || status.Dependencies.Verify == DependencyReady {
		t.Fatalf("malformed failed evidence bypassed strict parser: %#v", status)
	}
	if !strings.Contains(strings.Join(status.BlockedReasons, "\n"), "evidence_revision") {
		t.Fatalf("blocked reasons = %v, want preserved evidence_revision diagnostic", status.BlockedReasons)
	}
}

func TestMissingEvidenceRevisionPreservesStrictParserReasonBeforeLegacyTransactionComparison(t *testing.T) {
	report := strings.ReplaceAll(
		testVerifyEnvelope("fail", 1, 1, "1/1", "1/1", 0, 0),
		"evidence_revision: sha256:"+strings.Repeat("a", 64)+"\n", "",
	)
	verify := parseVerifyResult(report, SpecCounts{Requirements: 1, Scenarios: 1})
	transaction := &reviewtransaction.Transaction{FailedEvidenceRevision: runtimeTestHash('e')}
	remediation := resolveBoundedRemediation(true, false, verify, transaction, nil, "", "")
	const want = "verify evidence cannot enter remediation: missing evidence_revision in verify result envelope"
	if remediation.Reason != want {
		t.Fatalf("missing evidence remediation reason = %q, want %q", remediation.Reason, want)
	}
}

func TestResolveFailsClosedOnCorruptNativeRuntimeAuthority(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	seedReadyChange(t, repo, "corrupt-runtime", "- [ ] 1.1 Work\n")
	store := mustRuntimeStore(t, repo, "corrupt-runtime")
	if _, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "begin-corrupt-runtime", WorkUnit: "apply",
		EvidenceGoal: "prove corruption fails closed", MaxAttempts: 2, MaxChangedLines: 20,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "HEAD"), []byte("corrupt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "corrupt-runtime"})
	if err != nil {
		t.Fatal(err)
	}
	if status.NextRecommended != "resolve-blockers" || status.Dependencies.Apply != DependencyBlocked ||
		!strings.Contains(strings.Join(status.BlockedReasons, "\n"), "native SDD runtime authority is unreadable") {
		t.Fatalf("corrupt runtime did not fail closed with structured recovery: %#v", status)
	}
}

func TestResolveFailsClosedWhenGitMetadataCannotResolveNativeAuthority(t *testing.T) {
	root := t.TempDir()
	seedReadyChange(t, root, "unresolved-runtime-root", "- [ ] 1.1 Work\n")
	write(t, filepath.Join(root, ".git", "config"), "[core]\n\trepositoryformatversion = 0\n")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "unresolved-runtime-root"})
	if err != nil {
		t.Fatal(err)
	}
	if status.NextRecommended != "resolve-blockers" || status.Dependencies.Apply != DependencyBlocked ||
		!strings.Contains(strings.Join(status.BlockedReasons, "\n"), "native SDD runtime authority is unreadable") {
		t.Fatalf("unresolved Git authority did not fail closed: %#v", status)
	}
}

func assertRuntimeStatusRevision(t *testing.T, status Status, revision string) {
	t.Helper()
	if status.RuntimeStatus == nil || status.RuntimeStatus.Revision != revision {
		t.Fatalf("RuntimeStatus = %#v, want revision %q", status.RuntimeStatus, revision)
	}
}

// assertRuntimeContinuationOffered is the active-attempt counterpart of
// assertRuntimeContinuationBlocked after #2463. A live attempt is not a stop:
// compact acquire admits the holder of that attempt's token, so status names
// the same continuation acquire names and leaves routing to the artifacts.
func assertRuntimeContinuationOffered(t *testing.T, status Status, activeToken string) {
	t.Helper()
	if status.NextRecommended == "resolve-blockers" || status.Dependencies.Apply == DependencyBlocked {
		t.Fatalf("live attempt stopped a caller compact acquire admits: next=%q dependencies=%#v blocked=%v",
			status.NextRecommended, status.Dependencies, status.BlockedReasons)
	}
	if reasons := strings.Join(status.BlockedReasons, "\n"); strings.Contains(reasons, "active_attempt") {
		t.Fatalf("live attempt was published as a blocker: %v", status.BlockedReasons)
	}
	if guidance := activeAttemptGuidance(t, status); !strings.Contains(guidance, "--token "+activeToken) {
		t.Fatalf("apply instructions omit the live attempt's own token continuation:\n%s", guidance)
	}
}

func assertRuntimeContinuationBlocked(t *testing.T, status Status, command string) {
	t.Helper()
	if status.NextRecommended != "resolve-blockers" || status.Dependencies.Apply != DependencyBlocked ||
		status.Dependencies.Verify != DependencyBlocked || status.Dependencies.Archive != DependencyBlocked {
		t.Fatalf("runtime continuation was not blocked: next=%q dependencies=%#v", status.NextRecommended, status.Dependencies)
	}
	if !strings.Contains(strings.Join(status.BlockedReasons, "\n"), command) {
		t.Fatalf("blocked reasons = %v, want executable %s guidance", status.BlockedReasons, command)
	}
}
