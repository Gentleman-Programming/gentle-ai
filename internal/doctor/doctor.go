package doctor

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/verify"
)

// Check represents a single doctor diagnostic. The struct mirrors verify.Check
// but returns a richer CheckResult instead of a plain error, since doctor needs
// to surface details (paths, fix commands) beyond pass/fail.
type Check struct {
	ID          string
	Category    string // "deps", "platform", "agents", "env"
	Description string
	Run         func(context.Context) CheckResult
}

// CheckResult holds the outcome of a single diagnostic.
type CheckResult struct {
	ID          string              `json:"id"`
	Category    string              `json:"category"`
	Description string              `json:"description"`
	Status      verify.CheckStatus  `json:"status"`
	Message     string              `json:"message"`
	Details     []string            `json:"details,omitempty"`
}

// Report aggregates results from all checks in a single doctor run.
type Report struct {
	Checks   []CheckResult
	Passed   int
	Failed   int
	Skipped  int
	Warnings int
	Healthy  bool
	Duration time.Duration
}

// Options configures a doctor run.
type Options struct {
	Category string        // filter to a single category (empty = all)
	JSON     bool          // machine-readable output
	Verbose  bool          // extended detail
	Timeout  time.Duration // global timeout (default 10s)
}

// DefaultTimeout is the global timeout for all doctor checks.
const DefaultTimeout = 10 * time.Second

// RunChecks executes all provided checks concurrently, bounded by a semaphore
// sized to runtime.NumCPU(). The context carries the global timeout.
//
// Doctor checks are read-only by contract: no check may write files, modify
// configs, or start services.
func RunChecks(ctx context.Context, checks []Check, opts Options) Report {
	start := time.Now()

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Filter by category if requested.
	var filtered []Check
	for _, c := range checks {
		if opts.Category == "" || c.Category == opts.Category {
			filtered = append(filtered, c)
		}
	}

	results := make([]CheckResult, len(filtered))

	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())

	for i, check := range filtered {
		wg.Add(1)
		go func(idx int, c Check) {
			defer wg.Done()

			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			if ctx.Err() != nil {
				results[idx] = CheckResult{
					ID:       c.ID,
					Category: c.Category,
					Status:   verify.CheckStatusSkipped,
					Message:  "skipped: timeout exceeded",
				}
				return
			}

			results[idx] = c.Run(ctx)
		}(i, check)
	}

	wg.Wait()

	return BuildReport(results, time.Since(start))
}

// BuildReport aggregates check results into a Report with summary counts.
func BuildReport(results []CheckResult, duration time.Duration) Report {
	report := Report{
		Checks:   results,
		Duration: duration,
	}

	for _, r := range results {
		switch r.Status {
		case verify.CheckStatusPassed:
			report.Passed++
		case verify.CheckStatusFailed:
			report.Failed++
		case verify.CheckStatusSkipped:
			report.Skipped++
		case verify.CheckStatusWarning:
			report.Warnings++
		}
	}

	report.Healthy = report.Failed == 0
	return report
}
