package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// j94 is the public/runtime replay for #1867. It spends two failed reviewer
// submissions on one frozen slot: a schema-valid zero-finding result that says
// the candidate was unreadable, and a response ending inside JSON. Neither
// failure may advance authority; a complete retry must still use that slot.
func unreadableCaptureJourneys() []Journey {
	return []Journey{{
		ID:     "j94-review-rejects-unreadable-and-truncated-capture",
		Title:  "Unreadable candidate and truncated capture fail closed, then recapture",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/1867",
		Steps: []Step{
			{Name: "fixture: repo", Fixture: baseRepo},
			{Name: "fixture: stage auth code", Fixture: stageAuthCode},
			{Name: "review start", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
			{Name: "reject unreadable and truncated results, then recapture", Requires: captureResultCapability, Composite: exerciseUnreadableCaptureRecovery},
		},
	}}
}

func exerciseUnreadableCaptureRecovery(r *journeyRun) error {
	envelope, err := readStatus(r)
	if err != nil {
		return err
	}
	if envelope.NextTransition.Kind != "collect" || len(envelope.NextTransition.Collect.Inputs) == 0 {
		return errors.New("expected reviewer-result collect transitions")
	}
	input := envelope.NextTransition.Collect.Inputs[0]
	if input.Name != "reviewer_result" || input.ArtifactSubject.SubjectHash == "" || len(envelope.paths()) == 0 {
		return fmt.Errorf("reviewer collect input is incomplete: %#v", input)
	}
	initialRevision := envelope.Authority.Revision
	args := func(path string) []string {
		return []string{
			"review", "capture-result", "--cwd", r.sandbox.Repo,
			"--lineage", envelope.argument("lineage"), "--target", envelope.argument("target"),
			"--expected-revision", envelope.argument("expected-revision"), "--lens", envelope.argument("lens"),
			"--order", envelope.argument("order"), "--input", path,
		}
	}
	assertStillCollecting := func(label string) error {
		after, statusErr := readStatus(r)
		if statusErr != nil {
			return statusErr
		}
		if after.Authority.Revision != initialRevision || after.NextTransition.Kind != "collect" ||
			after.argument("lineage") != envelope.argument("lineage") || after.argument("lens") != envelope.argument("lens") ||
			after.argument("order") != envelope.argument("order") {
			return fmt.Errorf("%s changed the authority or collect binding: before revision=%q after=%#v", label, initialRevision, after)
		}
		return nil
	}

	unreadable, err := json.Marshal(map[string]any{
		"subject_hash": input.ArtifactSubject.SubjectHash,
		"inspection":   map[string]any{"status": "completed", "paths": envelope.paths()},
		"findings":     []any{},
		"evidence":     []string{"candidate input read denied by filesystem; inspection could not occur"},
	})
	if err != nil {
		return err
	}
	unreadable = append(unreadable, '\n')
	path, err := writeScratch(r.sandbox, "reviewer-unreadable.json", unreadable)
	if err != nil {
		return err
	}
	observation := r.run(args(path), true)
	if observation.ExitCode == 0 || !strings.Contains(observation.Stderr+observation.Stdout, "candidate_input_unreadable") {
		return fmt.Errorf("unreadable candidate result = exit %d, stderr=%q", observation.ExitCode, firstLine(observation.Stderr))
	}
	if err := assertStillCollecting("unreadable candidate result"); err != nil {
		return err
	}

	valid, err := synthesizeReviewerResult(input.ArtifactSubject.SubjectHash, envelope.paths())
	if err != nil {
		return err
	}
	if len(valid) < 2 {
		return errors.New("synthesized reviewer result is too short to truncate")
	}
	truncatedPath, err := writeScratch(r.sandbox, "reviewer-truncated.json", valid[:len(valid)-1])
	if err != nil {
		return err
	}
	observation = r.run(args(truncatedPath), true)
	if observation.ExitCode == 0 || !strings.Contains(observation.Stderr+observation.Stdout, "truncated_capture") {
		return fmt.Errorf("truncated reviewer result = exit %d, stderr=%q", observation.ExitCode, firstLine(observation.Stderr))
	}
	if err := assertStillCollecting("truncated reviewer result"); err != nil {
		return err
	}

	validPath, err := writeScratch(r.sandbox, "reviewer-corrected.json", valid)
	if err != nil {
		return err
	}
	if observation = r.run(args(validPath), true); observation.ExitCode != 0 {
		return fmt.Errorf("corrected reviewer result failed: %s", firstLine(observation.Stderr))
	}
	final, err := readStatus(r)
	if err != nil {
		return err
	}
	if final.NextTransition.Kind != "collect" ||
		(final.argument("lens") == envelope.argument("lens") && final.argument("order") == envelope.argument("order")) {
		return fmt.Errorf("corrected capture did not advance the exact collect slot: %#v", final)
	}
	return nil
}
