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

// An arbitrary label can record ordinary runtime work, but cannot attest the
// canonical final verification report that archive requires.
func TestArbitrarySddVerifyWorkUnitReroutesArchiveAfterPassingReport(t *testing.T) {
	const change = "arbitrary-work-unit-direct"
	repo := initRuntimeLedgerRepo(t)
	changeRoot := seedReadyChange(t, repo, change, "- [x] 1.1 Work\n")
	store := mustRuntimeStore(t, repo, change)

	attempt := acquireRegressionAttempt(t, store, "direct-acquire", "sdd-verify", "record final verification", "", 2)
	persistRegressionPassingReport(t, repo, changeRoot, regressionPassedEvidence)
	settleRegressionAttempt(t, store, "direct-settle", attempt.Token, AttemptPassed, regressionPassedEvidence, "")

	assertArbitraryWorkUnitReroutesArchive(t, repo, change, store)
}

func TestArbitrarySddVerifyWorkUnitReroutesArchiveAfterResetRemediationAndFreshReport(t *testing.T) {
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

	assertArbitraryWorkUnitReroutesArchive(t, repo, change, store)
}

func TestCanonicalVerifyWorkUnitsPermitArchiveAfterPassingReport(t *testing.T) {
	for _, workUnit := range []string{finalVerifyWorkUnit, finalVerifyAttestationWorkUnit} {
		t.Run(workUnit, func(t *testing.T) {
			change := "canonical-" + workUnit
			repo := initRuntimeLedgerRepo(t)
			changeRoot := seedReadyChange(t, repo, change, "- [x] 1.1 Work\n")
			store := mustRuntimeStore(t, repo, change)

			attempt := acquireRegressionAttempt(t, store, "canonical-acquire", workUnit, "record final verification", "", 2)
			persistRegressionPassingReport(t, repo, changeRoot, regressionPassedEvidence)
			settleRegressionAttempt(t, store, "canonical-settle", attempt.Token, AttemptPassed, regressionPassedEvidence, "")

			assertCanonicalWorkUnitArchiveRoute(t, repo, change, store, workUnit)
		})
	}
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

func assertArbitraryWorkUnitReroutesArchive(t *testing.T, repo, change string, store RuntimeStore) {
	t.Helper()
	final := regressionFinalAttempt(t, store)
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
	if resolved.Dependencies.Verify != DependencyReady || resolved.Dependencies.Archive != DependencyBlocked || resolved.NextRecommended != "verify" {
		t.Fatalf("arbitrary verification routing = verify:%q archive:%q next:%q blockers:%v", resolved.Dependencies.Verify, resolved.Dependencies.Archive, resolved.NextRecommended, resolved.BlockedReasons)
	}
}

func assertCanonicalWorkUnitArchiveRoute(t *testing.T, repo, change string, store RuntimeStore, workUnit string) {
	t.Helper()
	final := regressionFinalAttempt(t, store)
	if final.WorkUnit != workUnit || final.Outcome != AttemptPassed || final.EvidenceRevision != regressionPassedEvidence || final.AttestedVerifyReportDigest == "" {
		t.Fatalf("final attempt = %#v, want canonical %q passing attestation for %q", final, workUnit, regressionPassedEvidence)
	}

	resolved, err := Resolve(ResolveOptions{CWD: repo, ChangeName: change, ReviewDisabled: true, IncludeInstructions: true})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Dependencies.Verify != DependencyAllDone || resolved.Dependencies.Archive != DependencyReady || resolved.NextRecommended != "archive" {
		t.Fatalf("canonical verification routing = verify:%q archive:%q next:%q blockers:%v", resolved.Dependencies.Verify, resolved.Dependencies.Archive, resolved.NextRecommended, resolved.BlockedReasons)
	}
}

func regressionFinalAttempt(t *testing.T, store RuntimeStore) RuntimeAttempt {
	t.Helper()
	runtime, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.Attempts) == 0 {
		t.Fatal("fixture recorded no attempts")
	}
	return runtime.Attempts[len(runtime.Attempts)-1]
}
