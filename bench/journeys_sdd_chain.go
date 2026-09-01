package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// journeys_sdd_chain.go pins #1974 slice 2 (#2565): the unmanaged-remediation
// binding derives from the immutable attempt chain, so an audited reset
// between the failed settle and the correction acquire no longer orphans the
// correction. It is the j63 variant from the fix design, in its own file so
// the collision-guarded corpus grows without touching journeys_sdd.go. The
// anti-laundering budget (one passed correction per failed evidence) is
// pinned at the unit level in runtime_ledger_chain_binding_test.go.

// sddChainVerifyObjective exhausts its budget on the single failed
// verification attempt, which is exactly the decision-required state the
// audited reset exists to escape (#1974 field report).
var sddChainVerifyObjective = []string{
	"--work-unit", "bench chain verification",
	"--evidence-goal", "admit the verification failure",
	"--max-attempts", "1", "--max-changed-lines", "20",
}

// sddChainFailedVerificationExhaustsBudget settles the admitted verification
// failure and proves the runtime demands a maintainer decision.
func sddChainFailedVerificationExhaustsBudget(r *journeyRun) error {
	acquire := r.run(append([]string{
		"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange,
		"--request-id", "bench-chain-acquire-verification",
	}, sddChainVerifyObjective...), false)
	var result sddCompactAttemptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(acquire.Stdout)), &result); err != nil {
		return fmt.Errorf("parse failed verification acquire: %w (stderr: %s)", err, firstLine(acquire.Stderr))
	}
	if acquire.ExitCode != 0 || result.State != "proceed" || result.Token == "" {
		return fmt.Errorf("failed verification acquire = %#v exit=%d", result, acquire.ExitCode)
	}
	r.run(append([]string{
		"sdd-attempt", "settle", "--cwd", r.sandbox.Repo, "--change", sddChange,
		"--token", result.Token, "--request-id", "bench-chain-settle-verification",
		"--outcome", "failed", "--evidence-revision", sddFailedEvidence,
	}, sddTerminalEvidence...), false)
	proved, err := proveRuntime(r.sandbox)
	if err != nil {
		return err
	}
	if proved.ActiveAttempt != nil || len(proved.Attempts) != 1 || proved.Attempts[0].Outcome != "failed" || proved.NextAction != "reset" {
		return fmt.Errorf("exhausted failed verification did not demand a maintainer decision: %#v", proved)
	}
	return nil
}

// sddChainAuditedReset records the maintainer decision between the failed
// settle and the correction acquire. The reset wipes the live evidence
// pointer; only the immutable attempt chain remembers the failed evidence.
func sddChainAuditedReset(r *journeyRun) error {
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "reset", status.Revision, "bench-chain-reset",
		"--reason", "maintainer decision: remediate the admitted failure under a fresh objective",
		"--actor", "bench"), false)
	proved, err := proveRuntime(r.sandbox)
	if err != nil {
		return err
	}
	if proved.NextAction != "begin" || proved.EvidenceRevision != "" {
		return fmt.Errorf("audited reset did not clear the live evidence pointer: %#v", proved)
	}
	return nil
}

// sddChainInterruptedAttempt records a later non-remediation interruption.
// Its evidence pointer is cleared, while the earlier failed verification
// remains the immutable correction binding after the audited reset.
func sddChainInterruptedAttempt(r *journeyRun) error {
	acquire := r.run(append([]string{
		"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange,
		"--request-id", "bench-chain-acquire-interruption",
	}, sddUnmanagedObjective...), false)
	var result sddCompactAttemptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(acquire.Stdout)), &result); err != nil {
		return fmt.Errorf("parse interruption acquire: %w (stderr: %s)", err, firstLine(acquire.Stderr))
	}
	if acquire.ExitCode != 0 || result.State != "proceed" || result.Token == "" {
		return fmt.Errorf("interruption acquire = %#v exit=%d", result, acquire.ExitCode)
	}
	r.run(append([]string{
		"sdd-attempt", "settle", "--cwd", r.sandbox.Repo, "--change", sddChange,
		"--token", result.Token, "--request-id", "bench-chain-settle-interruption",
		"--outcome", "interrupted",
	}, sddTerminalEvidence...), false)
	status, err := proveRuntime(r.sandbox)
	if err != nil {
		return err
	}
	if status.ActiveAttempt != nil || len(status.Attempts) != 2 || status.Attempts[1].Outcome != "interrupted" ||
		status.Attempts[1].EvidenceRevision != "" || status.EvidenceRevision != "" || status.NextAction != "begin" {
		return fmt.Errorf("interrupted successor did not omit evidence: %#v", status)
	}
	return nil
}

