package doctor

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/cli"
	"github.com/gentleman-programming/gentle-ai/pkg/doctor/fixer"
	"github.com/mattn/go-isatty"
)

// RunDoctor executes the doctor command with the given flags and checkers.
// Returns an error if any check fails (exit code 1), nil otherwise (exit code 0).
// Does not call os.Exit - the caller handles exit codes.
func RunDoctor(ctx context.Context, flags DoctorFlags, stdout io.Writer, checkers []Checker) error {
	startTime := time.Now()

	// Run all checkers
	var allResults []CheckResult
	for _, c := range checkers {
		results := c.Run(ctx)
		allResults = append(allResults, results...)
	}

	// Build summary
	summary := buildSummary(allResults, time.Since(startTime))

	// Create fix registry if FixMode is enabled
	var fixRegistry *fixer.FixerRegistry
	if flags.FixMode {
		fixRegistry = fixer.NewFixerRegistry()
	}

	// Create report
	report := DoctorReport{
		GeneratedAt: time.Now(),
		Version:     cli.AppVersion,
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
		Results:     allResults,
		Summary:     summary,
		FixRegistry: fixRegistry,
	}

	// Select and create renderer
	renderer := createRenderer(flags, stdout)

	// Render report
	if err := renderer.Render(report); err != nil {
		return fmt.Errorf("render report: %w", err)
	}

	// Return error if any FAIL status (for exit code 1)
	if summary.Fail > 0 {
		return fmt.Errorf("doctor: %d failing check(s)", summary.Fail)
	}

	return nil
}

// createRenderer creates the appropriate renderer based on flags.
// TUI renderer auto-falls back to text if not in a TTY.
func createRenderer(flags DoctorFlags, stdout io.Writer) Renderer {
	verbose := flags.Verbose
	fixMode := flags.FixMode

	if flags.JSONOutput {
		return NewJSONRenderer(stdout)
	}

	// Check if we're in a TTY for TUI
	if f, ok := stdout.(*os.File); ok {
		if isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd()) {
			return NewTUIRenderer(RenderOptions{
				Format:      "tui",
				Verbose:     verbose,
				Color:       true,
				ShowPassed:  true,
				ShowSkipped: true,
				FixMode:     fixMode,
				Writer:      stdout,
			})
		}
	}

	// Fallback to text renderer
	return NewTextRenderer(stdout, RenderOptions{
		Format:      "text",
		Verbose:     verbose,
		Color:       true,
		ShowPassed:  true,
		ShowSkipped: true,
		FixMode:     fixMode,
		Writer:      stdout,
	})
}

// parseCategories converts string category flags to Category enum.
// Accepts: "hw"/"hardware", "sw"/"software", "cfg"/"config" (case-insensitive).
// Default: all categories if empty.
func parseCategories(input []string) []Category {
	if len(input) == 0 {
		return []Category{CategoryHardware, CategorySoftware, CategoryConfig}
	}

	catMap := make(map[Category]bool)
	for _, c := range input {
		lower := strings.ToLower(strings.TrimSpace(c))
		switch lower {
		case "hw", "hardware":
			catMap[CategoryHardware] = true
		case "sw", "software":
			catMap[CategorySoftware] = true
		case "cfg", "config":
			catMap[CategoryConfig] = true
		default:
			// Unknown category - ignore
		}
	}

	if len(catMap) == 0 {
		return []Category{CategoryHardware, CategorySoftware, CategoryConfig}
	}

	var result []Category
	for cat := range catMap {
		result = append(result, cat)
	}
	return result
}

// buildSummary aggregates check results into a Summary.
func buildSummary(results []CheckResult, duration time.Duration) Summary {
	var s Summary
	s.Duration = duration
	for _, r := range results {
		s.Total++
		switch r.Status {
		case StatusPass:
			s.Pass++
		case StatusWarn:
			s.Warn++
		case StatusFail:
			s.Fail++
		case StatusInfo:
			s.Info++
		case StatusSkip:
			s.Skip++
		}
	}
	return s
}