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

// VerifyReportContract is the stable, user-facing shape validated before an
// SDD verify report is admitted. It contains no artifact or repository state.
type VerifyReportContract struct {
	Schema         string
	MaxBytes       int
	RequiredFields []string
	Verdicts       []string
}

var verifyReportRequiredFields = []string{
	"schema", "evidence_revision", "verdict", "blockers", "critical_findings",
	"requirements", "scenarios", "test_command", "test_exit_code", "test_output_hash",
	"build_command", "build_exit_code", "build_output_hash",
}

// verifyReportLegacyFields are decoder-only compatibility inputs. They are not
// part of the current verification contract and never affect review routing.
var verifyReportLegacyFields = []string{"missing_review_authority"}

var verifyReportVerdicts = []string{"pass", "pass_with_warnings", "fail"}

func VerifyReportValidationContract() VerifyReportContract {
	return VerifyReportContract{
		Schema:         VerifyResultSchema,
		MaxBytes:       MaxVerifyReportBytes,
		RequiredFields: append([]string(nil), verifyReportRequiredFields...),
		Verdicts:       append([]string(nil), verifyReportVerdicts...),
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
	// Incomplete marks evidence that did not run independent SDD test and build
	// verification. It must re-enter verify rather than route to review.
	Incomplete bool
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
	LegacyMissingReview                     bool
}

var sha256IdentityPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func diagnoseSHA256Identity(val string) string {
	const prefix = "sha256:"
	const expectedHexLen = 64
	got := fmt.Sprintf("%q (length %d)", truncateSHA256Value(val, 80), len(val))
	if !strings.HasPrefix(val, prefix) {
		return fmt.Sprintf("expected sha256:<64 lowercase hex>, got %s: missing '%s' prefix", got, prefix)
	}
	hexPart := val[len(prefix):]
	if len(hexPart) != expectedHexLen {
		return fmt.Sprintf("expected sha256:<64 lowercase hex>, got %s: wrong length (got %d hex characters after prefix, expected %d)", got, len(hexPart), expectedHexLen)
	}
	for i, r := range hexPart {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		pos := len(prefix) + i
		if r >= 'A' && r <= 'F' {
			return fmt.Sprintf("expected sha256:<64 lowercase hex>, got %s: non-hex character %q at position %d (must be lowercase)", got, r, pos)
		}
		return fmt.Sprintf("expected sha256:<64 lowercase hex>, got %s: non-hex character %q at position %d", got, r, pos)
	}
	return fmt.Sprintf("expected sha256:<64 lowercase hex>, got %s", got)
}

func truncateSHA256Value(val string, limit int) string {
	if len(val) <= limit {
		return val
	}
	if limit <= 3 {
		return val[:limit]
	}
	return val[:limit-3] + "..."
}

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
		return verifyResultEvaluation{
			Incomplete:       true,
			Reason:           "verification evidence is incomplete: " + reason,
			EvidenceRevision: report.EvidenceRevision,
		}
	}
	evaluation := verifyResultEvaluation{EvidenceRevision: report.EvidenceRevision}
	if report.LegacyMissingReview {
		evaluation.Incomplete = true
		evaluation.Reason = "independent test and build execution evidence is incomplete; rerun SDD verification"
		return evaluation
	}
	if report.Verdict == "fail" && (report.Requirements.Completed != report.Requirements.Total || report.Scenarios.Completed != report.Scenarios.Total) {
		evaluation.Incomplete = true
		evaluation.Reason = "failed verification evidence is incomplete; rerun SDD verification"
		return evaluation
	}
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
	} else if report.TestExit == 0 && report.BuildExit == 0 && report.Blockers == 0 && report.Critical == 0 && complete {
		result.Reason = "fail verdict is contradictory with all-green evidence"
		return result
	}
	result.Valid, result.Reason = true, ""
	return result
}

