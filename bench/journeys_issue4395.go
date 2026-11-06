package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

var issue4395GlobalModeCapability = &Capability{Verb: []string{"review", "mode"}, Flags: []string{"--cwd", "--scope", "--json"}}

// issue4395Journeys keeps the public telemetry trigger on its repository-aware
// path. The Windows console assertion belongs to native Windows tests; this
// portable journey proves the trigger and preview reach RDD's Git resolver.
// The sandbox-local opt-out makes the trigger inert for dev and release builds,
// so it cannot enroll or contact an external telemetry server.
func issue4395Journeys() []Journey {
	return []Journey{{
		ID:     "j4395-telemetry-trigger-resolves-rdd-repository",
		Review: reviewUntouched,
		Title:  "#4395: telemetry trigger resolves RDD mode through the repository Git boundary",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/4395",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "enable global review mode", Requires: issue4395GlobalModeCapability, Args: productArgs("review", "mode", "enable", "--scope", "global", "--json")},
			{Name: "fixture: disable sandbox telemetry", Fixture: issue4395DisableSandboxTelemetry},
			{Name: "telemetry trigger resolves repository RDD mode", Args: productArgs("telemetry", "trigger", "--json"), After: issue4395TriggerDisabledAfterSandboxOptOut},
			{Name: "telemetry preview exposes repository RDD mode", Args: productArgs("telemetry", "preview", "--json"), After: issue4395PreviewReportsRDDEnabled},
		},
	}}
}

func issue4395DisableSandboxTelemetry(sandbox *Sandbox) error {
	observation := sandbox.readBackAt(sandbox.Repo, "telemetry", "disable", "--json")
	var result struct {
		Schema    string `json:"schema"`
		Operation string `json:"operation"`
		Enabled   bool   `json:"enabled"`
		Source    string `json:"source"`
	}
	if observation.ExitCode != 0 {
		return fmt.Errorf("telemetry disable exited %d: %s", observation.ExitCode, firstLine(observation.Stderr, observation.Stdout))
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &result); err != nil {
		return fmt.Errorf("parse telemetry disable JSON: %w", err)
	}
	if result.Schema != "gentle-ai.telemetry-status/v1" || result.Operation != "disable" || result.Enabled || result.Source != "state" {
		return fmt.Errorf("telemetry disable = schema=%q operation=%q enabled=%t source=%q, want telemetry-status/v1/disable/false/state", result.Schema, result.Operation, result.Enabled, result.Source)
	}
	return nil
}

func issue4395TriggerDisabledAfterSandboxOptOut(_ *Sandbox, observation Observation) error {
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
	if result.Schema != "gentle-ai.telemetry-trigger/v1" || result.Decision != "disabled" || result.Source != "state" {
		return fmt.Errorf("telemetry trigger = schema=%q decision=%q source=%q, want telemetry-trigger/v1/disabled/state after the sandbox opt-out", result.Schema, result.Decision, result.Source)
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
