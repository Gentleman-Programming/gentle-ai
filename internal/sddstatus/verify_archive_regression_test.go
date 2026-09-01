package sddstatus

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

const (
	regressionFailedEvidence = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	regressionPassedEvidence = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// These cases preserve the #3123/#3174 regression contract: a persisted passing
// report and runtime settlement permit archive even when the work-unit label is
// accepted as arbitrary by public instructions; this test selects sdd-verify and
// proves no magic label is required, with no magic attestation attached.
func TestArbitrarySddVerifyWorkUnitPermitsArchiveAfterPassingReport(t *testing.T) {
	const change = "arbitrary-work-unit-direct"
	repo := initRuntimeLedgerRepo(t)
	changeRoot := seedReadyChange(t, repo, change, "- [x] 1.1 Work\n")
	store := mustRuntimeStore(t, repo, change)

	attempt := acquireRegressionAttempt(t, store, "direct-acquire", "sdd-verify", "record final verification", "", 2)
	persistRegressionPassingReport(t, repo, changeRoot, regressionPassedEvidence)
	settleRegressionAttempt(t, store, "direct-settle", attempt.Token, AttemptPassed, regressionPassedEvidence, "")

	assertArbitraryWorkUnitArchiveRoute(t, repo, change, store)
}

func TestArbitrarySddVerifyWorkUnitPermitsArchiveAfterResetRemediationAndFreshReport(t *testing.T) {
	const change = "arbitrary-work-unit-remediation"
	repo := initRuntimeLedgerRepo(t)
	changeRoot := seedReadyChange(t, repo, change, "- [x] 1.1 Work\n")
	store := mustRuntimeStore(t, repo, change)

	failed := acquireRegressionAttempt(t, store, "failed-a-acquire", "sdd-verify", "record failed verification A", "", 1)
	settleRegressionAttempt(t, store, "failed-a-settle", failed.Token, AttemptFailed, regressionFailedEvidence, "")

	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Reset(context.Background(), ResetObjectiveRequest{
		ExpectedRevision: status.Revision,
		RequestID:        "failed-a-authorized-reset",
		Reason:           "maintainer authorizes remediation B after failed verification A",
		Actor:            "maintainer",
	}); err != nil {
		t.Fatalf("authorized reset: %v", err)
	}

	remediation := acquireRegressionAttempt(t, store, "remediation-b-acquire", "sdd-remediate", "remediate failed verification A", regressionFailedEvidence, 2)
	appendRuntimeLedgerFile(t, repo, "remediation B changed the candidate\n")
	settleRegressionAttempt(t, store, "remediation-b-settle", remediation.Token, AttemptPassed, regressionPassedEvidence, regressionFailedEvidence)

	final := acquireRegressionAttempt(t, store, "fresh-c-acquire", "sdd-verify", "record fresh final verification C", "", 2)
	persistRegressionPassingReport(t, repo, changeRoot, regressionPassedEvidence)
	settleRegressionAttempt(t, store, "fresh-c-settle", final.Token, AttemptPassed, regressionPassedEvidence, "")

	assertArbitraryWorkUnitArchiveRoute(t, repo, change, store)
}

func acquireRegressionAttempt(t *testing.T, store RuntimeStore, requestID, workUnit, goal, remediates string, maxAttempts int) CompactAttemptResult {
	t.Helper()
	result, err := store.Acquire(context.Background(), CompactAcquireRequest{
		BeginAttemptRequest: BeginAttemptRequest{
			RequestID: requestID, WorkUnit: workUnit, EvidenceGoal: goal, MaxAttempts: maxAttempts, MaxChangedLines: 100,
		},
		RemediatesEvidenceRevision: remediates,
	})
	if err != nil {
		t.Fatalf("acquire %s: %v", requestID, err)
	}
	if result.State != CompactStateProceed || result.Token == "" {
		t.Fatalf("acquire %s = %#v, want proceed with token", requestID, result)
	}
	return result
}

func settleRegressionAttempt(t *testing.T, store RuntimeStore, requestID, token string, outcome AttemptOutcome, evidence, remediates string) {
	t.Helper()
	if _, err := store.Settle(context.Background(), CompactSettleRequest{
		RequestID: requestID, Token: token, Outcome: outcome, EvidenceRevision: evidence,
		Diagnosis:          "#3123/#3174 regression " + requestID,
		HarnessDisposition: HarnessReused, CleanupEvidence: "fixture has no external resources",
		ProcessEvidence: "fixture process scan found no descendants", RemediatesEvidenceRevision: remediates,
	}); err != nil {
		t.Fatalf("settle %s: %v", requestID, err)
	}
}

func persistRegressionPassingReport(t *testing.T, repo, changeRoot, evidence string) {
	t.Helper()
	report := strings.ReplaceAll(testVerifyEnvelope("pass", 0, 0, "1/1", "1/1", 0, 0), regressionFailedEvidence, evidence)
	if report == "" || evidence == regressionFailedEvidence {
		t.Fatal("test fixture must replace the default evidence revision")
	}
	path := filepath.Join(changeRoot, "verify-report.md")
	write(t, path, report)
	rel, err := filepath.Rel(repo, path)
	if err != nil {
		t.Fatal(err)
	}
	runRuntimeLedgerGit(t, repo, "add", rel)
	runRuntimeLedgerGit(t, repo, "commit", "-qm", "test: persist passing verification report")
}

func assertArbitraryWorkUnitArchiveRoute(t *testing.T, repo, change string, store RuntimeStore) {
	t.Helper()
	runtime, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.Attempts) == 0 {
		t.Fatal("fixture recorded no attempts")
	}
	final := runtime.Attempts[len(runtime.Attempts)-1]
	if final.WorkUnit != "sdd-verify" || final.Outcome != AttemptPassed || final.EvidenceRevision != regressionPassedEvidence {
		t.Fatalf("final attempt = work unit:%q outcome:%q evidence:%q, want sdd-verify, passed, %q", final.WorkUnit, final.Outcome, final.EvidenceRevision, regressionPassedEvidence)
	}
	if final.AttestedVerifyReportDigest != "" {
		t.Fatalf("arbitrary work-unit unexpectedly carried an attestation: %q", final.AttestedVerifyReportDigest)
	}

	resolved, err := Resolve(ResolveOptions{CWD: repo, ChangeName: change, ReviewDisabled: true, IncludeInstructions: true})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Dependencies.Verify != DependencyAllDone || resolved.Dependencies.Archive != DependencyReady || resolved.NextRecommended != "archive" {
		t.Fatalf("archive routing = verify:%q archive:%q next:%q blockers:%v", resolved.Dependencies.Verify, resolved.Dependencies.Archive, resolved.NextRecommended, resolved.BlockedReasons)
	}
	if len(resolved.BlockedReasons) != 0 {
		t.Fatalf("archive-ready reporter sequence carried blockers: %v", resolved.BlockedReasons)
	}
}
