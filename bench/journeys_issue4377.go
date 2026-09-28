package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// issue4377Journeys drives the customizable installer through its RDD choice
// and final confirmation under a real PTY. The sandbox HOME is isolated, and
// the exchange quits before installation, proving a cancelled choice cannot
// change the user's global configuration. #4377 tests the deferred RDD choice;
// ODD's default applicable test-first policy is not an installer choice.
func issue4377Journeys() []Journey {
	return []Journey{{
		ID:     "j127-customizable-install-rdd-choice",
		Review: reviewUntouched,
		Title:  "#4377: customizable installation explains, revises, and summarizes the deferred RDD choice",
		Source: "#4377 requires a deferred global RDD choice before final installation confirmation without mutating a cancelled installation",
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
	agentCheckboxRows := 0
	communityToolCursorRows := 0
	return waitForIssue4377TTY(reader, []string{"Start installation", "q: quit"}, func() error {
		if _, err := io.WriteString(writer, "\r"); err != nil {
			return err
		}
		return waitForIssue4377TTY(reader, []string{"Detected Configs", "Continue"}, func() error {
			if _, err := io.WriteString(writer, "\r"); err != nil {
				return err
			}
			return waitForIssue4377TTY(reader, []string{"[x] claude-code", "Continue"}, func() error {
				if agentCheckboxRows == 0 {
					return fmt.Errorf("agent picker rendered no checkbox rows")
				}
				if _, err := io.WriteString(writer, strings.Repeat("\x1b[B", agentCheckboxRows)+"\r"); err != nil {
					return err
				}
				return waitForIssue4377TTY(reader, []string{"Choose your Persona", "gentleman"}, func() error {
					if _, err := io.WriteString(writer, "\r"); err != nil {
						return err
					}
					return waitForIssue4377TTY(reader, []string{"Select Ecosystem Preset", "Memory Only"}, func() error {
						if _, err := io.WriteString(writer, strings.Repeat("\x1b[B", 2)+"\r"); err != nil {
							return err
						}
						// #4377 keeps the deferred RDD decision after community tools;
						// there is no separate Strict TDD installer choice to confirm.
						return waitForIssue4377TTY(reader, []string{"Community Tools/Plugins", "Continue"}, func() error {
							if communityToolCursorRows == 0 {
								return fmt.Errorf("community tools rendered no tool rows")
							}
							if _, err := io.WriteString(writer, strings.Repeat("\x1b[B", communityToolCursorRows)+"\r"); err != nil {
								return err
							}
							return waitForIssue4377TTY(reader, []string{"Install Plan", "Continue"}, func() error {
								if _, err := io.WriteString(writer, "\r"); err != nil {
									return err
								}
								return waitForIssue4377TTY(reader, []string{
									"Receipt-Driven Development",
									"RDD is ON by default. You can opt out.",
									"Disable RDD",
									"No global RDD preference is configured.",
								}, func() error {
									// Confirm default ON, then return and explicitly opt out.
									// Cancelling must persist neither selection.
									if _, err := io.WriteString(writer, "\r"); err != nil {
										return err
									}
									return waitForIssue4377TTY(reader, []string{"Review and Confirm", "Receipt-Driven Development", "RDD ON"}, func() error {
										if _, err := io.WriteString(writer, "\x1b[B\r"); err != nil {
											return err
										}
										return waitForIssue4377TTY(reader, []string{"Receipt-Driven Development", "Enable RDD"}, func() error {
											if _, err := io.WriteString(writer, "\x1b[B\r"); err != nil {
												return err
											}
											return waitForIssue4377TTY(reader, []string{"Review and Confirm", "Receipt-Driven Development", "RDD OFF"}, func() error {
												_, err := io.WriteString(writer, "q")
												return err
											})
										})
									})
								})
							})
						}, func(screen string) {
							communityToolCursorRows = strings.Count(screen, "View repo:") * 2
						})
					})
				})
			}, func(screen string) {
				agentCheckboxRows = strings.Count(screen, "[x]") + strings.Count(screen, "[ ]")
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
