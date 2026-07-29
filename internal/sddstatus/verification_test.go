package sddstatus

import (
	"strings"
	"testing"
)

func TestParseVerifyResultFailsClosedAndRequiresCurrentExecutionEvidence(t *testing.T) {
	valid := testVerifyEnvelope("pass", 0, 0, "2/2", "3/3", 0, 0)
	tests := []struct {
		name       string
		report     string
		expected   SpecCounts
		wantPass   bool
		wantStale  bool
		wantReason string
	}{
		{name: "valid measured result", report: valid, expected: SpecCounts{Requirements: 2, Scenarios: 3}, wantPass: true},
		{name: "prose cannot pass", report: "Verdict: PASS\nAll checks passed.", expected: SpecCounts{Requirements: 2, Scenarios: 3}, wantReason: "missing valid"},
		{name: "unknown field fails closed", report: strings.Replace(valid, "verdict: pass", "verdict: pass\nextra: value", 1), expected: SpecCounts{Requirements: 2, Scenarios: 3}, wantReason: "unknown"},
		{name: "missing build command fails closed", report: strings.Replace(valid, "build_command: go test ./cmd/gentle-ai\n", "", 1), expected: SpecCounts{Requirements: 2, Scenarios: 3}, wantReason: "missing build_command"},
		{name: "failed tests cannot pass", report: testVerifyEnvelope("pass", 0, 0, "2/2", "3/3", 1, 0), expected: SpecCounts{Requirements: 2, Scenarios: 3}, wantReason: "test_exit_code"},
		{name: "failed build cannot pass", report: testVerifyEnvelope("pass", 0, 0, "2/2", "3/3", 0, 1), expected: SpecCounts{Requirements: 2, Scenarios: 3}, wantReason: "build_exit_code"},
		{name: "complete requirement total mismatch is stale", report: valid, expected: SpecCounts{Requirements: 3, Scenarios: 3}, wantStale: true, wantReason: "actual requirement count"},
		{name: "complete scenario total mismatch is stale", report: valid, expected: SpecCounts{Requirements: 2, Scenarios: 4}, wantStale: true, wantReason: "actual scenario count"},
		{name: "incomplete requirements are not stale", report: testVerifyEnvelope("pass", 0, 0, "1/2", "3/3", 0, 0), expected: SpecCounts{Requirements: 3, Scenarios: 3}, wantReason: "actual requirement count"},
		{name: "incomplete scenarios are not stale", report: testVerifyEnvelope("pass", 0, 0, "2/2", "2/3", 0, 0), expected: SpecCounts{Requirements: 2, Scenarios: 4}, wantReason: "actual scenario count"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseVerifyResult(tt.report, tt.expected)
			if got.Passing != tt.wantPass {
				t.Fatalf("Passing = %v, want %v (reason %q)", got.Passing, tt.wantPass, got.Reason)
			}
			if got.Stale != tt.wantStale {
				t.Fatalf("Stale = %v, want %v (reason %q)", got.Stale, tt.wantStale, got.Reason)
			}
			if tt.wantReason != "" && !strings.Contains(got.Reason, tt.wantReason) {
				t.Fatalf("Reason = %q, want containing %q", got.Reason, tt.wantReason)
			}
		})
	}
}

