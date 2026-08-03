package sddstatus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeLedgerUnmanagedFailedEvidenceRemediationIsOneExactAttempt(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "unmanaged-remediation")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "unmanaged-begin-failed", WorkUnit: "verify-runtime",
		EvidenceGoal: "prove the failed runtime", MaxAttempts: 1, MaxChangedLines: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('a')
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "unmanaged-finish-failed", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "admitted substantive verification failure",
		HarnessDisposition: HarnessReused, CleanupEvidence: "failed verification cleanup completed",
		ProcessEvidence: "failed verification process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	last := failed.Attempts[len(failed.Attempts)-1]
	authorization := RenderUnmanagedRemediationAuthorization(
		failed.Revision, store.Change, failed.Objective.ID, failed.Objective.Generation, failedEvidence,
		last.FinishCandidateIdentity, last.FinishCandidateTree, "correct-verification", "prove corrected runtime", 20,
		"maintainer@example.com", "one bounded correction is authorized",
	)
	store.ReviewDisabled = true
	request := ResetObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "authorize-unmanaged-correction", Disposition: ResetDispositionFailedEvidenceRemediation,
		RemediatesEvidenceRevision: failedEvidence, WorkUnit: "correct-verification", EvidenceGoal: "prove corrected runtime",
		MaxChangedLines: 20, MaintainerAuthorization: authorization,
	}
	for _, mismatch := range []ResetObjectiveRequest{
		{ExpectedRevision: failed.Revision, RequestID: "wrong-failed-evidence", Disposition: ResetDispositionFailedEvidenceRemediation, RemediatesEvidenceRevision: runtimeTestHash('f'), WorkUnit: "correct-verification", EvidenceGoal: "prove corrected runtime", MaxChangedLines: 20, MaintainerAuthorization: authorization},
		{ExpectedRevision: failed.Revision, RequestID: "wrong-scope", Disposition: ResetDispositionFailedEvidenceRemediation, RemediatesEvidenceRevision: failedEvidence, WorkUnit: "renamed-correction", EvidenceGoal: "prove corrected runtime", MaxChangedLines: 20, MaintainerAuthorization: authorization},
	} {
		if _, err := store.Reset(context.Background(), mismatch); err == nil {
			t.Fatalf("mismatched authorization %+v unexpectedly succeeded", mismatch)
		}
	}
	store.ReviewDisabled = false
	if _, err := store.Reset(context.Background(), request); err == nil {
		t.Fatal("re-enabled review unexpectedly authorized unmanaged remediation")
	}
	store.ReviewDisabled = true
	authorized, err := store.Reset(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if authorized.Objective == nil || authorized.Objective.Generation != 2 || authorized.Objective.MaxAttempts != 1 ||
		authorized.UnmanagedRemediation == nil || authorized.LifetimeAttempts != 1 || len(authorized.Attempts) != 1 {
		t.Fatalf("authorized remediation = %#v", authorized)
	}
	replayed, err := store.Reset(context.Background(), request)
	if err != nil || replayed.Revision != authorized.Revision {
		t.Fatalf("exact authorization replay = %#v err=%v", replayed, err)
	}
	changedRequest := request
	changedRequest.MaxChangedLines = 21
	if _, err := store.Reset(context.Background(), changedRequest); !errors.Is(err, ErrRuntimeRequestConflict) {
		t.Fatalf("changed authorization replay = %v, want request conflict", err)
	}

	wrongScope := BeginAttemptRequest{ExpectedRevision: authorized.Revision, RequestID: "unmanaged-wrong-begin", WorkUnit: "renamed-work", EvidenceGoal: "prove corrected runtime", MaxAttempts: 1, MaxChangedLines: 20}
	if _, err := store.Begin(context.Background(), wrongScope); !errors.Is(err, ErrRuntimeObjectiveChange) {
		t.Fatalf("renamed remediation scope = %v, want objective change refusal", err)
	}
	store.ReviewDisabled = false
	if _, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: authorized.Revision, RequestID: "unmanaged-reenabled-begin", WorkUnit: "correct-verification",
		EvidenceGoal: "prove corrected runtime", MaxAttempts: 1, MaxChangedLines: 20,
	}); err == nil {
		t.Fatal("re-enabled review unexpectedly accepted unmanaged remediation")
	}
	store.ReviewDisabled = true
	correction, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: authorized.Revision, RequestID: "unmanaged-begin-correction", WorkUnit: "correct-verification",
		EvidenceGoal: "prove corrected runtime", MaxAttempts: 1, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "bounded correction\n")
	store.ReviewDisabled = false
	if _, err := store.Finish(context.Background(), FinishAttemptRequest{ExpectedRevision: correction.Revision, RequestID: "unmanaged-reenabled-finish", Outcome: AttemptPassed, EvidenceRevision: runtimeTestHash('b'), Diagnosis: "corrected candidate passed the bounded execution", HarnessDisposition: HarnessInvalidated, CleanupEvidence: "corrected process cleanup completed", ProcessEvidence: "corrected process scan found no descendants"}); err == nil {
		t.Fatal("re-enabled review unexpectedly finished unmanaged remediation")
	}
	store.ReviewDisabled = true
	completed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: correction.Revision, RequestID: "unmanaged-finish-correction", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('b'), Diagnosis: "corrected candidate passed the bounded execution",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "corrected process cleanup completed",
		ProcessEvidence: "corrected process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !completed.Complete || completed.ActiveAttempt != nil || completed.LifetimeAttempts != 2 || len(completed.Attempts) != 2 ||
		completed.Attempts[1].ChangedLines == 0 || completed.Binding != nil {
		t.Fatalf("completed unmanaged correction = %#v", completed)
	}
	if exists, err := bindingExists(context.Background(), repo, store.Change); err != nil || exists {
		t.Fatalf("unmanaged correction created a review binding: exists=%v err=%v", exists, err)
	}
	if _, err := discoverNativeReceipts(context.Background(), repo); !errors.Is(err, errTerminalReceiptMissing) {
		t.Fatalf("unmanaged correction created a native review receipt or approval: %v", err)
	}
	if _, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: completed.Revision, RequestID: "rename-consumed-correction", Disposition: ResetDispositionFailedEvidenceRemediation,
		RemediatesEvidenceRevision: failedEvidence, WorkUnit: "second-correction", EvidenceGoal: "prove another correction", MaxChangedLines: 20,
		MaintainerAuthorization: authorization,
	}); err == nil {
		t.Fatal("consumed unmanaged authorization unexpectedly opened another correction")
	}
}