func parseVerifyReport(text string) (verifyReport, string) {
	lines, end, reason := parseLeadingEnvelope(text)
	if reason != "" {
		return verifyReport{}, reason
	}
	allowed := make(map[string]bool, len(verifyReportRequiredFields)+len(verifyReportLegacyFields))
	for _, field := range append(append([]string{}, verifyReportRequiredFields...), verifyReportLegacyFields...) {
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
	if fields["schema"] != VerifyResultSchema {
		return report, fmt.Sprintf("unsupported verify result schema %s", fields["schema"])
	}
	for _, field := range []string{"evidence_revision", "test_output_hash", "build_output_hash"} {
		if !sha256IdentityPattern.MatchString(fields[field]) {
			return report, fmt.Sprintf("invalid %s in verify result envelope: %s", field, diagnoseSHA256Identity(fields[field]))
		}
	}
	if !isConcreteEvidence(fields["test_command"]) || !isConcreteEvidence(fields["build_command"]) {
		return report, "test_command and build_command require concrete current execution evidence"
	}
	report.Verdict = fields["verdict"]
	if legacy, ok := fields["missing_review_authority"]; ok {
		if legacy != "true" && legacy != "false" {
			return report, "invalid legacy missing_review_authority field"
		}
		report.LegacyMissingReview = legacy == "true"
	}
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

// verifyEnvelopeFenceRefusal names the exact first line the contract admits
// and the command that checks candidate bytes, so a refused report can be
// fixed from the message alone (#2828).
const verifyEnvelopeFenceRefusal = "missing valid gentle-ai.verify-result/v1 envelope: the first non-empty line must be ```yaml (```yml and any letter case are admitted; ~~~ fences, untagged fences, and content before the fence are not); check the exact bytes with gentle-ai sdd-verify-validate --input <path|-> --requirements <n> --scenarios <n>"

func parseLeadingEnvelope(text string) ([]string, int, string) {
	// PowerShell 5.1 writes a UTF-8 BOM that TrimSpace never removes (#2828).
	text = strings.TrimPrefix(text, "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		return nil, -1, "YAML front matter is unsupported; the first non-empty content must be a fenced yaml envelope"
	}
	if len(lines) == 0 || !isYAMLFenceOpener(lines[0]) {
		return nil, -1, verifyEnvelopeFenceRefusal
	}
	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == "```" {
			return lines, index, ""
		}
	}
	return nil, -1, "unterminated verify result envelope"
}

func isYAMLFenceOpener(line string) bool {
	tag, fenced := strings.CutPrefix(strings.TrimSpace(line), "```")
	return fenced && (strings.EqualFold(tag, "yaml") || strings.EqualFold(tag, "yml"))
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
	Commands               []remediationCommandEvidence `json:"commands"`
	RuntimeHarness         remediationRuntimeEvidence   `json:"runtime_harness"`
	Rollback               remediationRollbackEvidence  `json:"rollback"`
}

type remediationIdentity struct {
	Revision string
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

func parseRemediationResult(text, expectedRevision string) remediationResultEvaluation {
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
			if identity, ok := remediationJSONIdentity(lines[block.start+1 : block.end]); ok && remediationIdentityMatches(identity, expectedRevision) {
				claims++
			}
			continue
		}
		if block.token != "yaml" {
			continue
		}

		if identity, ok := remediationYAMLIdentity(lines[block.start+1 : block.end]); ok && remediationIdentityMatches(identity, expectedRevision) {
			claims++
		}
		envelope := parseRemediationResultEnvelope(strings.Join(lines[block.start:block.end+1], "\n"), expectedRevision)
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
		evaluation := parseRemediationResultEnvelope(pair, expectedRevision)
		if fallback.EvidenceRevision == "" && evaluation.EvidenceRevision != "" {
			fallback = evaluation
		}
		if evaluation.Complete {
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
		Revision: firstStringFieldAny(fields, "failed_evidence_revision", "failed_verify_revision", "failedEvidenceRevision", "failedVerifyRevision"),
	}
	return identity, identity.Revision != ""
}

func remediationIdentityFromFields(fields map[string]string) (remediationIdentity, bool) {
	if fields["schema"] != RemediationResultSchema {
		return remediationIdentity{}, false
	}
	identity := remediationIdentity{
		Revision: firstStringField(fields, "failed_evidence_revision", "failed_verify_revision", "failedEvidenceRevision", "failedVerifyRevision"),
	}
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

func remediationIdentityMatches(identity remediationIdentity, expectedRevision string) bool {
	return identity.Revision == expectedRevision
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

func parseRemediationResultEnvelope(text, expectedRevision string) remediationResultEvaluation {
	lines, end, reason := parseLeadingEnvelope(text)
	if reason != "" {
		return remediationResultEvaluation{}
	}
	allowed := map[string]bool{
		"schema": true, "status": true, "failed_evidence_revision": true,
		"focused_tests": true, "runtime_harness": true, "rollback_boundary": true,
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
