package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// The recovery guard-rail journey reads the review gate without charging
// instrumentation to the operator's measured invocations.
type recoveryGateResult struct {
	Result  string `json:"result"`
	Allowed bool   `json:"allowed"`
}

func recoveryPostApplyAllows(sandbox *Sandbox) bool {
	var result recoveryGateResult
	observation := sandbox.readBack("review", "validate", "--cwd", sandbox.Repo, "--gate", "post-apply")
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &result); err != nil {
		return false
	}
	return result.Allowed && result.Result == "allow"
}

var recoveryInvalidateCapability = &Capability{
	Verb:  []string{"review", "invalidate"},
	Flags: []string{"--cwd", "--lineage", "--expected-revision", "--gate"},
}

// The finalize step already recorded the selectors, keeping this declaration
// attached to exactly one counted invocation.
func recoveryInvalidateHealthyApproved(sandbox *Sandbox) ([]string, error) {
	if strings.TrimSpace(sandbox.Lineage) == "" || strings.TrimSpace(sandbox.Revision) == "" {
		return nil, fmt.Errorf("no lineage recorded to invalidate: lineage %q revision %q", sandbox.Lineage, sandbox.Revision)
	}
	return []string{
		"review", "invalidate",
		"--lineage", sandbox.Lineage,
		"--expected-revision", sandbox.Revision,
		"--gate", "post-apply",
		"--cwd", sandbox.Repo,
	}, nil
}

func recoveryProveApprovalSurvived(r *journeyRun) error {
	if !recoveryPostApplyAllows(r.sandbox) {
		return errors.New("a refused invalidation left the approved authority no longer allowing")
	}
	return nil
}

// Walk the two recover refusals in the same order as the operator, then prove
// neither refusal modified the healthy authority.
func recoveryWalkIntoGuardRails(r *journeyRun) error {
	observation := r.run([]string{"review", "status", "--cwd", r.sandbox.Repo}, false)
	var head authorityHead
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &head); err != nil {
		return fmt.Errorf("parse review status: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	if len(head.Entries) != 1 || head.Entries[0].State != "approved" {
		return fmt.Errorf("expected exactly one approved lineage, got %+v", head.Entries)
	}
	lineage := head.Entries[0].LineageID
	revision := head.Entries[0].Revision
	if !recoveryPostApplyAllows(r.sandbox) {
		return errors.New("fixture claims healthy approved authority but its post-apply gate does not allow")
	}

	r.run(productArgsFor(r, "review", "recover",
		"--predecessor-lineage", lineage,
		"--expected-predecessor-revision", revision,
		"--successor-lineage", "review-unwanted-successor",
		"--disposition", "scope_changed"), false)
	r.run(productArgsFor(r, "review", "recover",
		"--predecessor-lineage", lineage,
		"--expected-predecessor-revision", revision,
		"--successor-lineage", "review-unwanted-successor",
		"--disposition", "invalidated"), false)

	// Invalidation has its own step so its by-design classification applies to
	// its one invocation, rather than to this composite.
	if !recoveryPostApplyAllows(r.sandbox) {
		return errors.New("two refused operations left the approved authority no longer allowing")
	}
	return nil
}

func reviewRecoveryJourneys() []Journey {
	return []Journey{{
		ID:     "j43-recovery-guard-rails-as-an-operator-meets-them",
		Review: reviewOptedIn,
		Title:  "Three correct refusals around healthy approved authority, and the one exit that works",
		Source: "shape 4 (a correct refusal that names nothing runnable) + community deadlock report",
		// All three refusals are correct. A healthy approved authority cannot
		// acquire a same-scope successor or be invalidated. Changing the candidate
		// makes recovery possible; the final step ends at an actual allow.
		Steps: []Step{
			{Name: "fixture: repo", Fixture: baseRepo},
			{Name: "fixture: stage docs", Fixture: stageProse("", "healthy")},
			{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
			{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberLineage},
			{Name: "walk into the recovery guard rails", Requires: recoverCapability, Composite: recoveryWalkIntoGuardRails},
			{Name: "invalidate the healthy approved authority", Requires: recoveryInvalidateCapability,
				Args: recoveryInvalidateHealthyApproved},
			{Name: "the refused invalidation changed nothing", Composite: recoveryProveApprovalSurvived},
			{Name: "fixture: change the candidate, which is what all three asked for", Fixture: stageProse("", "changed")},
			{Name: "recover, following exactly what the gate then names",
				Requires: recoverCapability, Composite: recoverScopeChangeRoundTrip("review-guardrail-successor")},
		},
	}}
}
