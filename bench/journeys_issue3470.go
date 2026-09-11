package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// issue3470Journeys proves that parallel apply scheduling defaults to serialized.
func issue3470Journeys() []Journey {
	return []Journey{{
		ID:     "j3470-sdd-parallel-apply-scheduling-policy-serialized-by-default",
		Review: reviewUntouched,
		Title:  "SDD parallel apply scheduling defaults to serialized while retaining item authority",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/3470",
		Steps: []Step{
			{Name: "fixture: repository with complete planning artifacts", Fixture: sddPlanningArtifacts("")},
			{Name: "acquire bounded item attempt under default serialized policy", Requires: sddAttemptBeginCapability, Composite: issue3470AcquireAndSettleItem},
			{Name: "sdd-status verifies serialized progression and retained item authority", Requires: sddStatusCapability, Composite: issue3470VerifyStatusProgression},
		},
	}}
}

func issue3470AcquireAndSettleItem(r *journeyRun) error {
	acquire := r.run(append([]string{"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange, "--request-id", "issue3470-item-acquire"}, sddChainVerifyObjective...), false)
	var acq, set sddCompactAttemptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(acquire.Stdout)), &acq); err != nil || acquire.ExitCode != 0 || acq.State != "proceed" || acq.Token == "" {
		return fmt.Errorf("acquire = %#v exit=%d err=%v", acq, acquire.ExitCode, err)
	}
	settle := r.run(append([]string{"sdd-attempt", "settle", "--cwd", r.sandbox.Repo, "--change", sddChange, "--token", acq.Token, "--request-id", "issue3470-item-settle", "--outcome", "passed", "--evidence-revision", sddCorrectedEvidence}, sddTerminalEvidence...), false)
	if err := json.Unmarshal([]byte(strings.TrimSpace(settle.Stdout)), &set); err != nil || settle.ExitCode != 0 || set.State != "complete" {
		return fmt.Errorf("settle = %#v exit=%d err=%v", set, settle.ExitCode, err)
	}
	return nil
}

func issue3470VerifyStatusProgression(r *journeyRun) error {
	status := r.run([]string{"sdd-status", sddChange, "--cwd", r.sandbox.Repo, "--json"}, false)
	if status.ExitCode != 0 {
		return fmt.Errorf("sdd-status exit %d: %s", status.ExitCode, firstLine(status.Stderr, status.Stdout))
	}
	var env struct {
		NextRecommended string                                  `json:"nextRecommended"`
		Dependencies    struct{ Apply, Verify, Archive string } `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(status.Stdout)), &env); err != nil {
		return fmt.Errorf("parse sdd-status JSON: %w", err)
	}
	if env.NextRecommended != "verify" || env.Dependencies.Apply != "all_done" || env.Dependencies.Verify != "ready" || env.Dependencies.Archive != "blocked" {
		return fmt.Errorf("sdd-status progression = %+v, want next:verify apply:all_done verify:ready archive:blocked", env)
	}
	return nil
}
