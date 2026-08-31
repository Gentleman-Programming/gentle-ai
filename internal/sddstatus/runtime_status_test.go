package sddstatus

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestResolveExplainsFreshVerificationAfterEvidenceOnlyRuntimeRemediation(t *testing.T) {
	for _, storeKind := range []string{"openspec", "engram"} {
		t.Run(storeKind, func(t *testing.T) {
			fixture := newEvidenceOnlyRuntimeRemediationFixture(t, "post-remediation-"+storeKind)
			if storeKind == "openspec" {
				changeRoot := seedReadyChange(t, fixture.repo, fixture.change, "- [x] 1.1 Work\n")
				write(t, filepath.Join(changeRoot, "verify-report.md"), testVerifyEnvelope("pass", 0, 0, "1/1", "1/1", 0, 0))
			} else {
				if err := os.MkdirAll(filepath.Join(fixture.repo, ".engram"), 0o755); err != nil {
					t.Fatal(err)
				}
				runRuntimeLedgerGit(t, fixture.repo, "remote", "add", "origin", "git@github.com:Gentleman-Programming/gentle-ai.git")
				restore := stubEngramExport(t, []engramObservation{
					{Title: "sdd/" + fixture.change + "/proposal", Content: "## Proposal\n", Project: "gentle-ai", Scope: "project"},
					{Title: "sdd/" + fixture.change + "/spec", Content: "### Requirement: Runtime\n#### Scenario: Fresh evidence\n", Project: "gentle-ai", Scope: "project"},
					{Title: "sdd/" + fixture.change + "/design", Content: "## Design\n", Project: "gentle-ai", Scope: "project"},
					{Title: "sdd/" + fixture.change + "/tasks", Content: "- [x] 1.1 Work\n", Project: "gentle-ai", Scope: "project"},
					{Title: "sdd/" + fixture.change + "/verify-report", Content: testVerifyEnvelope("pass", 0, 0, "1/1", "1/1", 0, 0), Project: "gentle-ai", Scope: "project"},
				})
				t.Cleanup(restore)
			}

			completed := fixture.settle(t)
			last := completed.Attempts[len(completed.Attempts)-1]
			if last.FinishCandidateTree != last.BeginCandidateTree {
				t.Fatalf("remediation changed the candidate: %#v", last)
			}

			status, err := Resolve(ResolveOptions{CWD: fixture.repo, ChangeName: fixture.change, ReviewDisabled: true, IncludeInstructions: true})
			if err != nil {
				t.Fatal(err)
			}
			if status.Dependencies.Verify != DependencyReady || status.Dependencies.Archive != DependencyBlocked || status.NextRecommended != "verify" {
				t.Fatalf("post-remediation routing: verify=%q archive=%q next=%q", status.Dependencies.Verify, status.Dependencies.Archive, status.NextRecommended)
			}
			const want = "A passing native remediation settlement completed after the persisted verification report; run fresh verification and persist a report bound after that settlement before archive."
			// The refresh obligation must reach blockedReasons, not only the
			// opt-in phase instructions, so a status without --instructions
			// never projects a silent verify-ready tuple (#3538).
			if occurrences := strings.Count(strings.Join(status.BlockedReasons, "\n"), want); occurrences != 1 {
				t.Fatalf("BlockedReasons = %v, want the post-remediation refresh instruction exactly once, found %d", status.BlockedReasons, occurrences)
			}
			if status.PhaseInstructions == nil {
				t.Fatal("PhaseInstructions is nil")
			}
			if instructions := strings.Join(status.PhaseInstructions.Verify, "\n"); !strings.Contains(instructions, want) {
				t.Fatalf("verify instructions omit the post-remediation fresh-report obligation:\n%s", instructions)
			}
		})
	}
}

