package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

const legacyFinalVerificationWorkUnit = "post-correction-final-verification"

// sddPostReviewVerifyReportJourneys proves the narrow archive-status exception:
// final verification changes its canonical report after a terminal review, and
// native settlement binds that exact replacement without relaxing review gates.
func sddPostReviewVerifyReportJourneys() []Journey {
	return []Journey{{
		Review: reviewOptedIn,
		ID:     "j108-sdd-post-review-verify-report-is-natively-bound",
		Title:  "#3417: native final-verify settlement preserves its report attestation while archive remains unmanaged ordinary policy",
		Source: "native SDD report-attestation contract under #3417: terminal review burns, so settlement metadata never fabricates durable review authority or a delivery gate",
		Steps: append(sddBurnedAuthoritySteps(sddSharedScaffoldingAuthorityFixture),
			Step{Name: "terminal burn leaves initial archive readiness under unmanaged ordinary policy", Requires: sddStatusCapability,
				Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusAssertion("pre-report-replacement ordinary archive readiness", requireSDDUnmanagedOrdinaryArchive("pre-replacement"))},
			Step{Name: "final verification replaces and stages only the canonical report", Fixture: sddReplacePostReviewVerifyReport},
			Step{Name: "the unbound report replacement remains archive-ready under unmanaged ordinary policy", Requires: sddStatusCapability,
				Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusAssertion("unattested report ordinary archive", requireSDDUnmanagedOrdinaryArchive("unattested report"))},
			Step{Name: "settle the final verify work unit and bind exact report bytes", Requires: sddAttemptSettleCapability, Composite: sddSettleAttestedFinalVerifyReport},
			Step{Name: "native report settlement leaves archive under unmanaged ordinary policy", Requires: sddStatusCapability,
				Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusAssertion("attested report ordinary archive", requireSDDUnmanagedOrdinaryArchive("attested report"))},
		),
	}, {
		ID:     "j109-sdd-legacy-post-review-report-requires-current-attestation",
		Review: reviewOptedIn,
		Title:  "#3417: legacy final-verify settlement preserves its compatibility record without creating a durable review gate",
		Source: "native SDD report-attestation compatibility under #3417: terminal review burn leaves legacy and current settlement metadata under unmanaged ordinary policy",
		Steps: append(sddBurnedAuthoritySteps(sddSharedScaffoldingAuthorityFixture),
			Step{Name: "final verification replaces and stages only the canonical report", Fixture: sddReplacePostReviewVerifyReport},
			Step{Name: "fixture: create a digestless settlement with its historical free-form work-unit label", Requires: sddAttemptSettleCapability, Composite: sddSettleLegacyFinalVerifyReport},
			Step{Name: "legacy settlement leaves archive under unmanaged ordinary policy instead of fabricating review authority", Requires: sddStatusCapability,
				Args: productArgs("sdd-status", sddChange, "--json", "--instructions"), After: sddStatusAssertion("legacy ordinary archive routing", requireSDDUnmanagedOrdinaryArchive("legacy settlement"))},
			Step{Name: "settle one distinct current verify-attestation work unit as ordinary-policy metadata", Requires: sddAttemptSettleCapability, Composite: func(r *journeyRun) error {
				return sddSettleAttestedFinalVerifyReportWorkUnit(r, "verify-attestation", "bench-legacy-attestation-acquire", "bench-legacy-attestation-settle")
			}},
			Step{Name: "current settlement still leaves archive under unmanaged ordinary policy", Requires: sddStatusCapability,
				Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusAssertion("current attestation ordinary archive", requireSDDUnmanagedOrdinaryArchive("current legacy attestation"))},
		),
	}}
}