// sddChainCorrectionCompletes keeps the RED diagnostic load-bearing: before
// #2871, compact settle compares the caller's failed evidence to the later
// interruption's live pointer, blocks invalid_continuation, and leaves token 3
// running. The chain-derived correction must instead settle atomically.
func sddChainCorrectionCompletes(r *journeyRun) error {
	token := r.sandbox.Scratch["unmanaged-token"]
	observation := r.run(append([]string{
		"sdd-attempt", "settle", "--cwd", r.sandbox.Repo, "--change", sddChange, "--token", token,
		"--request-id", "bench-chain-correct", "--outcome", "passed", "--evidence-revision", sddCorrectedEvidence,
		"--remediates-evidence-revision", sddFailedEvidence,
	}, sddTerminalEvidence...), false)
	var result sddCompactAttemptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &result); err != nil {
		return fmt.Errorf("parse chain correction settle: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	if result.State == "blocked" && result.Reason == "invalid_continuation" {
		status, err := proveRuntime(r.sandbox)
		if err != nil {
			return err
		}
		if status.ActiveAttempt == nil || len(status.Attempts) != 3 || status.ActiveAttempt.Ordinal != 3 ||
			status.EvidenceRevision != "" {
			return fmt.Errorf("invalid continuation did not leave the exact remediation attempt running: %#v", status)
		}
		return fmt.Errorf("bounded correction was blocked as invalid_continuation and left the exact live remediation attempt running")
	}
	if observation.ExitCode != 0 || result.State != "complete" {
		return fmt.Errorf("chain correction settle = %#v exit=%d", result, observation.ExitCode)
	}
	status, err := proveRuntime(r.sandbox)
	if err != nil {
		return err
	}
	if len(status.Attempts) != 3 || status.ActiveAttempt != nil || status.Attempts[2].RemediatesEvidenceRevision != sddFailedEvidence {
		return fmt.Errorf("chain correction did not settle against the immutable failed evidence: %#v", status)
	}
	return nil
}

// sddChainJourneys is the corpus slice this file registers.
func sddChainJourneys() []Journey {
	return []Journey{
		{
			ID:     "j64-unmanaged-remediation-survives-audited-reset",
			Review: reviewOptedIn,
			Title:  "An audited reset between the failed settle and the correction acquire no longer orphans the correction",
			Source: "#1974 slice 2 (#2565): chain-derived failed-evidence binding",
			Steps: []Step{
				{Name: "fixture: completed change with admitted failed verification", Fixture: sddPlanningArtifacts(sddFailedVerifyReport)},
				{Name: "mode disable", Requires: modeCapability, Args: productArgs("review", "mode", "disable", "--json")},
				{Name: "failed verification exhausts its objective budget", Requires: sddAttemptBeginCapability, Composite: sddChainFailedVerificationExhaustsBudget},
				{Name: "audited reset records the maintainer decision", Requires: sddAttemptResetCapability, Composite: sddChainAuditedReset},
				{Name: "acquire the one bounded correction after the reset", Requires: sddAttemptRemediationCapability, Composite: sddUnmanagedAcquireCorrection},
				{Name: "fixture: correction changes the candidate", Fixture: sddBoundedCorrection},
				{Name: "settle the evidence-bound correction across the reset", Requires: sddAttemptRemediationCapability, Composite: sddUnmanagedCorrectionCompletes},
				{Name: "replay cannot acquire another correction", Requires: sddAttemptRemediationCapability, Composite: sddUnmanagedReplayIsComplete},
				{Name: "fixture: fresh independent verification passes", Fixture: sddReplaceFailedVerifyReport},
				{Name: "archive is ready without review authority", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"),
					After: sddStatusAssertion("disabled archive after the reset-crossing correction", func(status sddStatusV2) error {
						if status.Dependencies.Archive != "ready" || status.NextRecommended != "archive" || status.ReviewOffer != nil {
							return fmt.Errorf("disabled archive = archive %q next %q offer=%+v", status.Dependencies.Archive, status.NextRecommended, status.ReviewOffer)
						}
						return nil
					})},
			},
		},
		{
			ID:     "j87-unmanaged-remediation-uses-chain-failed-evidence",
			Review: reviewOptedIn,
			Title:  "A reset-authorized correction settles against its failed evidence, not a later interruption",
			Source: "#2871: compact settle must match the immutable failed-evidence chain",
			Steps: []Step{
				{Name: "fixture: completed change with admitted failed verification", Fixture: sddPlanningArtifacts(sddFailedVerifyReport)},
				{Name: "mode disable", Requires: modeCapability, Args: productArgs("review", "mode", "disable", "--json")},
				{Name: "failed verification exhausts its objective budget", Requires: sddAttemptBeginCapability, Composite: sddChainFailedVerificationExhaustsBudget},
				{Name: "audited reset records the maintainer decision", Requires: sddAttemptResetCapability, Composite: sddChainAuditedReset},
				{Name: "later interruption records distinct live evidence", Requires: sddAttemptRemediationCapability, Composite: sddChainInterruptedAttempt},
				{Name: "acquire the correction bound to the original failed evidence", Requires: sddAttemptRemediationCapability, Composite: sddUnmanagedAcquireCorrection},
				{Name: "fixture: correction changes the candidate", Fixture: sddBoundedCorrection},
				{Name: "settle the exact live remediation token against the failed evidence chain", Requires: sddAttemptRemediationCapability, Composite: sddChainCorrectionCompletes},
			},
		},
	}
}
