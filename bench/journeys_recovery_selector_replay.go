package main

import "fmt"

func recoverySelectorReplayJourneys() []Journey {
	return []Journey{
		{
			ID:     "j86-recovery-authorization-preserves-selectors",
			Title:  "Recovery authorization preserves the exact staged target through printed execution",
			Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/1972",
			Steps: []Step{
				{Name: "fixture: linked worktree and remote", Fixture: linkedWorktreeWithRemote},
				{Name: "fixture: commit base-diff candidate", Fixture: commitStagedRecoveryCandidate},
				{Name: "start workspace-projected base-diff review", Requires: statusCapability, Composite: startStagedRecoveryReview},
				{Name: "capture blocker on predecessor", Requires: captureResultCapability, Composite: func(r *journeyRun) error {
					return captureCorrectableFindingFor(r, stagedPredecessorSelectors(r.sandbox)...)
				}},
				{Name: "enter correction-required", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--lineage", stagedRecoveryLineage, "--captured-results=true")},
				{Name: "forecast correction", Requires: finalizeCorrectionCapability, Args: productArgs("review", "finalize", "--lineage", stagedRecoveryLineage, "--correction-lines", "3")},
				{Name: "fixture: exact staged correction", Fixture: stageExpandedCorrection},
				{Name: "collect, replay, and execute exact selectors", Requires: recoverCapability, Composite: recoverStagedCorrection},
			},
		},
	}
}

func startStagedRecoveryReview(r *journeyRun) error {
	envelope, err := readStatusFor(r, "--lineage", stagedRecoveryLineage, "--base-ref", r.sandbox.Scratch["staged-recovery-base"])
	if err != nil {
		return err
	}
	if envelope.NextTransition.Kind != "execute" {
		return fmt.Errorf("expected an execute review.start transition for the staged recovery base-diff candidate, got %q", envelope.NextTransition.Kind)
	}
	started, err := runPrintedTransition(r, envelope)
	if err != nil {
		return err
	}
	if started.ExitCode != 0 {
		return fmt.Errorf("negotiated staged base-diff start failed: exit=%d stderr=%s", started.ExitCode, started.Stderr)
	}
	return nil
}
