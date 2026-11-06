package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// issue4377Journeys drives the current customizable installer through its RDD
// choice under a real PTY. The sandbox HOME is isolated, and the exchange
// cancels before the installation plan, proving a cancelled choice cannot
// change the user's global configuration. The current flow exposes RDD as a
// configuration toggle before the plan; the retired deferred-review screen is
// not part of this contract.
func issue4377Journeys() []Journey {
	return []Journey{{
		ID:     "j127-customizable-install-rdd-choice",
		Review: reviewUntouched,
		Title:  "#4377: customizable installation exposes the RDD default and cancellation",
		Source: "#4377 requires a visible global RDD default and no mutation when the installation is cancelled",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "fixture: update-check cooldown in the sandbox HOME", Fixture: issue3766UpdateCooldownFixture},
			{Name: "fixture: detected Claude configuration", Fixture: func(sandbox *Sandbox) error {
				return sandbox.write(filepath.Join(sandbox.Home, ".claude", "settings.json"), "{}\n")
			}},
			{Name: "customizable installer presents default ON before review and permits opting out", Composite: func(run *journeyRun) error {
				observation, err := run.runTTY(nil, false, issue4377TTYExchange)
				if err != nil {
					return err
				}
				if observation.ExitCode != 0 {
					return fmt.Errorf("customizable installer TUI exited %d: %s", observation.ExitCode, strings.TrimSpace(observation.Stderr))
				}
				return issue4377CancelledModeInheritsDefault(run.sandbox)
			}},
		},
	}}
}

func issue4377TTYExchange(reader *bufio.Reader, writer io.WriteCloser) error {
	return waitForIssue4377TTY(reader, []string{"Start installation", "q: quit"}, func() error {
		if _, err := io.WriteString(writer, "\r"); err != nil {
			return err
		}
		return waitForIssue4377TTY(reader, []string{"Configure Gentle-Shell experience", "[x] Global RDD: ON", "End: continue"}, func() error {
			// Cancel from configuration before the plan exists. The visible
			// default is enough to prove the operator can inspect it; the
			// readback proves cancellation persisted nothing.
			if _, err := io.WriteString(writer, "\x1b"); err != nil {
				return err
			}
			return waitForIssue4377TTY(reader, []string{"Start installation", "q: quit"}, func() error {
				_, err := io.WriteString(writer, "q")
				return err
			})
		})
	})
}

// issue4377CancelledModeInheritsDefault is deliberately black-box: status is
// read-only, and both sources must remain unset after cancelling the choice.
func issue4377CancelledModeInheritsDefault(sandbox *Sandbox) error {
	observation := sandbox.readBack("review", "mode", "status", "--cwd", sandbox.Repo, "--json")
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
		return fmt.Errorf("review mode status after cancellation exited %d: %s", observation.ExitCode, firstLine(observation.Stderr, observation.Stdout))
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &result); err != nil {
		return fmt.Errorf("parse review mode status after cancellation: %w", err)
	}
	if result.Operation != "status" || result.Scope != "both" || result.Status.Effective != "on" ||
		result.Status.Source != "default" || result.Status.Global != "" || result.Status.CloneLocal != "" {
		return fmt.Errorf("cancelled review mode = operation=%q scope=%q effective=%q source=%q global=%q clone=%q, want status/both/on/default and unset sources", result.Operation, result.Scope, result.Status.Effective, result.Status.Source, result.Status.Global, result.Status.CloneLocal)
	}
	return nil
}

func waitForIssue4377TTY(reader *bufio.Reader, required []string, next func() error, matched ...func(string)) error {
	var screen strings.Builder
	for {
		byteRead, err := reader.ReadByte()
		if err != nil {
			return fmt.Errorf("read TUI before %q: %w; output: %q", strings.Join(required, ", "), err, screen.String())
		}
		screen.WriteByte(byteRead)
		allMatched := true
		for _, text := range required {
			if !strings.Contains(screen.String(), text) {
				allMatched = false
				break
			}
		}
		if allMatched {
			if len(matched) > 0 {
				matched[0](screen.String())
			}
			return next()
		}
	}
}
