package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// sddVerifyApplicabilityCapability probes the shape rather than the state: a
// build that has the command answers with a decision, a build that does not
// rejects the flag.
var sddVerifyApplicabilityCapability = &Capability{
	Verb: []string{"sdd-verify-applicability"},
	// Every flag the journeys below actually pass. A probe that omits one
	// cannot tell a build that has it from a build that does not.
	Flags: []string{"--cwd", "--projection", "--base-ref", "--operational-path"},
}

// issue3708Journeys prove the final SDD verification is owed for what the
// candidate actually contains rather than for every change alike.
//
// Both journeys declare reviewUntouched, and that is the point rather than a
// convenience: the assessment is SDD-owned. Review risk and specification
// conformance are different questions, so nothing here may depend on whether
// receipt-driven development is on. A journey that opted review in would hide
// exactly the coupling #3708 exists to keep out.
func issue3708Journeys() []Journey {
	return []Journey{
		{
			ID:     "j119-passive-candidate-does-not-owe-final-verification",
			Review: reviewUntouched,
			Title:  "A provably passive candidate reports that verification is not required",
			Source: "#3708: assess final verification applicability before routing",
			Steps: []Step{
				{Name: "fixture: repository with a committed OpenSpec change", Fixture: sddRuntimeRepo},
				{
					Name:     "the staged prose candidate owes no execution",
					Requires: sddVerifyApplicabilityCapability,
					Args:     productArgs("sdd-verify-applicability", "--projection", "staged"),
					After:    issue3708DecisionIs("not_required", "passive_candidate"),
				},
			},
		},
		{
			ID:     "j120-active-candidate-still-owes-final-verification",
			Review: reviewUntouched,
			Title:  "An active candidate keeps the final verifier mandatory",
			Source: "#3708: active, mixed, and unclassifiable candidates fail closed",
			Steps: []Step{
				{Name: "fixture: repository with a committed OpenSpec change", Fixture: sddRuntimeRepo},
				{Name: "fixture: the candidate gains executable source", Fixture: issue3708StageActiveSource},
				{
					Name:     "a candidate carrying source still owes verification",
					Requires: sddVerifyApplicabilityCapability,
					Args:     productArgs("sdd-verify-applicability", "--projection", "staged"),
					After:    issue3708DecisionIs("required", "active_content"),
				},
				{
					Name: "the operator is told what evidence would satisfy it",
					// A required decision that does not name its evidence goal
					// is a dead end wearing a decision's clothes.
					Requires: sddVerifyApplicabilityCapability,
					Args:     productArgs("sdd-verify-applicability", "--projection", "staged"),
					After:    issue3708NamesAnEvidenceGoal,
				},
			},
		},
	}
}

// issue3708StageActiveSource adds one executable file to the same candidate the
// passive journey measured, so the two journeys differ only by content.
func issue3708StageActiveSource(sandbox *Sandbox) error {
	path := filepath.Join(sandbox.Repo, "internal", "run.go")
	if err := sandbox.write(path, "package internal\n\nfunc Run() {}\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "internal/run.go"); err != nil {
		return err
	}
	staged, err := gitOut(sandbox, sandbox.Repo, "diff", "--cached", "--name-only")
	if err != nil {
		return err
	}
	if !strings.Contains(staged, "internal/run.go") {
		return fmt.Errorf("fixture claims staged executable source but the staged diff is %q", staged)
	}
	return nil
}

type issue3708Assessment struct {
	Schema       string `json:"schema"`
	Decision     string `json:"decision"`
	Reason       string `json:"reason"`
	EvidenceGoal string `json:"evidence_goal"`
}

func issue3708Decode(observation Observation) (issue3708Assessment, error) {
	var assessment issue3708Assessment
	if observation.ExitCode != 0 {
		return assessment, fmt.Errorf("sdd-verify-applicability exited %d: %s",
			observation.ExitCode, firstLine(observation.Stderr, observation.Stdout))
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &assessment); err != nil {
		return assessment, fmt.Errorf("assessment is not JSON: %s", firstLine(observation.Stdout, observation.Stderr))
	}
	return assessment, nil
}

func issue3708DecisionIs(decision, reason string) func(*Sandbox, Observation) error {
	return func(_ *Sandbox, observation Observation) error {
		assessment, err := issue3708Decode(observation)
		if err != nil {
			return err
		}
		if assessment.Decision != decision || assessment.Reason != reason {
			return fmt.Errorf("decision = %q (%q), want %q (%q)",
				assessment.Decision, assessment.Reason, decision, reason)
		}
		return nil
	}
}

func issue3708NamesAnEvidenceGoal(_ *Sandbox, observation Observation) error {
	assessment, err := issue3708Decode(observation)
	if err != nil {
		return err
	}
	if strings.TrimSpace(assessment.EvidenceGoal) == "" {
		return fmt.Errorf("a %q decision named no evidence goal", assessment.Decision)
	}
	return nil
}