func TestValidateVerifyReportAdmission(t *testing.T) {
	valid := testVerifyEnvelope("pass", 0, 0, "2/2", "3/3", 0, 0)
	authority := strings.TrimSuffix(testVerifyEnvelope("fail", 1, 1, "0/2", "0/3", 125, 125), "```") + strings.Join([]string{
		"authority_only_failure: true", "missing_review_authority: true", "substantive_failure: false", "command_failed: false",
		"observed_authority_revision: sha256:" + strings.Repeat("d", 64), "```\n",
	}, "\n")
	authority = strings.Replace(authority, "test_exit_code: 1", "test_exit_code: 125", 1)
	authority = strings.Replace(authority, "build_exit_code: 1", "build_exit_code: 125", 1)
	authority = strings.ReplaceAll(authority, "sha256:"+strings.Repeat("b", 64), emptyOutputHash)
	authority = strings.ReplaceAll(authority, "sha256:"+strings.Repeat("c", 64), emptyOutputHash)
	tests := []struct {
		name, report, reason string
		valid                bool
	}{
		{"pass", valid, "", true},
		{"warning pass with CRLF and prose", strings.ReplaceAll(strings.Replace(valid, "verdict: pass", "verdict: pass_with_warnings", 1)+"\nDetails", "\n", "\r\n"), "", true},
		{"failed tests", testVerifyEnvelope("fail", 0, 0, "2/2", "3/3", 1, 0), "", true},
		{"failed build", testVerifyEnvelope("fail", 0, 0, "2/2", "3/3", 0, 1), "", true},
		{"blocker", testVerifyEnvelope("fail", 1, 0, "2/2", "3/3", 0, 0), "", true},
		{"critical", testVerifyEnvelope("fail", 0, 1, "2/2", "3/3", 0, 0), "", true},
		{"incomplete requirement", testVerifyEnvelope("fail", 0, 0, "1/2", "3/3", 0, 0), "", true},
		{"incomplete scenario", testVerifyEnvelope("fail", 0, 0, "2/2", "2/3", 0, 0), "", true},
		{"authority-only denial", authority, "", true},
		{"all-green failure", strings.Replace(valid, "verdict: pass", "verdict: fail", 1), "contradictory", false},
		{"passing blocker", strings.Replace(valid, "blockers: 0", "blockers: 1", 1), "contradicts", false},
		{"passing incomplete", strings.Replace(valid, "requirements: 2/2", "requirements: 1/2", 1), "contradicts", false},
		{"count mismatch", valid, "actual requirement count", false},
		{"front matter", "---\nverdict: pass\n---\n" + valid, "front matter", false},
		{"prose first", "Result follows\n" + valid, "first non-empty", false},
		{"unterminated", strings.TrimSuffix(valid, "```"), "unterminated", false},
		{"duplicate", strings.Replace(valid, "verdict: pass", "verdict: pass\nverdict: pass", 1), "duplicate", false},
		{"unknown", strings.Replace(valid, "verdict: pass", "verdict: pass\nextra: value", 1), "unknown", false},
		{"malformed", strings.Replace(valid, "blockers: 0", "blockers", 1), "malformed", false},
		{"missing", strings.Replace(valid, "build_command: go test ./cmd/gentle-ai\n", "", 1), "missing build_command", false},
		{"invalid hash", strings.Replace(valid, "sha256:"+strings.Repeat("b", 64), "sha256:nope", 1), "invalid test_output_hash", false},
		{"placeholder command", strings.Replace(valid, "test_command: go test ./internal/example", "test_command: placeholder", 1), "concrete", false},
		{"partial authority extension", strings.TrimSuffix(valid, "```") + "authority_only_failure: true\n```", "authority-only", false},
		{"invalid authority extension", strings.Replace(authority, "command_failed: false", "command_failed: true", 1), "invalid authority-only", false},
		{"reserved exit without extension", strings.Replace(strings.Replace(valid, "verdict: pass", "verdict: fail", 1), "test_exit_code: 0", "test_exit_code: 125", 1), "requires the exact authority-only", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expected := SpecCounts{Requirements: 2, Scenarios: 3}
			if tt.name == "count mismatch" {
				expected.Requirements = 3
			}
			got := ValidateVerifyReportAdmission(tt.report, expected)
			if got.Valid != tt.valid || (tt.reason != "" && !strings.Contains(got.Reason, tt.reason)) {
				t.Fatalf("admission = %#v, want valid=%v reason containing %q", got, tt.valid, tt.reason)
			}
		})
	}
}

func TestCountSpecRequirementsAndScenariosUsesActualArtifacts(t *testing.T) {
	counts := countSpecRequirementsAndScenarios([]string{
		"### Requirement: First\n#### Scenario: A\n#### Scenario: B\n",
		"### Requirement: Second\n#### Scenario: C\n",
	})
	if counts != (SpecCounts{Requirements: 2, Scenarios: 3}) {
		t.Fatalf("counts = %#v, want 2 requirements and 3 scenarios", counts)
	}
}

func testVerifyEnvelope(verdict string, blockers, critical int, requirements, scenarios string, testExit, buildExit int) string {
	return strings.Join([]string{
		"```yaml",
		"schema: gentle-ai.verify-result/v1",
		"evidence_revision: sha256:" + strings.Repeat("a", 64),
		"verdict: " + verdict,
		"blockers: " + itoa(blockers),
		"critical_findings: " + itoa(critical),
		"requirements: " + requirements,
		"scenarios: " + scenarios,
		"test_command: go test ./internal/example",
		"test_exit_code: " + itoa(testExit),
		"test_output_hash: sha256:" + strings.Repeat("b", 64),
		"build_command: go test ./cmd/gentle-ai",
		"build_exit_code: " + itoa(buildExit),
		"build_output_hash: sha256:" + strings.Repeat("c", 64),
		"```",
	}, "\n")
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	return "1"
}
