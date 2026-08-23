package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// issue3564Journeys proves an attempt token remains governed only by its SDD
// evidence chain when receipt-driven development changes while the token is
// live. The journey starts from the runner's untouched (off) mode, acquires a
// reset-authorized failed-evidence correction, turns review on, and settles the
// exact token without creating or consulting review authority.
func issue3564Journeys() []Journey {
	return []Journey{
		{
			ID:     "j112-sdd-attempt-settle-survives-review-mode-transition",
			Review: reviewUntouched,
			Title:  "A failed-evidence correction token settles after review turns on",
			Source: "#3564 Slice 1: SDD attempt authority is independent of receipt-driven development mode",
			Steps: []Step{
				{Name: "fixture: repository with a committed OpenSpec change", Fixture: sddRuntimeRepo},
				{Name: "review mode starts off", Requires: modeCapability,
					Args: productArgs("review", "mode", "status", "--json"), After: issue3564ModeIs("off")},
				{Name: "fail and reset the bounded verification objective", Requires: sddAttemptRemediationCapability,
					Composite: issue3564FailAndReset},
				{Name: "acquire the exact failed-evidence correction while review is off", Requires: sddAttemptRemediationCapability,
					Composite: issue3564AcquireCorrection},
				{Name: "turn review on after the correction token is issued", Requires: modeCapability,
					Args: productArgs("review", "mode", "enable", "--scope", "global", "--json"), After: issue3564ModeIs("on")},
				{Name: "fixture: correction changes the candidate", Fixture: sddBoundedCorrection},
				{Name: "settle the exact token using only failed SDD evidence", Requires: sddAttemptRemediationCapability,
					Composite: issue3564SettleCorrection},
			},
		},
	}
}

func issue3564ModeIs(want string) func(*Sandbox, Observation) error {
	return func(_ *Sandbox, observation Observation) error {
		var envelope struct {
			Status struct {
				Effective string `json:"effective"`
			} `json:"status"`
		}
		if observation.ExitCode != 0 || json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &envelope) != nil {
			return fmt.Errorf("review mode status did not return JSON at exit %d: %s", observation.ExitCode, firstLine(observation.Stderr, observation.Stdout))
		}
		if envelope.Status.Effective != want {
			return fmt.Errorf("review mode = %q, want %q", envelope.Status.Effective, want)
		}
		return nil
	}
}

func issue3564FailAndReset(r *journeyRun) error {
	acquire := r.run(append([]string{
		"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange,
		"--request-id", "issue3564-failed-acquire",
	}, sddChainVerifyObjective...), false)
	var acquired sddCompactAttemptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(acquire.Stdout)), &acquired); err != nil ||
		acquire.ExitCode != 0 || acquired.State != "proceed" || acquired.Token == "" {
		return fmt.Errorf("failed verification acquire = %#v exit=%d err=%v", acquired, acquire.ExitCode, err)
	}

	failed := r.run(append([]string{
		"sdd-attempt", "settle", "--cwd", r.sandbox.Repo, "--change", sddChange,
		"--token", acquired.Token, "--request-id", "issue3564-failed-settle",
		"--outcome", "failed", "--evidence-revision", sddFailedEvidence,
	}, sddTerminalEvidence...), false)
	var settled sddCompactAttemptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(failed.Stdout)), &settled); err != nil ||
		failed.ExitCode != 0 || settled.State != "blocked" || settled.Reason != "maintainer_decision" {
		return fmt.Errorf("failed verification settle = %#v exit=%d err=%v", settled, failed.ExitCode, err)
	}

	status, err := readRuntimeStatus(r)
	if err != nil || status.NextAction != "reset" {
		return fmt.Errorf("failed verification runtime status = %#v err=%v", status, err)
	}
	reset := r.run(sddAttemptArgs(r, "reset", status.Revision, "issue3564-reset",
		"--reason", "maintainer authorizes exact failed-evidence remediation", "--actor", "bench"), false)
	if reset.ExitCode != 0 {
		return fmt.Errorf("failed-evidence reset exited %d: %s", reset.ExitCode, firstLine(reset.Stderr, reset.Stdout))
	}
	return nil
}

func issue3564AcquireCorrection(r *journeyRun) error {
	acquire := r.run(append([]string{
		"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange,
		"--request-id", "issue3564-correction-acquire", "--remediates-evidence-revision", sddFailedEvidence,
	}, sddChainVerifyObjective...), false)
	var result sddCompactAttemptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(acquire.Stdout)), &result); err != nil ||
		acquire.ExitCode != 0 || result.State != "proceed" || result.Token == "" {
		return fmt.Errorf("failed-evidence correction acquire = %#v exit=%d err=%v", result, acquire.ExitCode, err)
	}
	r.sandbox.Scratch["issue3564-correction-token"] = result.Token
	return nil
}

func issue3564SettleCorrection(r *journeyRun) error {
	token := r.sandbox.Scratch["issue3564-correction-token"]
	if token == "" {
		return fmt.Errorf("mode-transition journey has no correction token")
	}
	settle := r.run(append([]string{
		"sdd-attempt", "settle", "--cwd", r.sandbox.Repo, "--change", sddChange,
		"--token", token, "--request-id", "issue3564-correction-settle",
		"--outcome", "passed", "--evidence-revision", sddCorrectedEvidence,
		"--remediates-evidence-revision", sddFailedEvidence,
	}, sddTerminalEvidence...), false)
	var result sddCompactAttemptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(settle.Stdout)), &result); err != nil ||
		settle.ExitCode != 0 || result.State != "complete" {
		return fmt.Errorf("mode-transition correction settle = %#v exit=%d err=%v", result, settle.ExitCode, err)
	}
	status, err := proveRuntime(r.sandbox)
	if err != nil || status.ActiveAttempt != nil || len(status.Attempts) != 2 ||
		status.Attempts[1].RemediatesEvidenceRevision != sddFailedEvidence {
		return fmt.Errorf("mode-transition settled runtime = %#v err=%v", status, err)
	}
	return nil
}
