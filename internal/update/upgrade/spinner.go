package upgrade

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type CLISpinner struct {
	w       io.Writer
	message string
	stop    chan struct{}
	done    sync.WaitGroup
}

func NewSpinner(w io.Writer, message string) *CLISpinner {
	s := &CLISpinner{
		w:       w,
		message: message,
		stop:    make(chan struct{}),
	}
	s.done.Add(1)
	go s.run()
	return s
}

func (s *CLISpinner) run() {
	defer s.done.Done()
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	frame := 0
	line := fmt.Sprintf("  %s %s...", spinnerFrames[frame], s.message)
	fmt.Fprint(s.w, line)

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			frame = (frame + 1) % len(spinnerFrames)
			line = fmt.Sprintf("  %s %s...", spinnerFrames[frame], s.message)
			fmt.Fprintf(s.w, "\r%s", line)
		}
	}
}

func (s *CLISpinner) Finish(success bool) {
	close(s.stop)
	s.done.Wait()

	icon := "✓"
	if !success {
		icon = "✗"
	}
	clearLine := "\r" + strings.Repeat(" ", len(s.message)+20) + "\r"
	fmt.Fprintf(s.w, "%s  %s %s\n", clearLine, icon, s.message)
}

// FinishSkipped stops the spinner and renders a skip marker (--) instead of
// the failure marker (✗). Use this for intentional skips such as manual-update
// fallbacks on Windows — these are NOT failures and must not be displayed as such.
func (s *CLISpinner) FinishSkipped() {
	close(s.stop)
	s.done.Wait()

	clearLine := "\r" + strings.Repeat(" ", len(s.message)+20) + "\r"
	fmt.Fprintf(s.w, "%s  -- %s\n", clearLine, s.message)
}
