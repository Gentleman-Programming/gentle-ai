package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// issue3813Journeys proves global review-mode controls from the place an
// operator naturally runs machine-wide settings: a directory that is not inside
// any Git repository. The sandbox owns HOME, so the journey can mutate global
// review mode without reading or changing the real user's state.
func issue3813Journeys() []Journey {
	return []Journey{{
		ID:     "j122-global-review-mode-from-non-git-cwd",
		Review: reviewUntouched,
		Title:  "#3813: global review mode works from a non-Git cwd",
		Source: "#3813: global scope has no repository dimension and must not fail after writing isolated HOME state",
		Steps: []Step{
			{Name: "fixture: non-Git working directory", Fixture: nonGitReviewModeCWD},
			{Name: "enable global review mode outside Git through a PTY",
				Composite: issue3813RunTTY("enable", "global", "on", "review", "mode", "enable", "--scope", "global", "--json")},
			{Name: "fresh status reports enabled global source outside Git through a PTY",
				Composite: issue3813RunTTY("status", "global", "on", "review", "mode", "status", "--json")},
			{Name: "disable global review mode outside Git through a PTY",
				Composite: issue3813RunTTY("disable", "global", "off", "review", "mode", "disable", "--scope", "global", "--json")},
		},
	}}
}

func nonGitReviewModeCWD(sandbox *Sandbox) error {
	nonGit := filepath.Join(sandbox.Root, "not-a-git-repo")
	if err := os.MkdirAll(nonGit, 0o755); err != nil {
		return err
	}
	sandbox.Repo = nonGit
	return nil
}

func issue3813RunTTY(operation, source, global string, args ...string) func(*journeyRun) error {
	return func(run *journeyRun) error {
		observation, err := run.runTTY(args, false, issue3813NoTTYInput)
		if err != nil {
			return err
		}
		return issue3813ModeStatus(operation, source, global)(run.sandbox, observation)
	}
}

func issue3813NoTTYInput(_ *bufio.Reader, _ io.WriteCloser) error { return nil }

func issue3813ModeStatus(operation, source, global string) func(*Sandbox, Observation) error {
	return func(sandbox *Sandbox, observation Observation) error {
		var result struct {
			Operation string `json:"operation"`
			Scope     string `json:"scope"`
			Status    struct {
				Effective  string `json:"effective"`
				Source     string `json:"source"`
				Global     string `json:"global"`
				CloneLocal string `json:"clone_local"`
			} `json:"status"`
		}
		if observation.ExitCode != 0 {
			return fmt.Errorf("review mode %s outside Git exited %d: %s", operation, observation.ExitCode, firstLine(observation.Stderr, observation.Stdout))
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &result); err != nil {
			return fmt.Errorf("parse review mode %s JSON: %w", operation, err)
		}
		if result.Operation != operation || result.Status.Effective != global || result.Status.Source != source ||
			result.Status.Global != global || result.Status.CloneLocal != "" {
			return fmt.Errorf("review mode %s outside Git = operation=%q effective=%q source=%q global=%q clone=%q, want operation=%q source=%q global/effective=%q and unset clone", operation, result.Operation, result.Status.Effective, result.Status.Source, result.Status.Global, result.Status.CloneLocal, operation, source, global)
		}
		if _, err := os.Lstat(filepath.Join(sandbox.Repo, ".git")); !os.IsNotExist(err) {
			return fmt.Errorf("review mode %s created Git metadata in non-Git cwd: %v", operation, err)
		}
		if !strings.HasPrefix(filepath.Clean(sandbox.Home), filepath.Clean(sandbox.Root)) {
			return fmt.Errorf("journey HOME %s is outside sandbox root %s", sandbox.Home, sandbox.Root)
		}
		return nil
	}
}
