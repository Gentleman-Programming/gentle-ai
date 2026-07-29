package sddstatus

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

const VerifyResultSchema = "gentle-ai.verify-result/v1"
const RemediationResultSchema = "gentle-ai.remediation-result/v1"
const verifyEmptyOutputHash = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

type SpecCounts struct {
	Requirements int
	Scenarios    int
}

type verifyCompletion struct {
	Completed int
	Total     int
}

type verifyResultEvaluation struct {
	Passing bool
	// Stale marks internally complete evidence whose only defect is a totals
	// mismatch against the current spec counts.
	Stale            bool
	Reason           string
	EvidenceRevision string
}

// VerifyReportAdmission is the pure, pre-persistence validity decision.
type VerifyReportAdmission struct {
	Valid            bool   `json:"valid"`
	Reason           string `json:"reason,omitempty"`
	Verdict          string `json:"verdict,omitempty"`
	EvidenceRevision string `json:"evidence_revision,omitempty"`
}

type verifyReport struct {
	Fields                                  map[string]string
	Verdict, EvidenceRevision               string
	Blockers, Critical, TestExit, BuildExit int
	Requirements, Scenarios                 verifyCompletion
	AuthorityOnly                           bool
}

var sha256IdentityPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var requirementHeadingPattern = regexp.MustCompile(`(?m)^### Requirement:\s+\S`)
var scenarioHeadingPattern = regexp.MustCompile(`(?m)^#### Scenario:\s+\S`)

func countSpecRequirementsAndScenarios(specs []string) SpecCounts {
	var counts SpecCounts
	for _, spec := range specs {
		counts.Requirements += len(requirementHeadingPattern.FindAllStringIndex(spec, -1))
		counts.Scenarios += len(scenarioHeadingPattern.FindAllStringIndex(spec, -1))
	}
	return counts
}

func parseVerifyResult(text string, expected SpecCounts) verifyResultEvaluation {
	report, reason := parseVerifyReport(text)
	if reason != "" {
		return verifyResultEvaluation{Reason: reason, EvidenceRevision: report.EvidenceRevision}
	}
	evaluation := verifyResultEvaluation{EvidenceRevision: report.EvidenceRevision}
	if report.TestExit != 0 {
		evaluation.Reason = "test_exit_code must be zero for archive readiness"
		return evaluation
	}
	if report.BuildExit != 0 {
		evaluation.Reason = "build_exit_code must be zero for archive readiness"
		return evaluation
	}
	internallyComplete := report.Requirements.Completed == report.Requirements.Total && report.Scenarios.Completed == report.Scenarios.Total
	totalsMatch := report.Requirements.Total == expected.Requirements && report.Scenarios.Total == expected.Scenarios
	if !totalsMatch {
		evaluation.Stale = report.Verdict != "fail" && report.Blockers == 0 && report.Critical == 0 && internallyComplete
	}
	if report.Requirements.Total != expected.Requirements {
		evaluation.Reason = fmt.Sprintf("verify result total %d does not match actual requirement count %d", report.Requirements.Total, expected.Requirements)
		return evaluation
	}
	if report.Scenarios.Total != expected.Scenarios {
		evaluation.Reason = fmt.Sprintf("verify result total %d does not match actual scenario count %d", report.Scenarios.Total, expected.Scenarios)
		return evaluation
	}
	if report.Blockers != 0 {
		evaluation.Reason = "blockers must be zero for archive readiness"
		return evaluation
	}
	if report.Critical != 0 {
		evaluation.Reason = "critical_findings must be zero for archive readiness"
		return evaluation
	}
	if report.Requirements.Completed != report.Requirements.Total {
		evaluation.Reason = "requirements are incomplete"
		return evaluation
	}
	if report.Scenarios.Completed != report.Scenarios.Total {
		evaluation.Reason = "scenarios are incomplete"
		return evaluation
	}
	if report.Verdict == "fail" {
		evaluation.Reason = "verdict requires remediation"
		return evaluation
	}
	evaluation.Passing = true
	return evaluation
}

