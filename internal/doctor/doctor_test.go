package doctor

import (
	"context"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/verify"
)

func TestRunChecksAllExecute(t *testing.T) {
	checks := []Check{
		{
			ID: "check-a", Category: "deps", Description: "first check",
			Run: func(context.Context) CheckResult {
				return CheckResult{ID: "check-a", Category: "deps", Status: verify.CheckStatusPassed, Message: "ok"}
			},
		},
		{
			ID: "check-b", Category: "env", Description: "second check",
			Run: func(context.Context) CheckResult {
				return CheckResult{ID: "check-b", Category: "env", Status: verify.CheckStatusFailed, Message: "missing"}
			},
		},
	}

	report := RunChecks(context.Background(), checks, Options{})

	if len(report.Checks) != 2 {
		t.Fatalf("Checks = %d, want 2", len(report.Checks))
	}
	if report.Passed != 1 {
		t.Errorf("Passed = %d, want 1", report.Passed)
	}
	if report.Failed != 1 {
		t.Errorf("Failed = %d, want 1", report.Failed)
	}
	if report.Healthy {
		t.Errorf("Healthy = true, want false when failures present")
	}
}

func TestRunChecksFiltersByCategory(t *testing.T) {
	checks := []Check{
		{ID: "dep-1", Category: "deps", Run: func(context.Context) CheckResult {
			return CheckResult{ID: "dep-1", Category: "deps", Status: verify.CheckStatusPassed}
		}},
		{ID: "env-1", Category: "env", Run: func(context.Context) CheckResult {
			return CheckResult{ID: "env-1", Category: "env", Status: verify.CheckStatusPassed}
		}},
		{ID: "dep-2", Category: "deps", Run: func(context.Context) CheckResult {
			return CheckResult{ID: "dep-2", Category: "deps", Status: verify.CheckStatusWarning}
		}},
	}

	report := RunChecks(context.Background(), checks, Options{Category: "deps"})

	if len(report.Checks) != 2 {
		t.Fatalf("Checks = %d, want 2 (only deps)", len(report.Checks))
	}
	for _, r := range report.Checks {
		if r.Category != "deps" {
			t.Errorf("unexpected category %q in filtered results", r.Category)
		}
	}
}

func TestRunChecksRespectsTimeout(t *testing.T) {
	checks := []Check{
		{
			ID: "slow", Category: "deps",
			Run: func(ctx context.Context) CheckResult {
				select {
				case <-time.After(5 * time.Second):
					return CheckResult{ID: "slow", Status: verify.CheckStatusPassed}
				case <-ctx.Done():
					return CheckResult{ID: "slow", Status: verify.CheckStatusSkipped, Message: "cancelled"}
				}
			},
		},
	}

	report := RunChecks(context.Background(), checks, Options{Timeout: 100 * time.Millisecond})

	if len(report.Checks) != 1 {
		t.Fatalf("Checks = %d, want 1", len(report.Checks))
	}
	if report.Checks[0].Status != verify.CheckStatusSkipped {
		t.Errorf("Status = %q, want skipped due to timeout", report.Checks[0].Status)
	}
}

func TestRunChecksConcurrentExecution(t *testing.T) {
	const n = 10
	checks := make([]Check, n)
	for i := range checks {
		id := string(rune('a' + i))
		checks[i] = Check{
			ID: "check-" + id, Category: "deps",
			Run: func(ctx context.Context) CheckResult {
				time.Sleep(50 * time.Millisecond)
				return CheckResult{ID: "check-" + id, Status: verify.CheckStatusPassed}
			},
		}
	}

	start := time.Now()
	report := RunChecks(context.Background(), checks, Options{})
	elapsed := time.Since(start)

	if len(report.Checks) != n {
		t.Fatalf("Checks = %d, want %d", len(report.Checks), n)
	}

	// Sequential: 10 * 50ms = 500ms. Concurrent should be well under that.
	if elapsed > 400*time.Millisecond {
		t.Errorf("checks appear sequential: took %v (expected < 400ms)", elapsed)
	}
}

func TestBuildReportSummaryCounts(t *testing.T) {
	results := []CheckResult{
		{Status: verify.CheckStatusPassed},
		{Status: verify.CheckStatusPassed},
		{Status: verify.CheckStatusFailed},
		{Status: verify.CheckStatusWarning},
		{Status: verify.CheckStatusSkipped},
	}

	report := BuildReport(results, 42*time.Millisecond)

	if report.Passed != 2 {
		t.Errorf("Passed = %d, want 2", report.Passed)
	}
	if report.Failed != 1 {
		t.Errorf("Failed = %d, want 1", report.Failed)
	}
	if report.Warnings != 1 {
		t.Errorf("Warnings = %d, want 1", report.Warnings)
	}
	if report.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", report.Skipped)
	}
	if report.Healthy {
		t.Errorf("Healthy = true, want false")
	}
}

func TestBuildReportHealthyWhenNoFailures(t *testing.T) {
	results := []CheckResult{
		{Status: verify.CheckStatusPassed},
		{Status: verify.CheckStatusWarning},
	}

	report := BuildReport(results, 0)

	if !report.Healthy {
		t.Errorf("Healthy = false, want true (warnings are non-blocking)")
	}
}

func TestBuildReportEmptyIsHealthy(t *testing.T) {
	report := BuildReport(nil, 0)

	if !report.Healthy {
		t.Errorf("Healthy = false, want true for empty check list")
	}
}
