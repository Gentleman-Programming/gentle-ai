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
		ID:     "j108-sdd-post-review-verify-report-is-natively-bound",
		Review: reviewOptedIn,
		Title:  "Post-review final verify report is admitted only through native settlement attestation",
		Source: "native SDD archive-status report-attestation contract",
		Steps: append(sddApprovedAuthoritySteps(sddSharedScaffoldingAuthorityFixture),
			Step{Name: "bind the approved authority to the active change", Requires: bindSDDCapability, Composite: sddBindApprovedReview},
			Step{Name: "the bound receipt initially allows archive", Requires: sddStatusCapability,
				Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusAssertion("pre-report-replacement archive readiness", func(status sddStatusV1) error {
					if status.ReviewGate == nil || status.ReviewGate.Result != "allow" || status.Dependencies.Archive != "ready" || status.NextRecommended != "archive" {
						return fmt.Errorf("pre-replacement status = %+v", status)
					}
					return nil
				})},
			Step{Name: "final verification replaces and stages only the canonical report", Fixture: sddReplacePostReviewVerifyReport},
			Step{Name: "the unbound report replacement remains archive-blocked", Requires: sddStatusCapability,
				Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusAssertion("unattested report delta", func(status sddStatusV1) error {
					if status.Dependencies.Archive != "blocked" || status.NextRecommended != "resolve-review" {
						return fmt.Errorf("unattested report status = %+v", status)
					}
					return nil
				})},
			Step{Name: "settle the final verify work unit and bind exact report bytes", Requires: sddAttemptSettleCapability, Composite: sddSettleAttestedFinalVerifyReport},
			Step{Name: "archive status accepts only the attested report-only delta", Requires: sddStatusCapability,
				Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusAssertion("attested report archive readiness", func(status sddStatusV1) error {
					if status.ReviewGate == nil || status.ReviewGate.Result != "allow" || status.Dependencies.Archive != "ready" || status.NextRecommended != "archive" {
						return fmt.Errorf("attested report status = %+v", status)
					}
					return nil
				})},
		),
	}, {
		ID:     "j109-sdd-legacy-post-review-report-requires-current-attestation",
		Review: reviewOptedIn,
		Title:  "Legacy post-review report settlement with an arbitrary work-unit label requires one current native attestation",
		Source: "native SDD report-attestation upgrade compatibility contract",
		Steps: append(sddApprovedAuthoritySteps(sddSharedScaffoldingAuthorityFixture),
			Step{Name: "bind the approved authority to the active change", Requires: bindSDDCapability, Composite: sddBindApprovedReview},
			Step{Name: "final verification replaces and stages only the canonical report", Fixture: sddReplacePostReviewVerifyReport},
			Step{Name: "fixture: create a digestless settlement with its historical free-form work-unit label", Requires: sddAttemptSettleCapability, Composite: sddSettleLegacyFinalVerifyReport},
			Step{Name: "legacy status requires verification attestation instead of a new review", Requires: sddStatusCapability,
				Args: productArgs("sdd-status", sddChange, "--json", "--instructions"), After: sddStatusAssertion("legacy attestation routing", func(status sddStatusV1) error {
					if status.ReviewGate != nil || status.Dependencies.Verify != "ready" || status.Dependencies.Archive != "blocked" || status.NextRecommended != "verify" {
						return fmt.Errorf("legacy attestation status = %+v", status)
					}
					instructions := strings.Join(status.PhaseInstructions.Verify, "\n")
					if !strings.Contains(instructions, "verification attestation required") || !strings.Contains(instructions, "--work-unit \"verify-attestation\"") {
						return fmt.Errorf("legacy attestation instructions = %q", instructions)
					}
					return nil
				})},
			Step{Name: "settle one distinct current verify-attestation work unit", Requires: sddAttemptSettleCapability, Composite: func(r *journeyRun) error {
				return sddSettleAttestedFinalVerifyReportWorkUnit(r, "verify-attestation", "bench-legacy-attestation-acquire", "bench-legacy-attestation-settle")
			}},
			Step{Name: "archive status accepts the current native attestation", Requires: sddStatusCapability,
				Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusAssertion("current attestation archive readiness", func(status sddStatusV1) error {
					if status.ReviewGate == nil || status.ReviewGate.Result != "allow" || status.Dependencies.Archive != "ready" || status.NextRecommended != "archive" {
						return fmt.Errorf("current-attested legacy status = %+v", status)
					}
					return nil
				})},
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