// ValidateVerifyReportAdmission validates exact report bytes before persistence.
func ValidateVerifyReportAdmission(text string, expected SpecCounts) VerifyReportAdmission {
	report, reason := parseVerifyReport(text)
	result := VerifyReportAdmission{Reason: reason}
	if reason != "" {
		return result
	}
	result.Verdict, result.EvidenceRevision = report.Verdict, report.EvidenceRevision
	if expected.Requirements < 0 || expected.Scenarios < 0 {
		result.Reason = "expected requirement and scenario counts must be nonnegative"
		return result
	}
	if report.Requirements.Total != expected.Requirements {
		result.Reason = fmt.Sprintf("verify result total %d does not match actual requirement count %d", report.Requirements.Total, expected.Requirements)
		return result
	}
	if report.Scenarios.Total != expected.Scenarios {
		result.Reason = fmt.Sprintf("verify result total %d does not match actual scenario count %d", report.Scenarios.Total, expected.Scenarios)
		return result
	}
	complete := report.Requirements.Completed == report.Requirements.Total && report.Scenarios.Completed == report.Scenarios.Total
	if report.Verdict != "fail" {
		if report.TestExit != 0 || report.BuildExit != 0 || report.Blockers != 0 || report.Critical != 0 || !complete {
			result.Reason = "passing verdict contradicts failing or incomplete evidence"
			return result
		}
	} else if !report.AuthorityOnly {
		if report.TestExit == 125 || report.BuildExit == 125 {
			result.Reason = "exit code 125 requires the exact authority-only extension"
			return result
		}
		if report.TestExit == 0 && report.BuildExit == 0 && report.Blockers == 0 && report.Critical == 0 && complete {
			result.Reason = "fail verdict is contradictory with all-green evidence"
			return result
		}
	}
	result.Valid, result.Reason = true, ""
	return result
}

func parseVerifyReport(text string) (verifyReport, string) {
	lines, end, reason := parseLeadingEnvelope(text)
	if reason != "" {
		return verifyReport{}, reason
	}
	base := []string{"schema", "evidence_revision", "verdict", "blockers", "critical_findings", "requirements", "scenarios", "test_command", "test_exit_code", "test_output_hash", "build_command", "build_exit_code", "build_output_hash"}
	extension := []string{"authority_only_failure", "missing_review_authority", "substantive_failure", "command_failed", "observed_authority_revision"}
	allowed := make(map[string]bool, len(base)+len(extension))
	for _, field := range append(base, extension...) {
		allowed[field] = true
	}
	fields, reason := parseScalarFields(lines[1:end], allowed, "verify result")
	report := verifyReport{Fields: fields}
	if fields["schema"] == VerifyResultSchema && sha256IdentityPattern.MatchString(fields["evidence_revision"]) {
		report.EvidenceRevision = fields["evidence_revision"]
	}
	if reason != "" {
		return report, reason
	}
	for _, required := range base {
		if _, ok := fields[required]; !ok {
			return report, fmt.Sprintf("missing %s in verify result envelope", required)
		}
	}
	extensionCount := 0
	for _, field := range extension {
		if _, ok := fields[field]; ok {
			extensionCount++
		}
	}
	if extensionCount != 0 && extensionCount != len(extension) {
		return report, "authority-only extension must contain exactly five fields"
	}
	if fields["schema"] != VerifyResultSchema {
		return report, fmt.Sprintf("unsupported verify result schema %s", fields["schema"])
	}
	for _, field := range []string{"evidence_revision", "test_output_hash", "build_output_hash"} {
		if !sha256IdentityPattern.MatchString(fields[field]) {
			return report, fmt.Sprintf("invalid %s in verify result envelope", field)
		}
	}
	if !isConcreteEvidence(fields["test_command"]) || !isConcreteEvidence(fields["build_command"]) {
		return report, "test_command and build_command require concrete current execution evidence"
	}
	report.Verdict, report.AuthorityOnly = fields["verdict"], extensionCount != 0
	for _, target := range []struct {
		name  string
		value *int
	}{{"blockers", &report.Blockers}, {"critical_findings", &report.Critical}, {"test_exit_code", &report.TestExit}, {"build_exit_code", &report.BuildExit}} {
		value, ok := parseNonnegativeInt(fields[target.name])
		if !ok {
			return report, fmt.Sprintf("invalid %s in verify result envelope", target.name)
		}
		*target.value = value
	}
	var ok bool
	if report.Requirements, ok = parseVerifyCompletion(fields["requirements"]); !ok {
		return report, "invalid requirements in verify result envelope"
	}
	if report.Scenarios, ok = parseVerifyCompletion(fields["scenarios"]); !ok {
		return report, "invalid scenarios in verify result envelope"
	}
	if report.Verdict != "pass" && report.Verdict != "pass_with_warnings" && report.Verdict != "fail" {
		return report, fmt.Sprintf("invalid verdict %s", report.Verdict)
	}
	if report.AuthorityOnly {
		for _, pair := range [][2]string{{"authority_only_failure", "true"}, {"missing_review_authority", "true"}, {"substantive_failure", "false"}, {"command_failed", "false"}} {
			if fields[pair[0]] != pair[1] {
				return report, "invalid authority-only extension"
			}
		}
		if report.Verdict != "fail" || report.TestExit != 125 || report.BuildExit != 125 || report.Blockers == 0 || report.Critical == 0 || fields["test_output_hash"] != verifyEmptyOutputHash || fields["build_output_hash"] != verifyEmptyOutputHash || !sha256IdentityPattern.MatchString(fields["observed_authority_revision"]) {
			return report, "invalid authority-only extension"
		}
	}
	return report, ""
}

