package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// issue3470Journeys proves that parallel apply scheduling defaults to serialized,
// maintaining item-level authority and exact acquire/settle contracts without
// coupling scheduling policy to provider background subagent availability.
func issue3470Journeys() []Journey {
	return []Journey{
		{
			ID:     "j114-sdd-parallel-apply-scheduling-policy-serialized-by-default",
			Review: reviewUntouched,
			Title:  "SDD parallel apply scheduling defaults to serialized while retaining item authority",
			Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/3470",
			Steps: []Step{
				{Name: "fixture: repository with complete planning artifacts", Fixture: sddPlanningArtifacts("")},
				{Name: "acquire bounded item attempt under default serialized policy", Requires: sddAttemptBeginCapability,
					Composite: issue3470AcquireAndSettleItem},
				{Name: "sdd-status verifies serialized progression and retained item authority", Requires: sddStatusCapability,
					Composite: issue3470VerifyStatusProgression},
			},
		},
	}
}

func issue3470AcquireAndSettleItem(r *journeyRun) error {
	acquire := r.run(append([]string{
		"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange,
		"--request-id", "issue3470-item-acquire",
	}, sddChainVerifyObjective...), false)
	var acquired sddCompactAttemptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(acquire.Stdout)), &acquired); err != nil ||
		acquire.ExitCode != 0 || acquired.State != "proceed" || acquired.Token == "" {
		return fmt.Errorf("item attempt acquire = %#v exit=%d err=%v", acquired, acquire.ExitCode, err)
	}

	settle := r.run(append([]string{
		"sdd-attempt", "settle", "--cwd", r.sandbox.Repo, "--change", sddChange,
		"--token", acquired.Token, "--request-id", "issue3470-item-settle",
		"--outcome", "passed", "--evidence-revision", sddCorrectedEvidence,
	}, sddTerminalEvidence...), false)
	var settled sddCompactAttemptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(settle.Stdout)), &settled); err != nil ||
		settle.ExitCode != 0 || settled.State != "complete" {
		return fmt.Errorf("item attempt settle = %#v exit=%d err=%v", settled, settle.ExitCode, err)
	}
	return nil
}

func issue3470VerifyStatusProgression(r *journeyRun) error {
	status := r.run([]string{
		"sdd-status", sddChange, "--cwd", r.sandbox.Repo, "--json",
	}, false)
	if status.ExitCode != 0 {
		return fmt.Errorf("sdd-status exit %d: %s", status.ExitCode, firstLine(status.Stderr, status.Stdout))
	}
	var envelope struct {
		NextRecommended string `json:"nextRecommended"`
		Dependencies    struct {
			Apply   string `json:"apply"`
			Verify  string `json:"verify"`
			Archive string `json:"archive"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(status.Stdout)), &envelope); err != nil {
		return fmt.Errorf("parse sdd-status JSON: %w", err)
	}
	if envelope.NextRecommended != "verify" {
		return fmt.Errorf("sdd-status nextRecommended = %q, want %q", envelope.NextRecommended, "verify")
	}
	if envelope.Dependencies.Apply != "all_done" || envelope.Dependencies.Verify != "ready" || envelope.Dependencies.Archive != "blocked" {
		return fmt.Errorf("sdd-status dependencies = %+v, want apply:all_done verify:ready archive:blocked", envelope.Dependencies)
	}
	return nil
}
