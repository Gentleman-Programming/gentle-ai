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
const MaxVerifyReportBytes = 1 << 20
const VerifyEmptyOutputHash = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// VerifyReportContract is the stable, user-facing shape validated before an
// SDD verify report is admitted. It contains no artifact or repository state.
type VerifyReportContract struct {
	Schema              string
	MaxBytes            int
	RequiredFields      []string
	Verdicts            []string
	AuthorityOnlyFields []string
	EmptyOutputHash     string
}

var verifyReportRequiredFields = []string{
	"schema", "evidence_revision", "verdict", "blockers", "critical_findings",
	"requirements", "scenarios", "test_command", "test_exit_code", "test_output_hash",
	"build_command", "build_exit_code", "build_output_hash",
}

var verifyReportAuthorityOnlyFields = []string{
	"authority_only_failure", "missing_review_authority", "substantive_failure",
	"command_failed", "observed_authority_revision",
}

var verifyReportVerdicts = []string{"pass", "pass_with_warnings", "fail"}

func VerifyReportValidationContract() VerifyReportContract {
	return VerifyReportContract{
		Schema:              VerifyResultSchema,
		MaxBytes:            MaxVerifyReportBytes,
		RequiredFields:      append([]string(nil), verifyReportRequiredFields...),
		Verdicts:            append([]string(nil), verifyReportVerdicts...),
		AuthorityOnlyFields: append([]string(nil), verifyReportAuthorityOnlyFields...),
		EmptyOutputHash:     VerifyEmptyOutputHash,
	}
}

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
var requirementHeadingPattern = regexp.MustCompile(`(?m)^### (?:Requirement|REQ-[0-9]+):\s+\S`)
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
	allowed := make(map[string]bool, len(verifyReportRequiredFields)+len(verifyReportAuthorityOnlyFields))
	for _, field := range append(append([]string{}, verifyReportRequiredFields...), verifyReportAuthorityOnlyFields...) {
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
	for _, required := range verifyReportRequiredFields {
		if _, ok := fields[required]; !ok {
			return report, fmt.Sprintf("missing %s in verify result envelope", required)
		}
	}
	extensionCount := 0
	for _, field := range verifyReportAuthorityOnlyFields {
		if _, ok := fields[field]; ok {
			extensionCount++
		}
	}
	if extensionCount != 0 && extensionCount != len(verifyReportAuthorityOnlyFields) {
		return report, fmt.Sprintf("authority-only extension must contain exactly %d fields", len(verifyReportAuthorityOnlyFields))
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
	if !validVerifyReportVerdict(report.Verdict) {
		return report, fmt.Sprintf("invalid verdict %s", report.Verdict)
	}
	if report.AuthorityOnly {
		for _, pair := range [][2]string{{"authority_only_failure", "true"}, {"missing_review_authority", "true"}, {"substantive_failure", "false"}, {"command_failed", "false"}} {
			if fields[pair[0]] != pair[1] {
				return report, "invalid authority-only extension"
			}
		}
		if report.Verdict != "fail" || report.TestExit != 125 || report.BuildExit != 125 || report.Blockers == 0 || report.Critical == 0 || fields["test_output_hash"] != VerifyEmptyOutputHash || fields["build_output_hash"] != VerifyEmptyOutputHash || !sha256IdentityPattern.MatchString(fields["observed_authority_revision"]) {
			return report, "invalid authority-only extension"
		}
	}
	return report, ""
}

func validVerifyReportVerdict(verdict string) bool {
	for _, valid := range verifyReportVerdicts {
		if verdict == valid {
			return true
		}
	}
	return false
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

type remediationIdentity struct {
	Revision   string
	LineageID  string
	Generation int
	FixBatch   int
}

type remediationFenceBlock struct {
	start, end int
	depth      int
	token      string
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
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	blocks, invalid := scanRemediationFences(lines)
	blocksByStart := make(map[int]remediationFenceBlock, len(blocks))
	for _, block := range blocks {
		blocksByStart[block.start] = block
	}
	claims := 0
	var candidate remediationResultEvaluation
	candidateEnd := -1
	var fallback remediationResultEvaluation

	for _, block := range blocks {
		if block.end < 0 {
			continue
		}
		if block.token == "json" {
			if identity, ok := remediationJSONIdentity(lines[block.start+1 : block.end]); ok && remediationIdentityMatches(identity, expectedRevision, bindings) {
				claims++
			}
			continue
		}
		if block.token != "yaml" {
			continue
		}

		if identity, ok := remediationYAMLIdentity(lines[block.start+1 : block.end]); ok && remediationIdentityMatches(identity, expectedRevision, bindings) {
			claims++
		}
		envelope := parseRemediationResultEnvelope(strings.Join(lines[block.start:block.end+1], "\n"), expectedRevision, bindings...)
		if fallback.EvidenceRevision == "" && envelope.EvidenceRevision != "" {
			fallback = envelope
		}

		if block.depth != 0 {
			continue
		}

		evidenceStart := nextNonemptyLine(lines, block.end+1)
		if evidenceStart < 0 {
			continue
		}
		evidenceBlock, evidenceOK := blocksByStart[evidenceStart]
		if !evidenceOK || evidenceBlock.depth != 0 || evidenceBlock.token != "json" {
			continue
		}

		pair := strings.Join(lines[block.start:evidenceBlock.end+1], "\n")
		evaluation := parseRemediationResultEnvelope(pair, expectedRevision, bindings...)
		if fallback.EvidenceRevision == "" && evaluation.EvidenceRevision != "" {
			fallback = evaluation
		}
		if evaluation.Complete && len(bindings) <= 1 {
			candidate = evaluation
			candidateEnd = evidenceBlock.end
		}
	}

	if invalid || candidateEnd < 0 || claims != 1 || !remediationTrailingContentValid(lines, candidateEnd, blocksByStart) {
		fallback.Complete = false
		return fallback
	}
	return candidate
}

func scanRemediationFences(lines []string) ([]remediationFenceBlock, bool) {
	blocks := make([]remediationFenceBlock, 0)
	var active remediationFenceBlock
	open := false
	for index, line := range lines {
		depth, token, ok := remediationFence(line)
		if !ok {
			continue
		}
		if open {
			if depth != active.depth || token != "bare" {
				continue
			}
			active.end = index
			blocks = append(blocks, active)
			open = false
			continue
		}
		active = remediationFenceBlock{start: index, end: -1, depth: depth, token: token}
		open = true
	}
	if open {
		blocks = append(blocks, active)
	}
	return blocks, open
}

func remediationFence(line string) (int, string, bool) {
	depth := 0
	for {
		trimmed := strings.TrimLeft(line, " \t")
		if !strings.HasPrefix(trimmed, ">") {
			line = trimmed
			break
		}
		depth++
		line = strings.TrimLeft(trimmed[1:], " \t")
	}

	line = strings.TrimSpace(line)
	switch line {
	case "```":
		return depth, "bare", true
	case "```yaml":
		return depth, "yaml", true
	case "```json":
		return depth, "json", true
	}
	if strings.HasPrefix(line, "```") && strings.TrimSpace(strings.TrimPrefix(line, "```")) != "" {
		return depth, "other", true
	}
	return 0, "", false
}

func nextNonemptyLine(lines []string, start int) int {
	for index := start; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) != "" {
			return index
		}
	}
	return -1
}

