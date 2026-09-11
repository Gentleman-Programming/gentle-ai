package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// issue4395Journeys keeps the public telemetry trigger on its repository-aware
// path. The Windows console assertion belongs to native Windows tests; this
// portable journey proves the trigger and preview reach RDD's Git resolver.
func issue4395Journeys() []Journey {
	return []Journey{{
		ID:     "j4395-telemetry-trigger-resolves-rdd-repository",
		Review: reviewUntouched,
		Title:  "#4395: telemetry trigger resolves RDD mode through the repository Git boundary",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/4395",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "enable global review mode", Requires: modeCapability, Args: productArgs("review", "mode", "enable", "--scope", "global", "--json")},
			{Name: "telemetry trigger resolves repository RDD mode", Args: productArgs("telemetry", "trigger", "--json"), After: issue4395TriggerEnrolled},
			{Name: "telemetry preview exposes repository RDD mode", Args: productArgs("telemetry", "preview", "--json"), After: issue4395PreviewReportsRDDEnabled},
		},
	}}
}

func issue4395TriggerEnrolled(_ *Sandbox, observation Observation) error {
	var result struct {
		Schema   string `json:"schema"`
		Decision string `json:"decision"`
		Source   string `json:"source"`
	}
	if observation.ExitCode != 0 {
		return fmt.Errorf("telemetry trigger exited %d: %s", observation.ExitCode, firstLine(observation.Stderr, observation.Stdout))
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &result); err != nil {
		return fmt.Errorf("parse telemetry trigger JSON: %w", err)
	}
	if result.Schema != "gentle-ai.telemetry-trigger/v1" || result.Decision != "disabled" || result.Source != "default" {
		return fmt.Errorf("telemetry trigger = schema=%q decision=%q source=%q, want telemetry-trigger/v1/disabled/default for a dev build", result.Schema, result.Decision, result.Source)
	}
	return nil
}

func issue4395PreviewReportsRDDEnabled(_ *Sandbox, observation Observation) error {
	var event struct {
		Schema     string `json:"schema"`
		RDDEnabled bool   `json:"rdd_enabled"`
	}
	if observation.ExitCode != 0 {
		return fmt.Errorf("telemetry preview exited %d: %s", observation.ExitCode, firstLine(observation.Stderr, observation.Stdout))
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &event); err != nil {
		return fmt.Errorf("parse telemetry preview JSON: %w", err)
	}
	if event.Schema != "gentle-ai.telemetry-event/v1" || !event.RDDEnabled {
		return fmt.Errorf("telemetry preview = schema=%q rdd_enabled=%t, want telemetry-event/v1 and true", event.Schema, event.RDDEnabled)
	}
	return nil
}
