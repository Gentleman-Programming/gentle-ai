package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// issue4210Journeys recreates the historical report contradiction from #4210:
// a legacy failed report exists before unfinished tasks, so apply must remain
// actionable while final verification remains unavailable.
func issue4210Journeys() []Journey {
	return []Journey{{
		ID:     "j128-historical-verification-does-not-block-apply",
		Review: reviewUntouched,
		Title:  "Historical verification evidence does not block unfinished apply work",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/4210",
		Steps: []Step{
			{Name: "fixture: pending task with a historical failed verification report", Fixture: issue4210Fixture},
			{Name: "sdd-status keeps apply actionable", Requires: sddStatusCapability,
				Args: productArgs("sdd-status", sddChange, "--json"), After: issue4210ApplyAssertion("sdd-status")},
			{Name: "sdd-continue keeps the same apply contract", Requires: sddContinueCapability,
				Args: productArgs("sdd-continue", sddChange, "--json"), After: issue4210ApplyAssertion("sdd-continue")},
		},
	}}
}

func issue4210Fixture(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	root := sddChangeRoot(sandbox)
	for name, content := range map[string]string{
		"proposal.md":           "# Proposal\n",
		"design.md":             "# Design\n",
		"tasks.md":              "- [ ] 1.1 Finish implementation\n",
		"specs/routing/spec.md": "### Requirement: apply remains actionable\n#### Scenario: historical verification predates unfinished work\n",
		"verify-report.md":      "## Historical verification\n\nVerdict: FAIL\n\n- coverage was incomplete\n",
	} {
		if err := sandbox.write(filepath.Join(root, name), content); err != nil {
			return err
		}
	}
	if err := sandbox.git(sandbox.Repo, "add", "openspec"); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "commit", "-qm", "issue 4210 historical verification fixture")
}

func issue4210ApplyAssertion(command string) func(*Sandbox, Observation) error {
	return func(sandbox *Sandbox, observation Observation) error {
		var status sddStatusV2
		if err := json.Unmarshal([]byte(observation.Stdout), &status); err != nil {
			return fmt.Errorf("parse %s JSON: %w", command, err)
		}
		if status.Dependencies.Apply != "ready" || status.Dependencies.Verify != "blocked" ||
			status.Dependencies.Archive != "blocked" || status.NextRecommended != "apply" ||
			status.TaskProgress.AllComplete || len(status.BlockedReasons) != 0 {
			return fmt.Errorf("%s status = apply %q verify %q archive %q next %q complete %t blockers %v, want ready/blocked/blocked/apply/false/[]",
				command, status.Dependencies.Apply, status.Dependencies.Verify, status.Dependencies.Archive,
				status.NextRecommended, status.TaskProgress.AllComplete, status.BlockedReasons)
		}
		fingerprint := fmt.Sprintf("%s|%s|%s|%s|%t|%v", status.Dependencies.Apply, status.Dependencies.Verify,
			status.Dependencies.Archive, status.NextRecommended, status.TaskProgress.AllComplete, status.BlockedReasons)
		if previous := sandbox.Scratch["issue-4210-status"]; previous != "" && previous != fingerprint {
			return fmt.Errorf("%s status differs from the earlier public command: %q != %q", command, fingerprint, previous)
		}
		sandbox.Scratch["issue-4210-status"] = fingerprint
		return nil
	}
}
