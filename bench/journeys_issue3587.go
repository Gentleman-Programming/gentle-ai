package main

import (
	"encoding/json"
	"fmt"
)

const issue3587Lineage = "issue-3587-last-event-closure"

// issue3587Journeys proves the final admitted reviewer event owns reduction and
// burn. A clean 4R lifecycle has no separate FINALIZE or final-evidence round.
func issue3587Journeys() []Journey {
	return []Journey{{
		ID:     "j114-last-reviewer-capture-closes-and-burns",
		Review: reviewOptedIn,
		Title:  "The last admitted reviewer capture closes a clean 4R review with pending acknowledgement",
		Source: "issue #3797: FINALIZE is not a public phase; exact acknowledgement is the only approved terminal continuation",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "fixture: staged high-risk candidate", Fixture: stageAuthCode},
			{Name: "start exact high-risk lineage", Requires: startNamedCapability,
				Args: productArgs("review", "start", "--lineage", issue3587Lineage)},
			{Name: "capture every lens; the final capture reduces and emits acknowledgement", Requires: captureResultCapability,
				Composite: captureIssue3587LensesAndClose},
			{Name: "no review authority survives the exact acknowledgement", Requires: statusCapability,
				Composite: func(r *journeyRun) error { return requireAtomicLineageAcknowledged(r, issue3587Lineage) }},
		},
	}}
}

func captureIssue3587LensesAndClose(r *journeyRun) error {
	for round := 0; round < 8; round++ {
		envelope, err := readStatusFor(r, "--lineage", issue3587Lineage)
		if err != nil {
			return err
		}
		if envelope.NextTransition.Kind != "collect" || len(envelope.NextTransition.Collect.Inputs) == 0 ||
			envelope.NextTransition.Collect.Inputs[0].Name != "reviewer_result" {
			return fmt.Errorf("reviewer capture round %d offered %+v", round, envelope.NextTransition)
		}
		result, err := synthesizeReviewerResult(
			envelope.NextTransition.Collect.Inputs[0].ArtifactSubject.SubjectHash,
			envelope.paths(),
		)
		if err != nil {
			return err
		}
		path, err := writeScratch(r.sandbox, fmt.Sprintf("issue-3587-reviewer-%d.json", round), result)
		if err != nil {
			return err
		}
		observation := r.run([]string{
			"review", "capture-result", "--cwd", r.sandbox.Repo,
			"--lineage", envelope.argument("lineage"),
			"--target", envelope.argument("target"),
			"--expected-revision", envelope.argument("expected-revision"),
			"--lens", envelope.argument("lens"),
			"--order", envelope.argument("order"),
			"--input", path,
		}, true)
		if observation.ExitCode != 0 {
			return fmt.Errorf("capture reviewer round %d: %s", round, firstLine(observation.Stderr))
		}
		var output struct {
			Schema    string `json:"schema"`
			Operation string `json:"operation"`
			LineageID string `json:"lineage_id"`
			State     string `json:"state"`
		}
		if err := json.Unmarshal([]byte(observation.Stdout), &output); err != nil {
			return fmt.Errorf("decode reviewer capture round %d: %w", round, err)
		}
		if output.Schema != "gentle-ai.review-last-event-closure/v1" {
			continue
		}
		if output.Operation != "review/capture-result" || output.LineageID != issue3587Lineage || output.State != "approved" {
			return fmt.Errorf("terminal reviewer capture = %+v", output)
		}
		return nil
	}
	return fmt.Errorf("reviewer captures never produced terminal last-event closure")
}
