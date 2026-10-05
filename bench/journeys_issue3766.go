package main

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
)

// issue3766UpdateCooldownFixture seeds the sandbox HOME's state.json with a
// current last_update_check, mirroring issue3561DanglingAncestorFixture: a
// fresh sandbox HOME has no update cooldown, so when upstream is newer the
// launch update check pops an "Update Available" modal that covers the menu
// the TTY exchange waits for (#3971's second manifestation). No state.json
// exists at this point, so only the cooldown is seeded and the journey keeps
// running against a not-installed state.
func issue3766UpdateCooldownFixture(sandbox *Sandbox) error {
	statePath := filepath.Join(sandbox.Home, ".gentle-ai", "state.json")
	state := fmt.Sprintf(`{"last_update_check":%q}`, time.Now().UTC().Format(time.RFC3339Nano))
	return sandbox.write(statePath, state)
}

// issue3766Journeys drives #3766's switch under a real PTY. The sandbox supplies
// an isolated HOME, so this proof cannot read or change a user's global config.
func issue3766Journeys() []Journey {
	return []Journey{{
		ID:     "j121-rdd-tui-controls-global-mode",
		Review: reviewUntouched,
		Title:  "#3766: TUI changes the global review mode inside its isolated HOME",
		Source: "#3766: the switch screen must expose resolved state and never touch a real user config",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "fixture: update-check cooldown in the sandbox HOME", Fixture: issue3766UpdateCooldownFixture},
			{Name: "Receipt-Driven Development toggles globally in the TUI", Composite: func(run *journeyRun) error {
				observation, err := run.runTTY(nil, false, reviewModeTTYExchange)
				if err != nil {
					return err
				}
				if observation.ExitCode != 0 {
					return fmt.Errorf("review-mode TUI exited %d: %s", observation.ExitCode, strings.TrimSpace(observation.Stderr))
				}
				return nil
			}},
		},
	}}
}

func reviewModeTTYExchange(reader *bufio.Reader, writer io.WriteCloser) error {
	return waitForReviewModeTTY(reader, "Start installation", "q: quit", "", func() error {
		time.Sleep(100 * time.Millisecond)
		if _, err := io.WriteString(writer, strings.Repeat("\x1b[A", 4)+"\r"); err != nil {
			return err
		}
		return waitForReviewModeTTY(reader, "RDD runs a bounded review before delivery and records review evidence.", "Delivery remains governed by repository policy", "RDD is currently DISABLED globally.", func() error {
			if _, err := io.WriteString(writer, "\r"); err != nil {
				return err
			}
			return waitForReviewModeTTY(reader, "Start installation", "q: quit", "", func() error {
				time.Sleep(100 * time.Millisecond)
				if _, err := io.WriteString(writer, strings.Repeat("\x1b[A", 4)+"\r"); err != nil {
					return err
				}
				return waitForReviewModeTTY(reader, "RDD is currently ENABLED globally.", "Disable globally", "", func() error {
					if _, err := io.WriteString(writer, "\r"); err != nil {
						return err
					}
					return waitForReviewModeTTY(reader, "Start installation", "q: quit", "", func() error {
						_, err := io.WriteString(writer, "q")
						return err
					})
				})
			})
		})
	})
}

func waitForReviewModeTTY(reader *bufio.Reader, required, also, third string, next func() error) error {
	var screen strings.Builder
	for {
		byteRead, err := reader.ReadByte()
		if err != nil {
			return fmt.Errorf("read TUI before %q: %w; output: %q", required, err, screen.String())
		}
		screen.WriteByte(byteRead)
		if strings.Contains(screen.String(), required) && strings.Contains(screen.String(), also) && strings.Contains(screen.String(), third) {
			return next()
		}
	}
}
