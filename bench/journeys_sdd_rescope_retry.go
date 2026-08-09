package main

import "fmt"

var sddRescopeRetryObjective = []string{
	"--work-unit", "bench independent verification",
	"--evidence-goal", "prove the unchanged candidate",
	"--max-attempts", "3", "--max-changed-lines", "20",
}

var sddRescopeRetrySuccessor = []string{
	"--work-unit", "bench narrower reverification",
	"--evidence-goal", "rerun the focused independent verification",
	"--max-attempts", "2", "--max-changed-lines", "10",
}

var sddAttemptRescopeCapability = &Capability{
	Verb:  []string{"sdd-attempt", "rescope"},
	Probe: []string{"sdd-attempt", "rescope", "--work-unit=probe", "--evidence-goal=probe", "--max-attempts=1", "--max-changed-lines=1", "--reason=probe", "--actor=probe"},
}

// sddRescopeEvidenceOnlyRetry reconstructs #2621 through public runtime calls.
// Every status proof starts a new process, so the final assertion also proves
// the successful unchanged settle survives full-chain replay.
func sddRescopeEvidenceOnlyRetry(r *journeyRun) error {
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "begin", status.Revision, "bench-rescope-retry-begin", sddRescopeRetryObjective...), false)
	if status, err = readRuntimeStatus(r); err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "finish", status.Revision, "bench-rescope-retry-fail",
		append([]string{"--outcome", "failed", "--evidence-revision", sddFailedEvidence}, sddTerminalEvidence...)...), false)
	if status, err = readRuntimeStatus(r); err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "rescope", status.Revision, "bench-rescope-retry-audit",
		append(append([]string{}, sddRescopeRetrySuccessor...),
			"--reason", "maintainer narrowed a transient failure to one evidence-only retry",
			"--actor", "bench maintainer")...), false)
	if status, err = readRuntimeStatus(r); err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "begin", status.Revision, "bench-rescope-retry-reverify", sddRescopeRetrySuccessor...), false)
	if status, err = readRuntimeStatus(r); err != nil {
		return err
	}
	observation := r.run(sddAttemptArgs(r, "finish", status.Revision, "bench-rescope-retry-pass",
		append([]string{
			"--outcome", "passed", "--evidence-revision", sddCorrectedEvidence,
			"--remediates-evidence-revision", sddFailedEvidence,
		}, sddTerminalEvidence...)...), false)
	if observation.ExitCode != 0 {
		return fmt.Errorf("rescope-authorized unchanged settle exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
	}
	completed, err := proveRuntime(r.sandbox)
	if err != nil {
		return err
	}
	if !completed.Complete || completed.Binding != nil || len(completed.Attempts) != 2 ||
		completed.Attempts[0].FinishCandidateTree == "" ||
		completed.Attempts[1].BeginCandidateTree == "" ||
		completed.Attempts[1].FinishCandidateTree == "" ||
		completed.Attempts[0].FinishCandidateTree != completed.Attempts[1].BeginCandidateTree ||
		completed.Attempts[1].RemediatesEvidenceRevision != sddFailedEvidence ||
		completed.Attempts[1].FinishCandidateTree != completed.Attempts[1].BeginCandidateTree {
		return fmt.Errorf("rescope-authorized unchanged settle did not survive replay: %#v", completed)
	}
	return nil
}

func sddRescopeRetryJourneys() []Journey {
	return []Journey{{
		ID:     "j79-sdd-rescope-authorizes-evidence-only-retry",
		Title:  "An audited narrower rescope authorizes unchanged successful evidence settlement",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/2621",
		Steps: []Step{
			{Name: "fixture: completed change with admitted failed verification", Fixture: sddPlanningArtifacts(sddFailedVerifyReport)},
			{Name: "mode disable", Requires: modeCapability, Args: productArgs("review", "mode", "disable", "--json")},
			{Name: "failed evidence, audited rescope, and unchanged passed settlement", Requires: sddAttemptRescopeCapability, Composite: sddRescopeEvidenceOnlyRetry},
		},
	}}
}