func TestUnmanagedRemediationAuthorizationRejectsMalformedBindings(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "unmanaged-auth-parser")
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "parser-begin", WorkUnit: "verify-runtime",
		EvidenceGoal: "prove failed runtime", MaxAttempts: 1, MaxChangedLines: 40,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('c')
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "parser-finish", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "admitted substantive verification failure",
		HarnessDisposition: HarnessReused, CleanupEvidence: "failed verification cleanup completed",
		ProcessEvidence: "failed verification process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	last := failed.Attempts[len(failed.Attempts)-1]
	authorization := RenderUnmanagedRemediationAuthorization(
		failed.Revision, store.Change, failed.Objective.ID, failed.Objective.Generation, failedEvidence,
		last.FinishCandidateIdentity, last.FinishCandidateTree, "correct-verification", "prove corrected runtime", 20,
		"maintainer@example.com", "one bounded correction is authorized",
	)
	store.ReviewDisabled = true
	request := ResetObjectiveRequest{
		ExpectedRevision: failed.Revision, Disposition: ResetDispositionFailedEvidenceRemediation,
		RemediatesEvidenceRevision: failedEvidence, WorkUnit: "correct-verification", EvidenceGoal: "prove corrected runtime",
		MaxChangedLines: 20,
	}
	for _, test := range []struct {
		name   string
		mutate func(string) string
	}{
		{name: "carriage-return", mutate: func(value string) string { return strings.Replace(value, "\n", "\r\n", 1) }},
		{name: "nul-byte", mutate: func(value string) string { return value + "\x00" }},
		{name: "oversized", mutate: func(string) string { return strings.Repeat("x", 4097) }},
		{name: "duplicate-field", mutate: func(value string) string { return value + "\nactor: maintainer@example.com" }},
		{name: "missing-field", mutate: func(value string) string { return strings.Replace(value, "actor: maintainer@example.com\n", "", 1) }},
		{name: "wrong-field-count", mutate: func(value string) string { return value + "\nextra: field" }},
		{name: "wrong-schema", mutate: func(value string) string { return strings.Replace(value, "/v1", "/v2", 1) }},
		{name: "enabled-delivery", mutate: func(value string) string { return strings.Replace(value, "disabled/unmanaged", "enabled", 1) }},
		{name: "signed-number", mutate: func(value string) string {
			return strings.Replace(value, "max_changed_lines: 20", "max_changed_lines: +20", 1)
		}},
		{name: "padded-number", mutate: func(value string) string {
			return strings.Replace(value, "max_changed_lines: 20", "max_changed_lines: 020", 1)
		}},
		{name: "out-of-range-lines", mutate: func(value string) string {
			return strings.Replace(value, "max_changed_lines: 20", "max_changed_lines: 1000001", 1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			bad := request
			bad.RequestID = "reject-" + test.name
			bad.MaintainerAuthorization = test.mutate(authorization)
			if _, err := store.Reset(context.Background(), bad); err == nil {
				t.Fatalf("%s authorization was accepted", test.name)
			}
		})
	}
}

