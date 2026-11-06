package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// issue3772Journeys proves that global review-mode writes survive the fresh
// product processes that operators use to read status. #3772 requires status
// to name both the persisted global value and the source that decided it.
func issue3772Journeys() []Journey {
	return []Journey{{
		ID:     "j119-global-review-mode-status-reports-persisted-source",
		Review: reviewUntouched,
		Title:  "#3772: global review mode status reports persisted source across fresh processes",
		Source: "#3772: status must truthfully expose the persisted global source after enable and disable",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "enable global review mode", Requires: modeCapability,
				Args: productArgs("review", "mode", "enable", "--scope", "global", "--json"), After: issue3772ModeStatus("enable", "global", "on")},
			{Name: "fresh status reports enabled global source", Requires: modeCapability,
				Args: productArgs("review", "mode", "status", "--json"), After: issue3772ModeStatus("status", "global", "on")},
			{Name: "disable global review mode", Requires: modeCapability,
				Args: productArgs("review", "mode", "disable", "--scope", "global", "--json"), After: issue3772ModeStatus("disable", "global", "off")},
			{Name: "fresh status reports disabled global source", Requires: modeCapability,
				Args: productArgs("review", "mode", "status", "--json"), After: issue3772ModeStatus("status", "global", "off")},
		},
	}}
}

func issue3772ModeStatus(operation, source, global string) func(*Sandbox, Observation) error {
	return func(_ *Sandbox, observation Observation) error {
		var result struct {
			Operation string `json:"operation"`
			Scope     string `json:"scope"`
			Status    struct {
				Effective string `json:"effective"`
				Source    string `json:"source"`
				Global    string `json:"global"`
			} `json:"status"`
		}
		if observation.ExitCode != 0 {
			return fmt.Errorf("review mode %s exited %d: %s", operation, observation.ExitCode, firstLine(observation.Stderr, observation.Stdout))
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &result); err != nil {
			return fmt.Errorf("parse review mode %s JSON: %w", operation, err)
		}
		if result.Operation != operation || result.Scope != "both" && operation == "status" ||
			result.Status.Effective != global || result.Status.Source != source || result.Status.Global != global {
			return fmt.Errorf("review mode %s = operation=%q scope=%q effective=%q source=%q global=%q, want operation=%q source=%q global/effective=%q", operation, result.Operation, result.Scope, result.Status.Effective, result.Status.Source, result.Status.Global, operation, source, global)
		}
		return nil
	}
}