func sddReplacePostReviewVerifyReport(sandbox *Sandbox) error {
	path := filepath.Join(sddChangeRoot(sandbox), "verify-report.md")
	before, err := gitOut(sandbox, sandbox.Repo, "rev-parse", ":openspec/changes/"+sddChange+"/verify-report.md")
	if err != nil {
		return err
	}
	finalReport := strings.Replace(sddVerifyReport,
		"sha256:1111111111111111111111111111111111111111111111111111111111111111", sddCorrectedEvidence, 1)
	if err := sandbox.write(path, finalReport); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "openspec"); err != nil {
		return err
	}
	after, err := gitOut(sandbox, sandbox.Repo, "rev-parse", ":openspec/changes/"+sddChange+"/verify-report.md")
	if err != nil {
		return err
	}
	if before == after {
		return fmt.Errorf("post-review verify report staged blob did not change: %s", after)
	}
	return nil
}

func sddSettleAttestedFinalVerifyReport(r *journeyRun) error {
	return sddSettleFinalVerifyReportWorkUnit(r, "verify", "bench-post-review-verify-acquire", "bench-post-review-verify-settle", true)
}

func sddSettleLegacyFinalVerifyReport(r *journeyRun) error {
	return sddSettleFinalVerifyReportWorkUnit(r, legacyFinalVerificationWorkUnit, "bench-legacy-final-verify-acquire", "bench-legacy-final-verify-settle", false)
}

func sddSettleAttestedFinalVerifyReportWorkUnit(r *journeyRun, workUnit, acquireID, settleID string) error {
	return sddSettleFinalVerifyReportWorkUnit(r, workUnit, acquireID, settleID, true)
}

func sddSettleFinalVerifyReportWorkUnit(r *journeyRun, workUnit, acquireID, settleID string, requireAttestation bool) error {
	acquire := r.run([]string{
		"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange,
		"--request-id", acquireID, "--work-unit", workUnit,
		"--evidence-goal", "run final independent verification", "--max-attempts", "1", "--max-changed-lines", "20",
	}, false)
	var acquired sddCompactAttemptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(acquire.Stdout)), &acquired); err != nil {
		return fmt.Errorf("parse final verify acquire: %w (stderr: %s)", err, firstLine(acquire.Stderr))
	}
	if acquire.ExitCode != 0 || acquired.State != "proceed" || acquired.Token == "" {
		return fmt.Errorf("final verify acquire = %#v exit=%d", acquired, acquire.ExitCode)
	}
	settle := r.run(append([]string{
		"sdd-attempt", "settle", "--cwd", r.sandbox.Repo, "--change", sddChange, "--token", acquired.Token,
		"--request-id", settleID, "--outcome", "passed", "--evidence-revision", sddCorrectedEvidence,
	}, sddTerminalEvidence...), false)
	var settled sddCompactAttemptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(settle.Stdout)), &settled); err != nil {
		return fmt.Errorf("parse final verify settle: %w (stderr: %s)", err, firstLine(settle.Stderr))
	}
	if settle.ExitCode != 0 || settled.State != "complete" {
		return fmt.Errorf("final verify settle = %#v exit=%d", settled, settle.ExitCode)
	}
	runtime, err := proveRuntime(r.sandbox)
	if err != nil {
		return err
	}
	if !runtime.Complete || len(runtime.Attempts) == 0 {
		return fmt.Errorf("final verify settlement did not complete: %#v", runtime)
	}
	last := runtime.Attempts[len(runtime.Attempts)-1]
	if last.Outcome != "passed" || last.EvidenceRevision != sddCorrectedEvidence {
		return fmt.Errorf("final verify settlement did not preserve the exact report evidence: %#v", runtime)
	}
	if requireAttestation && last.AttestedVerifyReportDigest == "" {
		return fmt.Errorf("explicit final verification settlement did not persist exact report attestation: %#v", runtime)
	}
	if !requireAttestation && last.AttestedVerifyReportDigest != "" {
		return fmt.Errorf("arbitrary historical work-unit settlement unexpectedly persisted report attestation: %#v", runtime)
	}
	return nil
}