func TestNormalizeResetObjectiveRequestNamesStrayRemediationField(t *testing.T) {
	_, err := normalizeResetObjectiveRequest(ResetObjectiveRequest{
		ExpectedRevision: runtimeTestHash('d'), RequestID: "stray-remediation-field", WorkUnit: "correct-verification",
	})
	if err == nil || !strings.Contains(err.Error(), "work_unit") {
		t.Fatalf("stray remediation field error = %v, want work_unit diagnostic", err)
	}
}

func TestUnmanagedRemediationSharesOneAuthorizationAcrossLinkedWorktrees(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "linked-unmanaged")
	started, err := store.Begin(context.Background(), BeginAttemptRequest{ExpectedRevision: "", RequestID: "linked-begin", WorkUnit: "verify", EvidenceGoal: "prove failed evidence", MaxAttempts: 1, MaxChangedLines: 20})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('9')
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{ExpectedRevision: started.Revision, RequestID: "linked-finish", Outcome: AttemptFailed, EvidenceRevision: failedEvidence, Diagnosis: "admitted failure", HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants"})
	if err != nil {
		t.Fatal(err)
	}
	last := failed.Attempts[0]
	authorization := RenderUnmanagedRemediationAuthorization(failed.Revision, store.Change, failed.Objective.ID, failed.Objective.Generation, failedEvidence, last.FinishCandidateIdentity, last.FinishCandidateTree, "linked-correction", "prove one correction", 20, "maintainer@example.com", "one correction")
	request := ResetObjectiveRequest{ExpectedRevision: failed.Revision, RequestID: "linked-authorize", Disposition: ResetDispositionFailedEvidenceRemediation, RemediatesEvidenceRevision: failedEvidence, WorkUnit: "linked-correction", EvidenceGoal: "prove one correction", MaxChangedLines: 20, MaintainerAuthorization: authorization}
	linked := filepath.Join(t.TempDir(), "linked")
	runRuntimeLedgerGit(t, repo, "worktree", "add", "-q", "--detach", linked)
	t.Cleanup(func() { runRuntimeLedgerGit(t, repo, "worktree", "remove", "--force", linked) })
	store.ReviewDisabled = true
	authorized, err := store.Reset(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	other := mustRuntimeStore(t, linked, "linked-unmanaged")
	other.ReviewDisabled = true
	replayed, err := other.Reset(context.Background(), request)
	if err != nil || replayed.Revision != authorized.Revision || len(replayed.Attempts) != 1 {
		t.Fatalf("linked exact replay = %#v err=%v", replayed, err)
	}
	changed := request
	changed.MaxChangedLines = 21
	if _, err := other.Reset(context.Background(), changed); !errors.Is(err, ErrRuntimeRequestConflict) {
		t.Fatalf("linked changed replay = %v, want request conflict", err)
	}
}

func TestResolveRoutesDisabledSubstantiveFailToUnmanagedAuthorization(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	changeRoot := seedReadyChange(t, repo, "disabled-failed-evidence", "- [x] 1.1 Work\n")
	store := mustRuntimeStore(t, repo, "disabled-failed-evidence")
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "disabled-fail-begin", WorkUnit: "verify-runtime",
		EvidenceGoal: "prove admitted failure", MaxAttempts: 1, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('c')
	write(t, filepath.Join(changeRoot, "verify-report.md"), strings.Replace(
		boundedVerifyEnvelope(failedEvidence, "fail"), "blockers: 0", "blockers: 1", 1))
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "disabled-fail-finish", Outcome: AttemptFailed,
		EvidenceRevision: failedEvidence, Diagnosis: "substantive verification failure was admitted",
		HarnessDisposition: HarnessReused, CleanupEvidence: "failure cleanup completed",
		ProcessEvidence: "failure process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "disabled-failed-evidence", ReviewDisabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if status.NextRecommended != RuntimeActionAuthorizeRemediation || status.ReviewGate == nil ||
		status.ReviewGate.Delivery != "disabled/unmanaged" || status.RuntimeStatus == nil || status.RuntimeStatus.Revision != failed.Revision {
		t.Fatalf("disabled failure routing = %#v", status)
	}
	last := failed.Attempts[len(failed.Attempts)-1]
	store.ReviewDisabled = true
	authorized, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: failed.Revision, RequestID: "disabled-authorize", Disposition: ResetDispositionFailedEvidenceRemediation,
		RemediatesEvidenceRevision: failedEvidence, WorkUnit: "correct-failed-verify", EvidenceGoal: "prove fixed verification", MaxChangedLines: 20,
		MaintainerAuthorization: RenderUnmanagedRemediationAuthorization(failed.Revision, store.Change, failed.Objective.ID, failed.Objective.Generation,
			failedEvidence, last.FinishCandidateIdentity, last.FinishCandidateTree, "correct-failed-verify", "prove fixed verification", 20,
			"maintainer@example.com", "one bounded correction is authorized"),
	})
	if err != nil {
		t.Fatal(err)
	}
	active, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: authorized.Revision, RequestID: "disabled-correction-begin", WorkUnit: "correct-failed-verify",
		EvidenceGoal: "prove fixed verification", MaxAttempts: 1, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendRuntimeLedgerFile(t, repo, "disabled bounded correction\n")
	if _, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: active.Revision, RequestID: "disabled-correction-finish", Outcome: AttemptPassed,
		EvidenceRevision: runtimeTestHash('d'), Diagnosis: "bounded correction completed successfully",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "correction cleanup completed",
		ProcessEvidence: "correction process scan found no descendants",
	}); err != nil {
		t.Fatal(err)
	}
	status, err = Resolve(ResolveOptions{CWD: repo, ChangeName: "disabled-failed-evidence", ReviewDisabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if status.NextRecommended != "verify" || status.Dependencies.Verify != DependencyReady || status.Dependencies.Archive != DependencyBlocked {
		t.Fatalf("successful correction did not require fresh verification: %#v", status)
	}
}

func TestResolvePureEngramRoutesDisabledSubstantiveFailToUnmanagedAuthorization(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".engram"), 0o755); err != nil {
		t.Fatal(err)
	}
	runRuntimeLedgerGit(t, repo, "remote", "add", "origin", "git@github.com:Gentleman-Programming/gentle-ai.git")
	store := mustRuntimeStore(t, repo, "engram-unmanaged")
	started, err := store.Begin(context.Background(), BeginAttemptRequest{ExpectedRevision: "", RequestID: "engram-unmanaged-begin", WorkUnit: "verify", EvidenceGoal: "prove failed evidence", MaxAttempts: 1, MaxChangedLines: 20})
	if err != nil {
		t.Fatal(err)
	}
	failedEvidence := runtimeTestHash('e')
	failed, err := store.Finish(context.Background(), FinishAttemptRequest{ExpectedRevision: started.Revision, RequestID: "engram-unmanaged-finish", Outcome: AttemptFailed, EvidenceRevision: failedEvidence, Diagnosis: "admitted substantive failure", HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants"})
	if err != nil {
		t.Fatal(err)
	}
	report := strings.Replace(boundedVerifyEnvelope(failedEvidence, "fail"), "blockers: 0", "blockers: 1", 1)
	restore := stubEngramExport(t, []engramObservation{
		{Title: "sdd/engram-unmanaged/proposal", Content: "proposal", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/engram-unmanaged/spec", Content: "### Requirement: Runtime\n#### Scenario: Failed evidence\n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/engram-unmanaged/design", Content: "design", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/engram-unmanaged/tasks", Content: "- [x] 1.1 Work\n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/engram-unmanaged/verify-report", Content: report, Project: "gentle-ai", Scope: "project"},
	})
	defer restore()
	status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "engram-unmanaged", ReviewDisabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if status.ArtifactStore != ArtifactStoreEngram || status.NextRecommended != RuntimeActionAuthorizeRemediation || status.ReviewGate == nil || status.ReviewGate.Delivery != "disabled/unmanaged" || status.ReviewGate.Result == "allow" || status.RuntimeStatus == nil || status.RuntimeStatus.Revision != failed.Revision || status.RuntimeStatus.Binding != nil || status.ReviewTransaction != nil {
		t.Fatalf("pure Engram unmanaged routing = %#v", status)
	}
}

func TestUnmanagedAuthorizationFailsClosedForExplicitReviewArtifacts(t *testing.T) {
	for _, artifact := range []struct {
		name, path, content string
	}{
		{"invalid transaction", "transaction.json", "{"},
		{"foreign receipt", "receipt.json", "foreign review authority"},
		{"ambiguous receipt", "receipt.json", "{}"},
	} {
		t.Run(artifact.name, func(t *testing.T) {
			repo := initRuntimeLedgerRepo(t)
			root := seedReadyChange(t, repo, "artifact-denial", "- [x] 1.1 Work\n")
			store := mustRuntimeStore(t, repo, "artifact-denial")
			started, err := store.Begin(context.Background(), BeginAttemptRequest{ExpectedRevision: "", RequestID: "artifact-begin", WorkUnit: "verify", EvidenceGoal: "prove failed evidence", MaxAttempts: 1, MaxChangedLines: 20})
			if err != nil {
				t.Fatal(err)
			}
			failedEvidence := runtimeTestHash('8')
			write(t, filepath.Join(root, "verify-report.md"), strings.Replace(boundedVerifyEnvelope(failedEvidence, "fail"), "blockers: 0", "blockers: 1", 1))
			if _, err := store.Finish(context.Background(), FinishAttemptRequest{ExpectedRevision: started.Revision, RequestID: "artifact-finish", Outcome: AttemptFailed, EvidenceRevision: failedEvidence, Diagnosis: "admitted failure", HarnessDisposition: HarnessReused, CleanupEvidence: "cleanup completed", ProcessEvidence: "no descendants"}); err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(root, "reviews", artifact.path), artifact.content)
			status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "artifact-denial", ReviewDisabled: true})
			if err != nil {
				t.Fatal(err)
			}
			if status.NextRecommended == RuntimeActionAuthorizeRemediation || len(status.BlockedReasons) == 0 || (status.ReviewGate != nil && status.ReviewGate.Delivery == "disabled/unmanaged") {
				t.Fatalf("explicit %s did not fail closed: %#v", artifact.name, status)
			}
		})
	}
}