func parseLeadingEnvelope(text string) ([]string, int, string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		return nil, -1, "YAML front matter is unsupported; the first non-empty content must be a fenced yaml envelope"
	}
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "```yaml" {
		return nil, -1, "missing valid gentle-ai.verify-result/v1 envelope: the first non-empty content must be fenced yaml"
	}
	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == "```" {
			return lines, index, ""
		}
	}
	return nil, -1, "unterminated verify result envelope"
}

func parseScalarFields(lines []string, allowed map[string]bool, label string) (map[string]string, string) {
	fields := make(map[string]string, len(allowed))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !ok || key == "" || value == "" {
			return fields, "malformed " + label + " field"
		}
		if !allowed[key] {
			return fields, fmt.Sprintf("unknown %s field %s", label, key)
		}
		if _, duplicate := fields[key]; duplicate {
			return fields, fmt.Sprintf("duplicate %s field %s", label, key)
		}
		fields[key] = value
	}
	return fields, ""
}

func parseNonnegativeInt(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, false
		}
	}
	parsed, err := strconv.Atoi(value)
	return parsed, err == nil
}

func parseVerifyCompletion(value string) (verifyCompletion, bool) {
	completedRaw, totalRaw, ok := strings.Cut(value, "/")
	if !ok || strings.Contains(totalRaw, "/") {
		return verifyCompletion{}, false
	}
	completed, completedOK := parseNonnegativeInt(completedRaw)
	total, totalOK := parseNonnegativeInt(totalRaw)
	if !completedOK || !totalOK || completed > total {
		return verifyCompletion{}, false
	}
	return verifyCompletion{Completed: completed, Total: total}, true
}

type remediationResultEvaluation struct {
	Complete         bool
	EvidenceRevision string
}

type remediationEvidence struct {
	Schema                 string                       `json:"schema"`
	FailedEvidenceRevision string                       `json:"failed_evidence_revision"`
	LineageID              string                       `json:"lineage_id,omitempty"`
	Generation             int                          `json:"generation,omitempty"`
	FixBatch               int                          `json:"fix_batch,omitempty"`
	Commands               []remediationCommandEvidence `json:"commands"`
	RuntimeHarness         remediationRuntimeEvidence   `json:"runtime_harness"`
	Rollback               remediationRollbackEvidence  `json:"rollback"`
}

type RemediationBinding struct {
	LineageID  string
	Generation int
	FixBatch   int
}

type remediationCommandEvidence struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Result   string `json:"result"`
}

type remediationRuntimeEvidence struct {
	Status   string `json:"status"`
	Command  string `json:"command"`
	Result   string `json:"result"`
	NAReason string `json:"na_reason"`
}

type remediationRollbackEvidence struct {
	Boundary string `json:"boundary"`
	Evidence string `json:"evidence"`
}

