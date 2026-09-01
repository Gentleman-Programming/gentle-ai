package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

type issue2696Status struct {
	ArtifactPaths struct {
		Specs []string `json:"specs"`
	} `json:"artifactPaths"`
	Artifacts    map[string]string `json:"artifacts"`
	Dependencies struct {
		Specs string `json:"specs"`
		Apply string `json:"apply"`
	} `json:"dependencies"`
	NextRecommended string   `json:"nextRecommended"`
	BlockedReasons  []string `json:"blockedReasons"`
}

var sddContinueCapability = &Capability{
	Verb:  []string{"sdd-continue"},
	Probe: []string{"sdd-continue", "--json"},
}

func issue2696Journeys() []Journey {
	return []Journey{{
		ID:     "j98-sdd-flat-root-spec-is-discovered",
		Review: reviewOptedIn,
		Title:  "Flat-root spec is discovered consistently by status and continue",
		Source: "issue #2696: approved flat-root spec.md artifact discovery contract",
		Steps: []Step{
			{Name: "fixture: committed change with a flat root spec", Fixture: issue2696FlatSpecFixture},
			{Name: "sdd-status reports the flat spec", Requires: sddStatusCapability,
				Args: productArgs("sdd-status", sddChange, "--json"), After: issue2696StatusAssertion("sdd-status")},
			{Name: "sdd-continue reports the same state", Requires: sddContinueCapability,
				Args: productArgs("sdd-continue", sddChange, "--json"), After: issue2696StatusAssertion("sdd-continue")},
		},
	}}
}

func issue2696FlatSpecFixture(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	root := sddChangeRoot(sandbox)
	files := []struct {
		name    string
		content string
	}{
		{name: "proposal.md", content: "# Flat spec change\n"},
		{name: "spec.md", content: "### Requirement: flat spec\n#### Scenario: present\n\n- **WHEN** status runs\n- **THEN** the root spec is detected\n"},
		{name: "design.md", content: "# Design\n"},
		{name: "tasks.md", content: "- [ ] 1.1 Work\n"},
	}
	for _, file := range files {
		if err := sandbox.write(filepath.Join(root, file.name), file.content); err != nil {
			return err
		}
	}
	if err := sandbox.git(sandbox.Repo, "add", "openspec"); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "commit", "-qm", "flat root spec fixture")
}

func issue2696StatusAssertion(command string) func(*Sandbox, Observation) error {
	return func(sandbox *Sandbox, observation Observation) error {
		var status issue2696Status
		if err := json.Unmarshal([]byte(observation.Stdout), &status); err != nil {
			return fmt.Errorf("parse %s JSON: %w", command, err)
		}
		expectedSpec := filepath.Join(sddChangeRoot(sandbox), "spec.md")
		if len(status.ArtifactPaths.Specs) != 1 || status.ArtifactPaths.Specs[0] != expectedSpec {
			return fmt.Errorf("%s artifactPaths.specs = %v, want [%s]", command, status.ArtifactPaths.Specs, expectedSpec)
		}
		if status.Artifacts["specs"] != "done" {
			return fmt.Errorf("%s artifacts.specs = %q, want done", command, status.Artifacts["specs"])
		}
		if status.Dependencies.Specs != "all_done" || status.Dependencies.Apply != "ready" {
			return fmt.Errorf("%s dependencies = %#v, want specs all_done/apply ready", command, status.Dependencies)
		}
		if status.NextRecommended != "apply" || len(status.BlockedReasons) != 0 {
			return fmt.Errorf("%s routing = next %q blocked %v, want apply and no blockers", command, status.NextRecommended, status.BlockedReasons)
		}
		fingerprint := fmt.Sprintf("%v|%s|%s|%s|%v", status.ArtifactPaths.Specs, status.Artifacts["specs"], status.Dependencies.Specs, status.NextRecommended, status.BlockedReasons)
		if previous := sandbox.Scratch["issue-2696-status"]; previous != "" && previous != fingerprint {
			return fmt.Errorf("%s state differs from the earlier public command: %q != %q", command, fingerprint, previous)
		}
		sandbox.Scratch["issue-2696-status"] = fingerprint
		return nil
	}
}
