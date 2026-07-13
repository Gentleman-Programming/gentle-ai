package doctor

import (
	"testing"
	"time"
)

func TestStatusConstants(t *testing.T) {
	if StatusPass != "PASS" {
		t.Errorf("StatusPass = %q, want %q", StatusPass, "PASS")
	}
	if StatusWarn != "WARN" {
		t.Errorf("StatusWarn = %q, want %q", StatusWarn, "WARN")
	}
	if StatusFail != "FAIL" {
		t.Errorf("StatusFail = %q, want %q", StatusFail, "FAIL")
	}
	if StatusInfo != "INFO" {
		t.Errorf("StatusInfo = %q, want %q", StatusInfo, "INFO")
	}
	if StatusSkip != "SKIP" {
		t.Errorf("StatusSkip = %q, want %q", StatusSkip, "SKIP")
	}
}

func TestCategoryConstants(t *testing.T) {
	if CategoryHardware != "hardware" {
		t.Errorf("CategoryHardware = %q, want %q", CategoryHardware, "hardware")
	}
	if CategorySoftware != "software" {
		t.Errorf("CategorySoftware = %q, want %q", CategorySoftware, "software")
	}
	if CategoryConfig != "config" {
		t.Errorf("CategoryConfig = %q, want %q", CategoryConfig, "config")
	}
}

func TestCheckResult_Defaults(t *testing.T) {
	result := CheckResult{
		Name:     "test:check",
		Category: CategoryHardware,
		Status:   StatusPass,
		Summary:  "Test passed",
	}

	if result.Name != "test:check" {
		t.Errorf("Name = %q, want %q", result.Name, "test:check")
	}
	if result.Status != StatusPass {
		t.Errorf("Status = %q, want %q", result.Status, StatusPass)
	}
	if result.Remediation != nil {
		t.Error("Remediation should be nil by default")
	}
	if result.Metadata != nil {
		t.Error("Metadata should be nil by default")
	}
}

func TestCheckResult_WithRemediation(t *testing.T) {
	result := CheckResult{
		Name:     "test:check",
		Category: CategorySoftware,
		Status:   StatusFail,
		Summary:  "Test failed",
		Remediation: &Remediation{
			Description: "Install missing dependency",
			Commands: map[string]string{
				"darwin":  "brew install foo",
				"linux":   "apt-get install foo",
				"windows": "winget install foo",
			},
			ManualSteps: []string{"Run the command manually", "Restart the service"},
			Links:       []string{"https://example.com/docs"},
		},
	}

	if result.Remediation == nil {
		t.Fatal("Remediation should not be nil")
	}
	if result.Remediation.Description != "Install missing dependency" {
		t.Errorf("Description = %q, want %q", result.Remediation.Description, "Install missing dependency")
	}
	if len(result.Remediation.Commands) != 3 {
		t.Errorf("Commands = %d, want 3", len(result.Remediation.Commands))
	}
	if len(result.Remediation.ManualSteps) != 2 {
		t.Errorf("ManualSteps = %d, want 2", len(result.Remediation.ManualSteps))
	}
	if len(result.Remediation.Links) != 1 {
		t.Errorf("Links = %d, want 1", len(result.Remediation.Links))
	}
}

func TestDoctorReport_Summary(t *testing.T) {
	report := DoctorReport{
		GeneratedAt: time.Now(),
		Version:     "1.0.0",
		GOOS:        "darwin",
		GOARCH:      "arm64",
		Results: []CheckResult{
			{Name: "a", Status: StatusPass, Category: CategoryHardware},
			{Name: "b", Status: StatusPass, Category: CategoryHardware},
			{Name: "c", Status: StatusWarn, Category: CategorySoftware},
			{Name: "d", Status: StatusFail, Category: CategoryConfig},
			{Name: "e", Status: StatusInfo, Category: CategoryConfig},
			{Name: "f", Status: StatusSkip, Category: CategoryConfig},
		},
		Summary: Summary{
			Total:    6,
			Pass:     2,
			Warn:     1,
			Fail:     1,
			Info:     1,
			Skip:     1,
			Duration: 100 * time.Millisecond,
		},
	}

	if report.Summary.Total != 6 {
		t.Errorf("Total = %d, want 6", report.Summary.Total)
	}
	if report.Summary.Pass != 2 {
		t.Errorf("Pass = %d, want 2", report.Summary.Pass)
	}
	if report.Summary.Fail != 1 {
		t.Errorf("Fail = %d, want 1", report.Summary.Fail)
	}
}

func TestParseCategories(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []Category
	}{
		{
			name:     "empty defaults to all",
			input:    []string{},
			expected: []Category{CategoryHardware, CategorySoftware, CategoryConfig},
		},
		{
			name:     "hardware short",
			input:    []string{"hw"},
			expected: []Category{CategoryHardware},
		},
		{
			name:     "hardware long",
			input:    []string{"hardware"},
			expected: []Category{CategoryHardware},
		},
		{
			name:     "software short",
			input:    []string{"sw"},
			expected: []Category{CategorySoftware},
		},
		{
			name:     "config short",
			input:    []string{"cfg"},
			expected: []Category{CategoryConfig},
		},
		{
			name:     "multiple categories",
			input:    []string{"hw", "sw"},
			expected: []Category{CategoryHardware, CategorySoftware},
		},
		{
			name:     "case insensitive",
			input:    []string{"HW", "Sw", "Cfg"},
			expected: []Category{CategoryHardware, CategorySoftware, CategoryConfig},
		},
		{
			name:     "with spaces",
			input:    []string{" hw ", " sw "},
			expected: []Category{CategoryHardware, CategorySoftware},
		},
		{
			name:     "unknown ignored",
			input:    []string{"unknown", "hw"},
			expected: []Category{CategoryHardware},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCategories(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("parseCategories(%v) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			// Check set equality (order doesn't matter due to map iteration)
			found := make(map[Category]bool)
			for _, r := range result {
				found[r] = true
			}
			for _, e := range tt.expected {
				if !found[e] {
					t.Errorf("parseCategories(%v) missing expected category %v, got %v", tt.input, e, result)
				}
			}
		})
	}
}

func TestBuildSummary(t *testing.T) {
	results := []CheckResult{
		{Status: StatusPass},
		{Status: StatusPass},
		{Status: StatusWarn},
		{Status: StatusFail},
		{Status: StatusInfo},
		{Status: StatusSkip},
	}
	summary := buildSummary(results, 100*time.Millisecond)

	if summary.Total != 6 {
		t.Errorf("Total = %d, want 6", summary.Total)
	}
	if summary.Pass != 2 {
		t.Errorf("Pass = %d, want 2", summary.Pass)
	}
	if summary.Warn != 1 {
		t.Errorf("Warn = %d, want 1", summary.Warn)
	}
	if summary.Fail != 1 {
		t.Errorf("Fail = %d, want 1", summary.Fail)
	}
	if summary.Info != 1 {
		t.Errorf("Info = %d, want 1", summary.Info)
	}
	if summary.Skip != 1 {
		t.Errorf("Skip = %d, want 1", summary.Skip)
	}
	if summary.Duration != 100*time.Millisecond {
		t.Errorf("Duration = %v, want 100ms", summary.Duration)
	}
}