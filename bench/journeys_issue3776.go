package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// issue3776Journeys proves the benchmark can drive the product's native TUI.
// It keeps the review switch untouched because the Welcome screen is unrelated
// to receipt-driven development.
func issue3776Journeys() []Journey {
	return []Journey{{
		ID:     "j120-welcome-tui-runs-under-a-real-tty",
		Review: reviewUntouched,
		Title:  "#3776: Welcome TUI renders its stable menu under a real TTY and q exits",
		Source: "#3776: PTY-backed benchmark smoke journey",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{
				Name: "Welcome TUI renders its menu and quits",
				Composite: func(run *journeyRun) error {
					observation, err := run.runTTY(nil, false, welcomeTTYExchange)
					if err != nil {
						return err
					}
					if observation.ExitCode != 0 {
						return fmt.Errorf("Welcome TUI exited %d: %s", observation.ExitCode, strings.TrimSpace(observation.Stderr))
					}
					return nil
				},
			},
		},
	}}
}

func welcomeTTYExchange(reader *bufio.Reader, writer io.WriteCloser) error {
	var screen strings.Builder
	for {
		byteRead, err := reader.ReadByte()
		if err != nil {
			return fmt.Errorf("read Welcome TUI before its stable menu: %w; output: %q", err, screen.String())
		}
		screen.WriteByte(byteRead)
		if strings.Contains(screen.String(), "Start installation") && strings.Contains(screen.String(), "q: quit") {
			_, err := io.WriteString(writer, "q")
			return err
		}
	}
}
