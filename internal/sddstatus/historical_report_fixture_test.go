package sddstatus

import "strings"

// Historical report bytes are fixtures, never certificates or execution evidence.
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
