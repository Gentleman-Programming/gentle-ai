package main

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// issue4377Journeys drives the customizable installer through its RDD choice
// and final confirmation under a real PTY. The sandbox HOME is isolated, and
// the exchange quits before installation, proving a cancelled choice cannot
// change the user's global configuration.
func issue4377Journeys() []Journey {
	return []Journey{{
		ID:     "j127-customizable-install-rdd-choice",
		Review: reviewUntouched,
		Title:  "#4377: customizable installation explains, revises, and summarizes the deferred RDD choice",
		Source: "#4377 requires an opt-in global RDD choice before final installation confirmation without mutating a cancelled installation",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "fixture: update-check cooldown in the sandbox HOME", Fixture: issue3766UpdateCooldownFixture},
			{Name: "fixture: detected Claude configuration", Fixture: func(sandbox *Sandbox) error {
				return sandbox.write(filepath.Join(sandbox.Home, ".claude", "settings.json"), "{}\n")
			}},
			{Name: "customizable installer presents RDD before review and permits revision", Composite: func(run *journeyRun) error {
				observation, err := run.runTTY(nil, false, issue4377TTYExchange)
				if err != nil {
					return err
				}
				if observation.ExitCode != 0 {
					return fmt.Errorf("customizable installer TUI exited %d: %s", observation.ExitCode, strings.TrimSpace(observation.Stderr))
				}
				return nil
			}},
		},
	}}
}

func issue4377TTYExchange(reader *bufio.Reader, writer io.WriteCloser) error {
	return waitForIssue4377TTY(reader, []string{"Start installation", "q: quit"}, func() error {
		if _, err := io.WriteString(writer, "\r"); err != nil {
			return err
		}
		return waitForIssue4377TTY(reader, []string{"Detected Configs", "Continue"}, func() error {
			if _, err := io.WriteString(writer, "\r"); err != nil {
				return err
			}
			return waitForIssue4377TTY(reader, []string{"[x] claude-code", "Continue"}, func() error {
				// Claude Code is selected from the sandbox configuration. There are
				// sixteen agents, followed by Continue and Back.
				if _, err := io.WriteString(writer, strings.Repeat("\x1b[B", 16)+"\r"); err != nil {
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
						return waitForIssue4377TTY(reader, []string{"Community Tools/Plugins", "Continue"}, func() error {
							if _, err := io.WriteString(writer, strings.Repeat("\x1b[B", 2)+"\r"); err != nil {
								return err
							}
							return waitForIssue4377TTY(reader, []string{"Install Plan", "Continue"}, func() error {
								if _, err := io.WriteString(writer, "\r"); err != nil {
									return err
								}
								return waitForIssue4377TTY(reader, []string{
									"Receipt-Driven Development",
									"Would you like to enable RDD?",
									"Disable RDD",
								}, func() error {
									// The fresh install default is RDD OFF. Choose Enable RDD, then
									// return from final confirmation and revise to OFF.
									if _, err := io.WriteString(writer, "\x1b[A\r"); err != nil {
										return err
									}
									return waitForIssue4377TTY(reader, []string{"Review and Confirm", "Receipt-Driven Development  RDD ON"}, func() error {
										if _, err := io.WriteString(writer, "\x1b[B\r"); err != nil {
											return err
										}
										return waitForIssue4377TTY(reader, []string{"Receipt-Driven Development", "Enable RDD"}, func() error {
											if _, err := io.WriteString(writer, "\x1b[B\r"); err != nil {
												return err
											}
											return waitForIssue4377TTY(reader, []string{"Review and Confirm", "Receipt-Driven Development  RDD OFF"}, func() error {
												_, err := io.WriteString(writer, "q")
												return err
											})
										})
									})
								})
							})
						})
					})
				})
			})
		})
	})
}

func waitForIssue4377TTY(reader *bufio.Reader, required []string, next func() error) error {
	var screen strings.Builder
	for {
		byteRead, err := reader.ReadByte()
		if err != nil {
			return fmt.Errorf("read TUI before %q: %w; output: %q", strings.Join(required, ", "), err, screen.String())
		}
		screen.WriteByte(byteRead)
		matched := true
		for _, text := range required {
			if !strings.Contains(screen.String(), text) {
				matched = false
				break
			}
		}
		if matched {
			return next()
		}
	}
}