func remediationYAMLIdentity(lines []string) (remediationIdentity, bool) {
	fields := make(map[string]string)
	for _, line := range lines {
		if key, value, ok := strings.Cut(remediationFenceContent(line), ":"); ok {
			fields[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return remediationIdentityFromFields(fields)
}

func remediationJSONIdentity(lines []string) (remediationIdentity, bool) {
	var fields map[string]any
	decoder := json.NewDecoder(strings.NewReader(strings.Join(remediationFenceContents(lines), "\n")))
	if err := decoder.Decode(&fields); err != nil {
		return remediationIdentity{}, false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF || fields["schema"] != RemediationResultSchema {
		return remediationIdentity{}, false
	}

	identity := remediationIdentity{
		Revision:  firstStringFieldAny(fields, "failed_evidence_revision", "failed_verify_revision", "failedEvidenceRevision", "failedVerifyRevision"),
		LineageID: firstStringFieldAny(fields, "lineage_id", "lineageId", "lineageID"),
	}
	identity.Generation, _ = firstIntField(fields, "generation")
	identity.FixBatch, _ = firstIntField(fields, "fix_batch", "fixBatch")
	return identity, identity.Revision != ""
}

func remediationIdentityFromFields(fields map[string]string) (remediationIdentity, bool) {
	if fields["schema"] != RemediationResultSchema {
		return remediationIdentity{}, false
	}
	identity := remediationIdentity{
		Revision:  firstStringField(fields, "failed_evidence_revision", "failed_verify_revision", "failedEvidenceRevision", "failedVerifyRevision"),
		LineageID: firstStringField(fields, "lineage_id", "lineageId", "lineageID"),
	}
	identity.Generation, _ = parseFirstIntField(fields, "generation")
	identity.FixBatch, _ = parseFirstIntField(fields, "fix_batch", "fixBatch")
	return identity, identity.Revision != ""
}

func firstStringField(fields map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := fields[key]; value != "" {
			return value
		}
	}
	return ""
}

func firstStringFieldAny(fields map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := fields[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func firstIntField(fields map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		switch value := fields[key].(type) {
		case float64:
			if value >= 0 && value == float64(int(value)) {
				return int(value), true
			}
		case string:
			if parsed, ok := parseNonnegativeInt(value); ok {
				return parsed, true
			}
		}
	}
	return 0, false
}

func parseFirstIntField(fields map[string]string, keys ...string) (int, bool) {
	for _, key := range keys {
		if parsed, ok := parseNonnegativeInt(fields[key]); ok {
			return parsed, true
		}
	}
	return 0, false
}

func remediationIdentityMatches(identity remediationIdentity, expectedRevision string, bindings []RemediationBinding) bool {
	if identity.Revision != expectedRevision || len(bindings) > 1 {
		return false
	}
	if len(bindings) == 0 {
		return true
	}
	binding := bindings[0]
	return identity.LineageID == binding.LineageID && identity.Generation == binding.Generation && identity.FixBatch == binding.FixBatch
}

func remediationTrailingContentValid(lines []string, start int, blocksByStart map[int]remediationFenceBlock) bool {
	for index := start + 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == "" {
			continue
		}
		if block, ok := blocksByStart[index]; ok {
			if block.end < 0 || remediationFenceContainsResult(lines[block.start+1:block.end]) {
				return false
			}
			index = block.end
		} else {
			return false
		}
	}
	return true
}

func remediationFenceContainsResult(lines []string) bool {
	for _, line := range lines {
		line = remediationFenceContent(line)
		if strings.HasPrefix(line, "schema:") {
			schema := strings.TrimSpace(strings.TrimPrefix(line, "schema:"))
			if schema == RemediationResultSchema || schema == "gentle-ai.remediation-evidence/v1" {
				return true
			}
		}
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(strings.Join(remediationFenceContents(lines), "\n")), &fields); err == nil {
		schema, _ := fields["schema"].(string)
		return schema == RemediationResultSchema || schema == "gentle-ai.remediation-evidence/v1"
	}
	return false
}

func remediationFenceContent(line string) string {
	for strings.HasPrefix(strings.TrimLeft(line, " \t"), ">") {
		trimmed := strings.TrimLeft(line, " \t")
		line = strings.TrimLeft(trimmed[1:], " \t")
	}
	return strings.TrimSpace(line)
}

func remediationFenceContents(lines []string) []string {
	contents := make([]string, len(lines))
	for index, line := range lines {
		contents[index] = remediationFenceContent(line)
	}
	return contents
}

func parseRemediationResultEnvelope(text, expectedRevision string, bindings ...RemediationBinding) remediationResultEvaluation {
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
