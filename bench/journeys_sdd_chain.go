package main

import "fmt"

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
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "begin", status.Revision, "bench-chain-begin-verification", sddChainVerifyObjective...), false)
	if status, err = readRuntimeStatus(r); err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "finish", status.Revision, "bench-chain-finish-verification",
		append([]string{"--outcome", "failed", "--evidence-revision", sddFailedEvidence}, sddTerminalEvidence...)...), false)
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

// sddChainJourneys is the corpus slice this file registers.
func sddChainJourneys() []Journey {
	return []Journey{
		{
			ID:     "j64-unmanaged-remediation-survives-audited-reset",
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
					After: sddStatusAssertion("disabled archive after the reset-crossing correction", func(status sddStatusV1) error {
						if status.Dependencies.Archive != "ready" || status.NextRecommended != "archive" || status.ReviewGate != nil || status.ReviewOffer != nil {
							return fmt.Errorf("disabled archive = archive %q next %q gate=%+v offer=%+v", status.Dependencies.Archive, status.NextRecommended, status.ReviewGate, status.ReviewOffer)
						}
						return nil
					})},
			},
		},
	}
}
