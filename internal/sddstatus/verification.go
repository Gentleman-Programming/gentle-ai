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

// remediationClaim is one identity assertion found while scanning cumulative
// apply-progress: a strict remediation result envelope, optionally completed by
// an adjacent JSON evidence fence. Complete pairs carry EndLine set to the end
// of their evidence fence so the terminal pair is unambiguous.
type remediationClaim struct {
	Revision   string
	LineageID  string
	Generation int
	FixBatch   int
	Complete   bool
	EndLine    int
}

// remediationResultFields is the scalar field vocabulary shared by the
// remediation result claim and pair validators.
var remediationResultFields = map[string]bool{
	"schema": true, "status": true, "failed_evidence_revision": true,
	"focused_tests": true, "runtime_harness": true, "rollback_boundary": true,
	"lineage_id": true, "generation": true, "fix_batch": true,
}

func parseRemediationResult(text, expectedRevision string, bindings ...RemediationBinding) remediationResultEvaluation {
	if len(bindings) > 1 {
		return remediationResultEvaluation{}
	}
	var binding *RemediationBinding
	if len(bindings) == 1 {
		binding = &bindings[0]
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	claims := scanRemediationClaims(strings.Split(text, "\n"), expectedRevision, binding)
	var terminal *remediationClaim
	for index := range claims {
		if terminal == nil || claims[index].EndLine > terminal.EndLine {
			terminal = &claims[index]
		}
	}
	var matched *remediationClaim
	matches := 0
	for index := range claims {
		if remediationClaimMatches(claims[index], expectedRevision, binding) {
			matches++
			matched = &claims[index]
		}
	}
	if matches != 1 || matched == nil || !matched.Complete || terminal == nil || matched.EndLine != terminal.EndLine {
		return remediationResultEvaluation{}
	}
	return remediationResultEvaluation{Complete: true, EvidenceRevision: expectedRevision}
}

func remediationClaimMatches(claim remediationClaim, expectedRevision string, binding *RemediationBinding) bool {
	if claim.Revision != expectedRevision {
		return false
	}
	if binding != nil && (claim.LineageID != binding.LineageID || claim.Generation != binding.Generation || claim.FixBatch != binding.FixBatch) {
		return false
	}
	return true
}

// scanRemediationClaims walks a cumulative apply-progress document and returns
// every remediation identity claim it can locate, in document order. Earlier
// remediation history with a different identity is kept as a claim so a later
// duplicate of the current identity is recognized as ambiguous. Unrelated
// closed fences, plain prose, and inline fence markers are skipped.
func scanRemediationClaims(lines []string, expectedRevision string, binding *RemediationBinding) []remediationClaim {
	var claims []remediationClaim
	for index := 0; index < len(lines); index++ {
		marker := stripBlockquote(lines[index])
		switch {
		case marker == "```yaml":
			end, ok := fenceEnd(lines, index+1)
			if !ok {
				return claims
			}
			body := stripBlockquoteLines(lines[index+1 : end])
			claim, ok := parseRemediationResultClaim(body)
			if !ok {
				index = end
				continue
			}
			claim.EndLine = end
			evidenceStart := end + 1
			for evidenceStart < len(lines) && strings.TrimSpace(lines[evidenceStart]) == "" {
				evidenceStart++
			}
			if evidenceStart < len(lines) && stripBlockquote(lines[evidenceStart]) == "```json" {
				if evidenceEnd, ok := fenceEnd(lines, evidenceStart+1); ok {
					evaluation, valid := validateRemediationPair(body, stripBlockquoteLines(lines[evidenceStart:evidenceEnd+1]), expectedRevision, binding)
					claim.Complete = valid && evaluation.Complete
					if claim.Complete {
						claim.EndLine = evidenceEnd
						index = evidenceEnd
						claims = append(claims, claim)
						continue
					}
				}
			}
			claims = append(claims, claim)
			index = end
		case marker == "```json":
			end, ok := fenceEnd(lines, index+1)
			if !ok {
				return claims
			}
			if claim, ok := parseLegacyRemediationResultClaim(stripBlockquoteLines(lines[index+1 : end])); ok {
				claim.EndLine = end
				claims = append(claims, claim)
			}
			index = end
		case strings.HasPrefix(marker, "```"):
			// Unrelated closed fence: skip it so its content is never scanned
			// as remediation claims.
			if end, ok := fenceEnd(lines, index+1); !ok {
				return claims
			} else {
				index = end
			}
		}
	}
	return claims
}

func fenceEnd(lines []string, start int) (int, bool) {
	for index := start; index < len(lines); index++ {
		if stripBlockquote(lines[index]) == "```" {
			return index, true
		}
	}
	return 0, false
}

func stripBlockquote(line string) string {
	trimmed := strings.TrimSpace(line)
	for strings.HasPrefix(trimmed, ">") {
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
		if rest == trimmed {
			break
		}
		trimmed = rest
	}
	return trimmed
}

func stripBlockquoteLines(lines []string) []string {
	stripped := make([]string, len(lines))
	for index, line := range lines {
		stripped[index] = stripBlockquote(line)
	}
	return stripped
}

// parseRemediationResultClaim extracts only the identity an envelope asserts.
// Full semantic validation stays in validateRemediationPair.
func parseRemediationResultClaim(lines []string) (remediationClaim, bool) {
	fields, reason := parseScalarFields(lines, remediationResultFields, "remediation result")
	if reason != "" || fields["schema"] != RemediationResultSchema || fields["status"] != "complete" {
		return remediationClaim{}, false
	}
	claim := remediationClaim{Revision: fields["failed_evidence_revision"]}
	claim.LineageID = fields["lineage_id"]
	if generation, ok := parseNonnegativeInt(fields["generation"]); ok {
		claim.Generation = generation
	}
	if fixBatch, ok := parseNonnegativeInt(fields["fix_batch"]); ok {
		claim.FixBatch = fixBatch
	}
	return claim, true
}

// parseLegacyRemediationResultClaim recognizes an older JSON-encoded
// remediation result envelope. It only asserts identity; like parseRemediationResultClaim,
// it exists so a same-identity claim in that historical form is rejected as an
// ambiguous duplicate instead of being silently ignored.
func parseLegacyRemediationResultClaim(lines []string) (remediationClaim, bool) {
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	var fields map[string]any
	if !strings.HasPrefix(text, "{") || json.Unmarshal([]byte(text), &fields) != nil {
		return remediationClaim{}, false
	}
	if fields["schema"] != RemediationResultSchema || fields["status"] != "complete" {
		return remediationClaim{}, false
	}
	claim := remediationClaim{Revision: jsonString(fields["failed_evidence_revision"])}
	claim.LineageID = jsonString(fields["lineage_id"])
	if generation, ok := jsonInt(fields["generation"]); ok {
		claim.Generation = generation
	}
	if fixBatch, ok := jsonInt(fields["fix_batch"]); ok {
		claim.FixBatch = fixBatch
	}
	return claim, true
}

func jsonString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func jsonInt(value any) (int, bool) {
	switch number := value.(type) {
	case float64:
		return int(number), true
	case int:
		return number, true
	}
	return 0, false
}

// validateRemediationPair runs the strict remediation result and evidence
// semantic validation over one extracted envelope pair. It is the unchanged
// authoritative validator: the scanner only isolates the exact slices.
func validateRemediationPair(resultBody, evidenceBody []string, expectedRevision string, binding *RemediationBinding) (remediationResultEvaluation, bool) {
	fields, reason := parseScalarFields(resultBody, remediationResultFields, "remediation result")
	if reason != "" {
		return remediationResultEvaluation{}, false
	}
	revision := fields["failed_evidence_revision"]
	evaluation := remediationResultEvaluation{EvidenceRevision: revision}
	if fields["schema"] != RemediationResultSchema || fields["status"] != "complete" || revision != expectedRevision {
		return evaluation, false
	}
	if binding != nil {
		generation, generationOK := parseNonnegativeInt(fields["generation"])
		fixBatch, fixBatchOK := parseNonnegativeInt(fields["fix_batch"])
		if fields["lineage_id"] != binding.LineageID || !generationOK || generation != binding.Generation || !fixBatchOK || fixBatch != binding.FixBatch {
			return evaluation, false
		}
	}
	if fields["focused_tests"] != "passed" || fields["rollback_boundary"] != "recorded" {
		return evaluation, false
	}
	if fields["runtime_harness"] != "passed" && fields["runtime_harness"] != "not_applicable" {
		return evaluation, false
	}
	evidence, ok := parseRemediationEvidence(evidenceBody)
	if !ok || evidence.FailedEvidenceRevision != expectedRevision || len(evidence.Commands) == 0 {
		return evaluation, false
	}
	if binding != nil {
		if evidence.LineageID != binding.LineageID || evidence.Generation != binding.Generation || evidence.FixBatch != binding.FixBatch {
			return evaluation, false
		}
	}
	for _, command := range evidence.Commands {
		if command.ExitCode != 0 || !isConcreteEvidence(command.Command) || !isConcreteEvidence(command.Result) {
			return evaluation, false
		}
	}
	if evidence.RuntimeHarness.Status != fields["runtime_harness"] {
		return evaluation, false
	}
	switch evidence.RuntimeHarness.Status {
	case "passed":
		if !isConcreteEvidence(evidence.RuntimeHarness.Command) || !isConcreteEvidence(evidence.RuntimeHarness.Result) || strings.TrimSpace(evidence.RuntimeHarness.NAReason) != "" {
			return evaluation, false
		}
	case "not_applicable":
		if evidence.RuntimeHarness.Command != "" || evidence.RuntimeHarness.Result != "" || !isConcreteNAReason(evidence.RuntimeHarness.NAReason) {
			return evaluation, false
		}
	default:
		return evaluation, false
	}
	if !isConcreteEvidence(evidence.Rollback.Boundary) || !isConcreteEvidence(evidence.Rollback.Evidence) {
		return evaluation, false
	}
	evaluation.Complete = true
	return evaluation, true
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
