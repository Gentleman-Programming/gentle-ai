package main

// Wave 5 (Gate Cutover) Slice 5, task 6.7: black-box bench evidence that a
// pre-PR delivery composition used to rescue now denies -- with a runnable
// next step named in the denial itself, per rdd-defect-workflow. This
// journey supersedes j51-unrelated-noop-authority-keeps-composed-delivery
// (deleted, not rewritten: see journeys_wave1.go's own retirement comment
// at that journey's former location -- its regression, a cycle-detection
// bug inside EvaluateCompactPrePRChain's own composition graph, has no
// analogous "after" behavior once that graph no longer exists).

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	prePRMultiSegmentFirst  = "wave-five-pre-pr-multi-segment-first"
	prePRMultiSegmentSecond = "wave-five-pre-pr-multi-segment-second"
	prePRMultiSegmentThird  = "wave-five-pre-pr-multi-segment-third"
)

// requirePrePRMultiSegmentDenialNamesRunnableNextStep proves the plain
// (non-negotiated) gate-result JSON's own `reason` text names a runnable
// next step -- `gentle-ai review start` -- even though the delivery used to
// compose to an allow before Wave 5 Slice 5 deleted
// EvaluateCompactPrePRChain. Denial.Code must be receipt_ambiguous
// (discovery itself found three terminal candidates, none spanning the
// whole delivered range, and named all three) -- the same shape
// TestUnqualifiedPrePRDiscoveryDeniesSequentialCompactReceiptsWithoutComposition
// (internal/cli) pins at the Go integration layer, reproduced here
// end-to-end through the built binary.
func requirePrePRMultiSegmentDenialNamesRunnableNextStep(_ *Sandbox, observation Observation) error {
	if observation.ExitCode == 0 {
		return fmt.Errorf("multi-segment pre-PR delivery unexpectedly allowed without composition:\n%s", observation.Stdout)
	}
	var gate waveGateResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &gate); err != nil {
		return fmt.Errorf("parse multi-segment pre-PR denial: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	if gate.Allowed || gate.Result != "invalidated" {
		return fmt.Errorf("multi-segment pre-PR delivery = %+v, want a denied (invalidated) result", gate)
	}
	if gate.Context.Denial == nil || gate.Context.Denial.Code != "receipt_ambiguous" {
		return fmt.Errorf("multi-segment pre-PR denial code = %+v, want receipt_ambiguous", gate.Context.Denial)
	}
	if !strings.Contains(gate.Reason, "gentle-ai review start") {
		return fmt.Errorf("multi-segment pre-PR denial reason = %q, want it to name the runnable next step (gentle-ai review start)", gate.Reason)
	}
	return nil
}

// waveFiveJourneys also carries the revision-canonicalization registrar from
// journeys_revision_canon.go: the central aggregator in journeys.go was owned
// by an open pull request when j62 landed, and this is the nearest registrar
// that was not.
func waveFiveJourneys() []Journey {
	journeys := []Journey{
		{
			ID:     "j61-pre-pr-multi-segment-delivery-denies-without-composition",
			Review: reviewOptedIn,
			Title:  "Pre-PR multi-segment delivery composition used to rescue now denies, naming a runnable next step",
			Source: "Wave 5 Slice 5 (design decision 1: DELETED edges: EvaluateCompactPrePRChain)",
			Steps: []Step{
				{Name: "fixture: repo with remote", Fixture: baseRepoWithRemote},
				{Name: "fixture: stage first segment", Fixture: stageDocs("wave-five-segment-one")},
				{Name: "review first segment", Requires: startNamedCapability, Args: productArgs("review", "start", "--lineage", prePRMultiSegmentFirst)},
				{Name: "approve first segment", Requires: finalizeCapability, Args: productArgs("review", "finalize", "--lineage", prePRMultiSegmentFirst)},
				{Name: "fixture: commit first segment", Fixture: commitStaged("docs: wave five segment one")},
				{Name: "fixture: stage second segment", Fixture: stageDocs("wave-five-segment-two")},
				{Name: "review second segment", Requires: startNamedCapability, Args: productArgs("review", "start", "--lineage", prePRMultiSegmentSecond)},
				{Name: "approve second segment", Requires: finalizeCapability, Args: productArgs("review", "finalize", "--lineage", prePRMultiSegmentSecond)},
				{Name: "fixture: commit second segment", Fixture: commitStaged("docs: wave five segment two")},
				{Name: "fixture: stage third segment", Fixture: stageDocs("wave-five-segment-three")},
				{Name: "review third segment", Requires: startNamedCapability, Args: productArgs("review", "start", "--lineage", prePRMultiSegmentThird)},
				{Name: "approve third segment", Requires: finalizeCapability, Args: productArgs("review", "finalize", "--lineage", prePRMultiSegmentThird)},
				{Name: "fixture: commit third segment", Fixture: commitStaged("docs: wave five segment three")},
				// No --lineage: lineage-free discovery is exactly the shape
				// EvaluateCompactPrePRChain used to compose. It no longer exists
				// (TestPrePRComposition_ZeroCallers, internal/cli, proves it by
				// call-absence), so the same three terminal, individually
				// approved segments now deny.
				{Name: "three delivered segments deny without composition", Requires: validateCapability,
					Args:  productArgs("review", "validate", "--gate", "pre-pr", "--base-ref", "origin/main"),
					After: requirePrePRMultiSegmentDenialNamesRunnableNextStep},
			},
		},
	}
	return append(journeys, revisionCanonJourneys()...)
}
