package main

import (
	"errors"
	"fmt"
)

var sddAttemptRescopeCapability = &Capability{
	Verb: []string{"sdd-attempt", "rescope"},
	Probe: []string{
		"sdd-attempt", "rescope", "--expected-revision=probe", "--request-id=probe",
		"--work-unit=probe", "--evidence-goal=probe", "--max-attempts=1", "--max-changed-lines=1",
		"--reason=probe", "--actor=bench",
	},
}

var sddRescopeObjectiveA = []string{
	"--work-unit", "bench objective A",
	"--evidence-goal", "prove the original bounded objective",
	"--max-attempts", "3", "--max-changed-lines", "90",
}

var sddRescopeObjectiveB = []string{
	"--work-unit", "bench objective B",
	"--evidence-goal", "prove the narrower successor objective",
	"--max-attempts", "3", "--max-changed-lines", "60",
}

var sddRescopeObjectiveC = []string{
	"--work-unit", "bench objective C",
	"--evidence-goal", "prove the still narrower successor objective",
	"--max-attempts", "3", "--max-changed-lines", "30",
}

// sddConsecutiveRescopeRefusal drives #2830 exactly as an operator does. A
// rescope successor has no terminal attempt of its own, so the next rescope
// must fail before publication and leave the readable authority unchanged.
func sddConsecutiveRescopeRefusal(r *journeyRun) error {
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	if begin := r.run(sddAttemptArgs(r, "begin", status.Revision, "bench-2830-begin-a", sddRescopeObjectiveA...), false); begin.ExitCode != 0 {
		return fmt.Errorf("begin objective A exit=%d: %s", begin.ExitCode, firstLine(begin.Stderr))
	}

	status, err = readRuntimeStatus(r)
	if err != nil {
		return err
	}
	if finish := r.run(sddAttemptArgs(r, "finish", status.Revision, "bench-2830-finish-a",
		append([]string{"--outcome", "failed", "--evidence-revision", sddFailedEvidence}, sddTerminalEvidence...)...), false); finish.ExitCode != 0 {
		return fmt.Errorf("finish objective A exit=%d: %s", finish.ExitCode, firstLine(finish.Stderr))
	}

	status, err = readRuntimeStatus(r)
	if err != nil {
		return err
	}
	if len(status.Attempts) != 1 || status.NextAction != "begin" || status.ActiveAttempt != nil {
		return fmt.Errorf("failed zero-drift objective A status = %#v", status)
	}
	if rescope := r.run(sddAttemptArgs(r, "rescope", status.Revision, "bench-2830-rescope-a-to-b",
		append(append([]string{}, sddRescopeObjectiveB...), "--reason", "maintainer narrowed A into B", "--actor", "bench")...), false); rescope.ExitCode != 0 {
		return fmt.Errorf("rescope objective A to B exit=%d: %s", rescope.ExitCode, firstLine(rescope.Stderr))
	}

	beforeRefusal, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	if len(beforeRefusal.Attempts) != 1 || beforeRefusal.NextAction != "begin" || beforeRefusal.ActiveAttempt != nil {
		return fmt.Errorf("rescoped objective B status = %#v", beforeRefusal)
	}
	refusal := r.run(sddAttemptArgs(r, "rescope", beforeRefusal.Revision, "bench-2830-rescope-b-to-c",
		append(append([]string{}, sddRescopeObjectiveC...), "--reason", "attempted B to C before B has an attempt", "--actor", "bench")...), false)
	if refusal.ExitCode == 0 {
		return errors.New("consecutive rescope before B has an attempt was accepted")
	}

	afterRefusal, err := readRuntimeStatus(r)
	if err != nil {
		return fmt.Errorf("status after refused consecutive rescope: %w", err)
	}
	if afterRefusal.Revision != beforeRefusal.Revision || len(afterRefusal.Attempts) != 1 ||
		afterRefusal.NextAction != "begin" || afterRefusal.ActiveAttempt != nil {
		return fmt.Errorf("refused consecutive rescope changed authority: before=%#v after=%#v", beforeRefusal, afterRefusal)
	}
	return nil
}

func rescopeWriteGuardJourneys() []Journey {
	return []Journey{
		{
			ID:     "j79-consecutive-rescope-refuses-before-publication",
			Review: reviewOptedIn,
			Title:  "Failed zero-drift objective: a second rescope refuses before it can corrupt status",
			Source: "issue #2830: rescope successors require their own terminal attempt",
			Steps: []Step{
				{Name: "fixture: runtime repository with no candidate drift", Fixture: sddRuntimeRepo},
				{Name: "begin, fail zero-drift, rescope once, refuse a second rescope, and read status", Requires: sddAttemptRescopeCapability, Composite: sddConsecutiveRescopeRefusal},
			},
		},
	}
}
