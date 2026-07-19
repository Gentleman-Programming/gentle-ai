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
