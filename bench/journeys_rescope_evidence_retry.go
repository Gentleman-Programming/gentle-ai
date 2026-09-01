package main

import "fmt"

// #2621: j79 is reserved by the independent #2830 branch, so this branch uses
// j80 to keep the black-box corpus collision-free when the two fixes converge.
var sddRescopeEvidenceObjectiveA = []string{
	"--work-unit", "verification objective A",
	"--evidence-goal", "record the failed verification evidence",
	"--max-attempts", "2", "--max-changed-lines", "20",
}

var sddRescopeEvidenceObjectiveB = []string{
	"--work-unit", "runtime proof objective B",
	"--evidence-goal", "rerun evidence against objective A's unchanged candidate",
	"--max-attempts", "2", "--max-changed-lines", "10",
}

func sddRescopeEvidenceFailureA(r *journeyRun) error {
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "begin", status.Revision, "bench-rescope-evidence-begin-a", sddRescopeEvidenceObjectiveA...), false)
	if status, err = readRuntimeStatus(r); err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "finish", status.Revision, "bench-rescope-evidence-finish-a",
		append([]string{"--outcome", "failed", "--evidence-revision", sddFailedEvidence}, sddTerminalEvidence...)...), false)
	failed, err := proveRuntime(r.sandbox)
	if err != nil {
		return err
	}
	if failed.ActiveAttempt != nil || len(failed.Attempts) != 1 || failed.Attempts[0].Outcome != "failed" ||
		failed.NextAction != "begin" {
		return fmt.Errorf("failed objective A did not retain an ordinary rescope continuation: %#v", failed)
	}
	return nil
}

func sddRescopeEvidenceAuditAtoB(r *journeyRun) error {
	failed, err := proveRuntime(r.sandbox)
	if err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "rescope", failed.Revision, "bench-rescope-evidence-a-to-b",
		append(sddRescopeEvidenceObjectiveB,
			"--reason", "maintainer authorized a narrower runtime proof for the unchanged candidate",
			"--actor", "bench")...), false)
	rescoped, err := proveRuntime(r.sandbox)
	if err != nil {
		return err
	}
	if rescoped.NextAction != "begin" || rescoped.LastRescope == nil ||
		rescoped.LastRescope.Actor != "bench" || rescoped.LastRescope.Reason == "" ||
		rescoped.LastRescope.PreviousObjectiveID != failed.Attempts[0].ObjectiveID ||
		rescoped.LastRescope.PreviousGeneration != failed.Attempts[0].ObjectiveGeneration ||
		rescoped.LastRescope.RescopeCandidateTree != failed.Attempts[0].FinishCandidateTree {
		return fmt.Errorf("audited A-to-B rescope did not preserve authority facts: %#v", rescoped)
	}
	return nil
}

func sddRescopeEvidenceOnlyPassB(r *journeyRun) error {
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "begin", status.Revision, "bench-rescope-evidence-begin-b", sddRescopeEvidenceObjectiveB...), false)
	if status, err = readRuntimeStatus(r); err != nil {
		return err
	}
	finished := r.run(sddAttemptArgs(r, "finish", status.Revision, "bench-rescope-evidence-finish-b",
		append([]string{
			"--outcome", "passed", "--evidence-revision", sddCorrectedEvidence,
			"--remediates-evidence-revision", sddFailedEvidence,
		}, sddTerminalEvidence...)...), false)
	if finished.ExitCode != 0 {
		return fmt.Errorf("rescope-authorized evidence-only pass was refused: %s", firstLine(finished.Stderr))
	}
	completed, err := proveRuntime(r.sandbox)
	if err != nil {
		return err
	}
	if !completed.Complete || completed.NextAction != "complete" || completed.ActiveAttempt != nil || completed.Binding != nil ||
		len(completed.Attempts) != 2 {
		return fmt.Errorf("rescope-authorized evidence-only pass did not leave readable complete status: %#v", completed)
	}
	failed, retried := completed.Attempts[0], completed.Attempts[1]
	if retried.RemediatesEvidenceRevision != sddFailedEvidence || retried.BeginCandidateTree != failed.FinishCandidateTree ||
		retried.FinishCandidateTree != failed.FinishCandidateTree {
		return fmt.Errorf("rescope-authorized pass did not preserve the failed evidence link and unchanged candidate: %#v", completed)
	}
	return nil
}

func rescopeEvidenceOnlyRetryJourneys() []Journey {
	return []Journey{{
		ID:     "j80-rescope-authorized-evidence-only-retry",
		Review: reviewOptedIn,
		Title:  "Audited rescope authorizes one unchanged evidence-only retry",
		Source: "#2621: failed A -> audited rescope B -> unchanged PASS linked to A",
		Steps: []Step{
			{Name: "fixture: runtime repository with one staged candidate", Fixture: sddRuntimeRepo},
			{Name: "mode disable", Requires: modeCapability, Args: productArgs("review", "mode", "disable", "--json")},
			{Name: "objective A fails with unused capacity", Requires: sddAttemptBeginCapability, Composite: sddRescopeEvidenceFailureA},
			{Name: "audited rescope carries A authority into B", Requires: sddAttemptRescopeCapability, Composite: sddRescopeEvidenceAuditAtoB},
			{Name: "unchanged evidence-only PASS for B links A and leaves complete status", Requires: sddAttemptRemediationCapability, Composite: sddRescopeEvidenceOnlyPassB},
		},
	}}
}
