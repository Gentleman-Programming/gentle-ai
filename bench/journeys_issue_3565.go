package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

const issue3565Axis = "issue-3565-skill-registry-cwd"

func init() {
	RegisterAxis(Axis{
		Name:     issue3565Axis,
		Title:    "Skill registry commands canonicalize explicit relative cwd values",
		BlackBox: true,
		Properties: []string{
			"Replays #3565 through the public CLI only: `skill-registry list --json --cwd .` and `skill-registry refresh --cwd .`.",
			"The fixture is a normal git worktree with a project-local skill; failure means literal dot cwd was treated as filesystem root or as a user-scope path.",
		},
		Review:   reviewUntouched,
		Journeys: issue3565Journeys,
	})
}

func issue3565ProjectSkillFixture(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	return sandbox.write(filepath.Join(sandbox.Repo, "skills", "project-local", "SKILL.md"), `---
name: project-local
description: project skill visible from literal dot cwd
---

Use the project skill.
`)
}

func issue3565ListDotArgs(*Sandbox) ([]string, error) {
	return []string{"skill-registry", "list", "--json", "--cwd", "."}, nil
}

func issue3565RefreshDotArgs(*Sandbox) ([]string, error) {
	return []string{"skill-registry", "refresh", "--cwd", "."}, nil
}

func issue3565VerifyListDot(sandbox *Sandbox, observation Observation) error {
	if observation.ExitCode != 0 {
		return fmt.Errorf("list --json --cwd . exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
	}
	var rows []struct {
		Name  string `json:"name"`
		Scope string `json:"scope"`
		Path  string `json:"path"`
	}
	if err := json.Unmarshal([]byte(observation.Stdout), &rows); err != nil {
		return fmt.Errorf("parse list --json --cwd . output: %w (stdout: %s)", err, firstLine(observation.Stdout))
	}
	if len(rows) != 1 {
		return fmt.Errorf("list --json --cwd . returned %#v, want exactly the project skill", rows)
	}
	wantPath := filepath.Join(sandbox.Repo, "skills", "project-local", "SKILL.md")
	if rows[0].Name != "project-local" || rows[0].Scope != "project" || !filepath.IsAbs(rows[0].Path) || filepath.Clean(rows[0].Path) != wantPath {
		return fmt.Errorf("list --json --cwd . row = %#v, want project-local/project/%s", rows[0], wantPath)
	}
	return nil
}

func issue3565VerifyRefreshDot(sandbox *Sandbox, observation Observation) error {
	if observation.ExitCode != 0 {
		return fmt.Errorf("refresh --cwd . exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
	}
	if strings.Contains(observation.Stdout, "filesystem-root") || strings.Contains(observation.Stdout, "skipped") {
		return fmt.Errorf("refresh --cwd . skipped instead of refreshing project registry: %s", firstLine(observation.Stdout))
	}
	if !strings.Contains(observation.Stdout, "Skill registry refreshed") {
		return fmt.Errorf("refresh --cwd . did not report a refreshed project registry: %s", firstLine(observation.Stdout))
	}
	return nil
}

func issue3565Journeys() []Journey {
	return []Journey{{
		ID:     "j3565-skill-registry-dot-cwd",
		Title:  "#3565: skill-registry list and refresh honor literal dot cwd",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/3565",
		Steps: []Step{
			{Name: "fixture: git project with a project-local skill", Fixture: issue3565ProjectSkillFixture},
			{Name: "list --json --cwd . reports the project skill as project-scoped", Args: issue3565ListDotArgs, After: issue3565VerifyListDot},
			{Name: "refresh --cwd . writes the project registry instead of filesystem-root skip", Args: issue3565RefreshDotArgs, After: issue3565VerifyRefreshDot},
		},
	}}
}
