package verify

import (
	"context"
	"errors"
	"testing"
)

func TestCheckPolicySurvivesRunChecks(t *testing.T) {
	checks := []Check{
		{
			ID:          "default-policy",
			Description: "default rollback policy is preserved",
			NoRollback:  false,
			Run: func(context.Context) error {
				return nil
			},
		},
		{
			ID:          "no-rollback-policy",
			Description: "no-rollback policy is preserved",
			NoRollback:  true,
			Run: func(context.Context) error {
				return nil
			},
		},
		{
			ID:          "skipped-check-policy",
			Description: "skipped check preserves no-rollback policy",
			NoRollback:  true,
		},
	}

	results := RunChecks(context.Background(), checks)
	if len(results) != len(checks) {
		t.Fatalf("RunChecks returned %d results, want %d", len(results), len(checks))
	}

	for i, check := range checks {
		if results[i].NoRollback != check.NoRollback {
			t.Errorf("results[%d].NoRollback = %v, want %v for check %q", i, results[i].NoRollback, check.NoRollback, check.ID)
		}
	}
}

func TestBuildReportRollbackRequired(t *testing.T) {
	errBoom := errors.New("check failed")

	tests := []struct {
		name                 string
		checks               []Check
		wantReady            bool
		wantRollbackRequired bool
		wantFailed           int
	}{
		{
			name: "fully passing run yields rollback required false",
			checks: []Check{
				{
					ID:         "pass-1",
					NoRollback: false,
					Run:        func(context.Context) error { return nil },
				},
				{
					ID:         "pass-2",
					NoRollback: true,
					Run:        func(context.Context) error { return nil },
				},
			},
			wantReady:            true,
			wantRollbackRequired: false,
			wantFailed:           0,
		},
		{
			name: "plain hard check failure preserves rollback required true by zero value",
			checks: []Check{
				{
					ID: "hard-default",
					// NoRollback zero value is false
					Run: func(context.Context) error { return errBoom },
				},
			},
			wantReady:            false,
			wantRollbackRequired: true,
			wantFailed:           1,
		},
		{
			name: "hard check failure with no-rollback true yields rollback required false and ready false",
			checks: []Check{
				{
					ID:         "hard-no-rollback",
					NoRollback: true,
					Run:        func(context.Context) error { return errBoom },
				},
			},
			wantReady:            false,
			wantRollbackRequired: false,
			wantFailed:           1,
		},
		{
			name: "mixed case one no-rollback failure plus ordinary hard failure yields rollback required true",
			checks: []Check{
				{
					ID:         "incompatible-core",
					NoRollback: true,
					Run:        func(context.Context) error { return errBoom },
				},
				{
					ID:         "missing-managed-file",
					NoRollback: false,
					Run:        func(context.Context) error { return errBoom },
				},
			},
			wantReady:            false,
			wantRollbackRequired: true,
			wantFailed:           2,
		},
		{
			name: "soft check failure is a warning and never requires rollback even with no-rollback false",
			checks: []Check{
				{
					ID:         "soft-fail-default",
					Soft:       true,
					NoRollback: false,
					Run:        func(context.Context) error { return errBoom },
				},
				{
					ID:         "soft-fail-no-rollback",
					Soft:       true,
					NoRollback: true,
					Run:        func(context.Context) error { return errBoom },
				},
			},
			wantReady:            true,
			wantRollbackRequired: false,
			wantFailed:           0,
		},
		{
			name: "multiple failures all with no-rollback true yield rollback required false",
			checks: []Check{
				{
					ID:         "fail-nr-1",
					NoRollback: true,
					Run:        func(context.Context) error { return errBoom },
				},
				{
					ID:         "fail-nr-2",
					NoRollback: true,
					Run:        func(context.Context) error { return errBoom },
				},
			},
			wantReady:            false,
			wantRollbackRequired: false,
			wantFailed:           2,
		},
		{
			name: "skipped check never requires rollback",
			checks: []Check{
				{
					ID:         "skip-check",
					NoRollback: false,
				},
			},
			wantReady:            true,
			wantRollbackRequired: false,
			wantFailed:           0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := RunChecks(context.Background(), tt.checks)
			report := BuildReport(results)

			if report.Ready != tt.wantReady {
				t.Errorf("Ready = %v, want %v", report.Ready, tt.wantReady)
			}
			if report.RollbackRequired != tt.wantRollbackRequired {
				t.Errorf("RollbackRequired = %v, want %v", report.RollbackRequired, tt.wantRollbackRequired)
			}
			if report.Failed != tt.wantFailed {
				t.Errorf("Failed = %d, want %d", report.Failed, tt.wantFailed)
			}
		})
	}
}

func TestBuildReportDirectResultsRollbackRequired(t *testing.T) {
	tests := []struct {
		name                 string
		results              []CheckResult
		wantRollbackRequired bool
	}{
		{
			name:                 "empty results",
			results:              nil,
			wantRollbackRequired: false,
		},
		{
			name: "single failed result without no-rollback",
			results: []CheckResult{
				{ID: "c1", Status: CheckStatusFailed, NoRollback: false},
			},
			wantRollbackRequired: true,
		},
		{
			name: "single failed result with no-rollback",
			results: []CheckResult{
				{ID: "c1", Status: CheckStatusFailed, NoRollback: true},
			},
			wantRollbackRequired: false,
		},
		{
			name: "mixed failed results",
			results: []CheckResult{
				{ID: "c1", Status: CheckStatusFailed, NoRollback: true},
				{ID: "c2", Status: CheckStatusFailed, NoRollback: false},
			},
			wantRollbackRequired: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := BuildReport(tt.results)
			if report.RollbackRequired != tt.wantRollbackRequired {
				t.Errorf("RollbackRequired = %v, want %v", report.RollbackRequired, tt.wantRollbackRequired)
			}
		})
	}
}
