package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

const modelPickerAxis = "model-picker"

var modelPickerCapability = &Capability{
	Verb:  []string{"bench-model-picker"},
	Flags: []string{"--json"},
}

func init() {
	RegisterAxis(Axis{
		Name:     modelPickerAxis,
		Title:    "OpenCode custom-agent model-picker runtime proof",
		BlackBox: false,
		Review:   reviewUntouched,
		Properties: []string{
			"j97 drives the compiled TUI model-picker state through the public gentle-ai binary and checks the persisted opencode.json boundary.",
			"The journey requires a product binary built with -tags bench_fixture; ordinary binaries report unsupported instead of fabricating a pass.",
			"The fixture uses a fresh HOME and opencode.json containing an unconfigured custom native agent plus a tool-call-capable model provider.",
		},
		Journeys: modelPickerJourneys,
	})
}

func modelPickerJourneys() []Journey {
	return []Journey{{
		ID:     "j97-opencode-custom-agent-model-picker-runtime",
		Review: reviewUntouched,
		Title:  "Runtime model picker discovers and persists a custom native agent assignment",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/2098",
		Steps: []Step{
			{Name: "fixture: unconfigured custom agent and selectable model", Fixture: modelPickerFixture},
			{Name: "public model-picker runtime exposes and persists the custom assignment", Requires: modelPickerCapability,
				Args: func(*Sandbox) ([]string, error) {
					return []string{"bench-model-picker", "--json"}, nil
				}, After: modelPickerAfter},
		},
	}}
}

func modelPickerFixture(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	settingsPath := filepath.Join(sandbox.Home, ".config", "opencode", "opencode.json")
	settings := `{
  "provider": {
    "bench-provider": {
      "name": "Bench Provider",
      "models": {
        "bench-model": { "name": "Bench Model", "tool_call": true }
      }
    }
  },
  "agent": {
    "custom-refactor-agent": {
      "mode": "subagent",
      "description": "preserve this custom agent"
    }
  }
}`
	if err := sandbox.write(settingsPath, settings); err != nil {
		return fmt.Errorf("write model-picker settings: %w", err)
	}
	return sandbox.write(filepath.Join(sandbox.Home, ".cache", "opencode", "models.json"), "{}\n")
}

func modelPickerAfter(sandbox *Sandbox, observation Observation) error {
	var result struct {
		CustomAgent          string `json:"custom_agent"`
		RowVisible           bool   `json:"row_visible"`
		AssignmentVisible    bool   `json:"assignment_visible"`
		SelectedAssignment   string `json:"selected_assignment"`
		PersistedModel       string `json:"persisted_model"`
		PersistedVariant     string `json:"persisted_variant"`
		VariantPresent       bool   `json:"variant_present"`
		DescriptionPreserved bool   `json:"description_preserved"`
	}
	if err := json.Unmarshal([]byte(observation.Stdout), &result); err != nil {
		return fmt.Errorf("parse model-picker runtime result: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	if result.CustomAgent != "custom-refactor-agent" {
		return fmt.Errorf("runtime custom agent = %q, want custom-refactor-agent", result.CustomAgent)
	}
	if !result.RowVisible || !result.AssignmentVisible {
		return fmt.Errorf("runtime picker did not expose the corrected custom row and assignment: %+v", result)
	}
	if result.SelectedAssignment != "bench-provider/bench-model" {
		return fmt.Errorf("selected assignment = %q, want bench-provider/bench-model", result.SelectedAssignment)
	}
	if result.PersistedModel != result.SelectedAssignment {
		return fmt.Errorf("persisted model = %q, want %q", result.PersistedModel, result.SelectedAssignment)
	}
	if !result.VariantPresent || result.PersistedVariant != "" {
		return fmt.Errorf("persisted variant = %q (present=%t), want present empty variant", result.PersistedVariant, result.VariantPresent)
	}
	if !result.DescriptionPreserved {
		return fmt.Errorf("custom-agent description was not preserved: %+v", result)
	}
	return nil
}