func parseRemediationResult(text, expectedRevision string, bindings ...RemediationBinding) remediationResultEvaluation {
	lines, end, reason := parseLeadingEnvelope(text)
	if reason != "" {
		return remediationResultEvaluation{}
	}
	allowed := map[string]bool{
		"schema": true, "status": true, "failed_evidence_revision": true,
		"focused_tests": true, "runtime_harness": true, "rollback_boundary": true,
		"lineage_id": true, "generation": true, "fix_batch": true,
	}
	fields, reason := parseScalarFields(lines[1:end], allowed, "remediation result")
	if reason != "" {
		return remediationResultEvaluation{}
	}
	revision := fields["failed_evidence_revision"]
	evaluation := remediationResultEvaluation{EvidenceRevision: revision}
	if fields["schema"] != RemediationResultSchema || fields["status"] != "complete" || revision != expectedRevision {
		return evaluation
	}
	if len(bindings) > 1 {
		return evaluation
	}
	if len(bindings) == 1 {
		binding := bindings[0]
		generation, generationOK := parseNonnegativeInt(fields["generation"])
		fixBatch, fixBatchOK := parseNonnegativeInt(fields["fix_batch"])
		if fields["lineage_id"] != binding.LineageID || !generationOK || generation != binding.Generation || !fixBatchOK || fixBatch != binding.FixBatch {
			return evaluation
		}
	}
	if fields["focused_tests"] != "passed" || fields["rollback_boundary"] != "recorded" {
		return evaluation
	}
	if fields["runtime_harness"] != "passed" && fields["runtime_harness"] != "not_applicable" {
		return evaluation
	}
	evidence, ok := parseRemediationEvidence(lines[end+1:])
	if !ok || evidence.FailedEvidenceRevision != expectedRevision || len(evidence.Commands) == 0 {
		return evaluation
	}
	if len(bindings) == 1 {
		binding := bindings[0]
		if evidence.LineageID != binding.LineageID || evidence.Generation != binding.Generation || evidence.FixBatch != binding.FixBatch {
			return evaluation
		}
	}
	for _, command := range evidence.Commands {
		if command.ExitCode != 0 || !isConcreteEvidence(command.Command) || !isConcreteEvidence(command.Result) {
			return evaluation
		}
	}
	if evidence.RuntimeHarness.Status != fields["runtime_harness"] {
		return evaluation
	}
	switch evidence.RuntimeHarness.Status {
	case "passed":
		if !isConcreteEvidence(evidence.RuntimeHarness.Command) || !isConcreteEvidence(evidence.RuntimeHarness.Result) || strings.TrimSpace(evidence.RuntimeHarness.NAReason) != "" {
			return evaluation
		}
	case "not_applicable":
		if evidence.RuntimeHarness.Command != "" || evidence.RuntimeHarness.Result != "" || !isConcreteNAReason(evidence.RuntimeHarness.NAReason) {
			return evaluation
		}
	default:
		return evaluation
	}
	if !isConcreteEvidence(evidence.Rollback.Boundary) || !isConcreteEvidence(evidence.Rollback.Evidence) {
		return evaluation
	}
	evaluation.Complete = true
	return evaluation
}

func parseRemediationEvidence(lines []string) (remediationEvidence, bool) {
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if !strings.HasPrefix(text, "```json\n") {
		return remediationEvidence{}, false
	}
	text = strings.TrimPrefix(text, "```json\n")
	end := strings.Index(text, "\n```")
	if end < 0 || strings.TrimSpace(text[end+4:]) != "" {
		return remediationEvidence{}, false
	}
	decoder := json.NewDecoder(strings.NewReader(text[:end]))
	decoder.DisallowUnknownFields()
	var evidence remediationEvidence
	if err := decoder.Decode(&evidence); err != nil {
		return remediationEvidence{}, false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return remediationEvidence{}, false
	}
	if evidence.Schema != "gentle-ai.remediation-evidence/v1" {
		return remediationEvidence{}, false
	}
	return evidence, true
}

func isConcreteEvidence(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.ContainsAny(trimmed, "{}<>") {
		return false
	}
	switch strings.ToLower(trimmed) {
	case "n/a", "na", "none", "todo", "tbd", "pass", "passed", "success", "recorded", "placeholder":
		return false
	}
	return true
}

func isConcreteNAReason(value string) bool {
	trimmed := strings.TrimSpace(value)
	return len(trimmed) >= 20 && strings.Contains(strings.ToLower(trimmed), "because") && isConcreteEvidence(trimmed)
}
