package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

const findingIDPrefixScratchKey = "finding-id-prefix:"

func findingIDPrefixJourneys() []Journey {
	return []Journey{
		{
			ID:     "j78-lens-finding-id-prefix-discovery",
			Title:  "Reviewer discovers lens finding-ID prefixes before native admission",
			Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/1844",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage high-risk code", Fixture: stageAuthCode},
				{Name: "review start discloses canonical prefixes", Requires: startCapability,
					Args: productArgs("review", "start"), After: rememberFindingIDPrefixes},
				{Name: "capture explicit IDs from disclosed prefixes", Requires: captureResultCapability,
					Composite: captureDisclosedFindingIDPrefixes},
			},
		},
	}
}

func rememberFindingIDPrefixes(sandbox *Sandbox, observation Observation) error {
	if observation.ExitCode != 0 {
		return fmt.Errorf("review start exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
	}
	if err := rememberLineage(sandbox, observation); err != nil {
		return err
	}
	var start struct {
		LensBindings []struct {
			Lens            string `json:"lens"`
			FindingIDPrefix string `json:"finding_id_prefix"`
		} `json:"lens_bindings"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &start); err != nil {
		return fmt.Errorf("parse review start: %w", err)
	}
	want := map[string]string{
		"review-risk":        "R1-",
		"review-readability": "R2-",
		"review-reliability": "R3-",
		"review-resilience":  "R4-",
	}
	if len(start.LensBindings) != len(want) {
		return fmt.Errorf("review start lens bindings = %#v, want all four canonical mappings", start.LensBindings)
	}
	seen := map[string]bool{}
	for _, binding := range start.LensBindings {
		prefix, known := want[binding.Lens]
		if !known || binding.FindingIDPrefix != prefix || seen[binding.Lens] {
			return fmt.Errorf("review start lens binding = %#v, want unique canonical mapping", binding)
		}
		seen[binding.Lens] = true
		sandbox.Scratch[findingIDPrefixScratchKey+binding.Lens] = binding.FindingIDPrefix
	}
	return nil
}

func captureDisclosedFindingIDPrefixes(r *journeyRun) error {
	for round := 0; round < 4; round++ {
		envelope, err := readStatus(r)
		if err != nil {
			return err
		}
		if envelope.NextTransition.Kind != "collect" || len(envelope.NextTransition.Collect.Inputs) == 0 ||
			envelope.NextTransition.Collect.Inputs[0].Name != "reviewer_result" {
			return fmt.Errorf("capture round %d did not publish a reviewer-result collection", round+1)
		}
		lens := envelope.argument("lens")
		prefix := r.sandbox.Scratch[findingIDPrefixScratchKey+lens]
		if prefix == "" {
			return fmt.Errorf("capture lens %q has no prefix disclosed by review start", lens)
		}
		paths := envelope.paths()
		if len(paths) == 0 {
			return fmt.Errorf("capture lens %q has no frozen inspection paths", lens)
		}
		location := paths[0] + ":1"
		payload, err := json.Marshal(map[string]any{
			"subject_hash": envelope.NextTransition.Collect.Inputs[0].ArtifactSubject.SubjectHash,
			"inspection":   map[string]any{"status": "completed", "paths": paths},
			"findings": []any{map[string]any{
				"id": prefix + fmt.Sprintf("bench-%d", round+1), "location": location,
				"severity": "WARNING", "claim": "the mapped finding ID reaches native admission",
				"proof_refs": []string{location},
			}},
			"evidence": []string{"inspected the complete frozen candidate scope named by the capture binding"},
		})
		if err != nil {
			return err
		}
		path, err := writeScratch(r.sandbox, fmt.Sprintf("mapped-reviewer-%d.json", round), payload)
		if err != nil {
			return err
		}
		observation := r.run([]string{
			"review", "capture-result", "--cwd", r.sandbox.Repo,
			"--lineage", envelope.argument("lineage"),
			"--target", envelope.argument("target"),
			"--expected-revision", envelope.argument("expected-revision"),
			"--lens", lens,
			"--order", envelope.argument("order"),
			"--input", path,
		}, true)
		if observation.ExitCode != 0 {
			return fmt.Errorf("capture mapped ID for %q exited %d: %s", lens, observation.ExitCode, firstLine(observation.Stderr))
		}
		var captured struct {
			AdmissionDecision string `json:"admission_decision"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &captured); err != nil || captured.AdmissionDecision != "completed" {
			return fmt.Errorf("capture mapped ID for %q = %q, %v; want completed native admission", lens, observation.Stdout, err)
		}
	}
	return nil
}