func TestResolveVerifyInstructionsDoNotMislabelOtherRoutes(t *testing.T) {
	const postRemediationInstruction = "A passing native remediation settlement completed after the persisted verification report"

	t.Run("ordinary first verification", func(t *testing.T) {
		repo := t.TempDir()
		seedReadyChange(t, repo, "first-verify", "- [x] 1.1 Work\n")

		status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "first-verify", ReviewDisabled: true, IncludeInstructions: true})
		if err != nil {
			t.Fatal(err)
		}
		if status.Dependencies.Verify != DependencyReady || status.PhaseInstructions == nil {
			t.Fatalf("ordinary first verification status = %#v", status)
		}
		if strings.Contains(strings.Join(status.PhaseInstructions.Verify, "\n"), postRemediationInstruction) {
			t.Fatalf("ordinary first verification claimed remediation: %v", status.PhaseInstructions.Verify)
		}
	})

	t.Run("fresh all done pass with warnings", func(t *testing.T) {
		repo := t.TempDir()
		changeRoot := seedReadyChange(t, repo, "fresh-verify", "- [x] 1.1 Work\n")
		write(t, filepath.Join(changeRoot, "verify-report.md"), testVerifyEnvelope("pass_with_warnings", 0, 0, "1/1", "1/1", 0, 0))

		status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "fresh-verify", ReviewDisabled: true, IncludeInstructions: true})
		if err != nil {
			t.Fatal(err)
		}
		if status.Dependencies.Verify != DependencyAllDone || status.Dependencies.Archive != DependencyReady || status.NextRecommended != "archive" {
			t.Fatalf("fresh passing report status = %#v", status)
		}
		if strings.Contains(strings.Join(status.PhaseInstructions.Verify, "\n"), postRemediationInstruction) {
			t.Fatalf("fresh passing report claimed remediation: %v", status.PhaseInstructions.Verify)
		}
	})
}

func TestMissingEvidenceRevisionPreservesStrictParserReasonWithoutAuthorityComparison(t *testing.T) {
	report := strings.ReplaceAll(
		testVerifyEnvelope("fail", 1, 1, "1/1", "1/1", 0, 0),
		"evidence_revision: sha256:"+strings.Repeat("a", 64)+"\n", "",
	)
	verify := parseVerifyResult(report, SpecCounts{Requirements: 1, Scenarios: 1})
	remediation := resolveBoundedRemediation(true, verify, "")
	const want = "verify evidence cannot enter remediation: verification evidence is incomplete: missing evidence_revision in verify result envelope"
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

type evidenceOnlyRuntimeRemediationFixture struct {
	repo           string
	change         string
	store          RuntimeStore
	failedEvidence string
	active         RuntimeStatus
}

func newEvidenceOnlyRuntimeRemediationFixture(t *testing.T, change string) evidenceOnlyRuntimeRemediationFixture {
	t.Helper()
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, change)
	store.ReviewDisabled = true
	first, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: change + "-begin-failed-verification", WorkUnit: "verify",
		EvidenceGoal: "independent verification", MaxAttempts: 1, MaxChangedLines: DefaultRuntimeChangedLines,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('a')
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: first.Revision, RequestID: change + "-finish-failed-verification", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "transient verification failure unrelated to the candidate",
		HarnessDisposition: HarnessReused, CleanupEvidence: "verification cleanup completed",
		ProcessEvidence: "verification process scan completed",
	})
	if err != nil {
		t.Fatal(err)
	}
	reset, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: change + "-reset", Actor: "maintainer",
		Reason: "authorize one evidence-only verification retry",
	})
	if err != nil {
		t.Fatal(err)
	}
	active, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: reset.Revision, RequestID: change + "-begin-remediation", WorkUnit: "verify",
		EvidenceGoal: "independent verification", MaxAttempts: 1, MaxChangedLines: DefaultRuntimeChangedLines,
	})
	if err != nil {
		t.Fatal(err)
	}
	return evidenceOnlyRuntimeRemediationFixture{repo: repo, change: change, store: store, failedEvidence: failedEvidence, active: active}
}

func (fixture evidenceOnlyRuntimeRemediationFixture) settle(t *testing.T) RuntimeStatus {
	t.Helper()
	completed, err := fixture.store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: fixture.active.Revision, RequestID: fixture.change + "-settle-remediation", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "verification passed against the unchanged candidate",
		HarnessDisposition: HarnessReused, CleanupEvidence: "retry cleanup completed",
		ProcessEvidence: "retry process scan completed", RemediatesEvidenceRevision: fixture.failedEvidence,
		// The change artifacts this fixture writes during the attempt are SDD
		// bookkeeping, not the work unit's product, and this scenario turns on
		// the candidate staying byte-identical. Excluding them is now an
		// explicit recorded decision rather than a silent omission (#3806).
		IntendedUntracked: &[]string{}, ExpectedUntrackedInventory: currentUntrackedInventoryDigest(t, fixture.repo),
	})
	if err != nil {
		t.Fatal(err)
	}
	return completed
}
