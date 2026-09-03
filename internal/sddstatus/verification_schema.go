package sddstatus

import "strings"

// VerifyResultSchemaV2 adds one state v1 cannot express: a candidate with no
// runtime obligation to execute against. v1 requires commands, exit codes, and
// output hashes on every report, so filing one for a passive candidate means
// fabricating evidence of an obligation rather than recording its absence.
//
// v2 follows the shape remediation runtime harnesses already use here: the
// state that skips execution evidence FORBIDS the execution fields instead of
// filling them, and owes a concrete reason instead. A bump rather than an
// optional field because parseScalarFields rejects unknown fields before any
// schema is read, so an added field reaches an older client as a corrupt
// report while an unknown schema names the real cause.
const VerifyResultSchemaV2 = "gentle-ai.verify-result/v2"

// VerifyVerdictNotApplicable is admissible only under v2. It asserts that the
// candidate carried no runtime obligation, never that one was discharged.
const VerifyVerdictNotApplicable = "not_applicable"

// verifyReportExecutionFields are the six fields that only mean something when
// a command actually ran. A not-applicable report must omit all of them.
var verifyReportExecutionFields = []string{
	"test_command", "test_exit_code", "test_output_hash",
	"build_command", "build_exit_code", "build_output_hash",
}

// verifyReportApplicabilityFields carry the substitute proof: candidate_digest
// binds the report to exact bytes so drift is detectable, and na_reason says
// why no execution was owed.
//
// It is a digest of the classified content rather than the candidate's tree
// OID, because the report is stored inside that tree. A tree OID would name a
// hash fixed point: the value the report must contain would depend on
// containing it. The digest excludes the report's own path, so it is stable
// across writing the report and recomputable by a later reader.
var verifyReportApplicabilityFields = []string{"candidate_digest", "na_reason"}

// verifyReportSharedFields are required under every schema and verdict.
var verifyReportSharedFields = []string{
	"schema", "evidence_revision", "verdict", "blockers", "critical_findings",
	"requirements", "scenarios",
}

// verifyReportSchemaSupported reports whether a schema string names a
// verification contract this build can read. v1 stays readable forever:
// reports persisted by earlier releases are still the authority for their own
// changes, and rejecting them would strand completed work.
func verifyReportSchemaSupported(schema string) bool {
	return schema == VerifyResultSchema || schema == VerifyResultSchemaV2
}

// verifyReportIsNotApplicable reports whether parsed fields claim the
// not-applicable state. The verdict alone decides; a v1 report that spells the
// verdict is rejected by verifyReportSchemaConstraints, not silently accepted.
func verifyReportIsNotApplicable(fields map[string]string) bool {
	return fields["verdict"] == VerifyVerdictNotApplicable
}

// verifyReportAllowedFields is the union every schema can mention. Membership
// here only means a field is recognized; whether it is required, optional, or
// forbidden is decided per schema by verifyReportSchemaConstraints.
func verifyReportAllowedFields() map[string]bool {
	allowed := make(map[string]bool)
	for _, group := range [][]string{
		verifyReportSharedFields, verifyReportExecutionFields,
		verifyReportApplicabilityFields, verifyReportLegacyFields,
	} {
		for _, field := range group {
			allowed[field] = true
		}
	}
	return allowed
}

// verifyReportSchemaConstraints enforces the per-schema shape after the
// envelope has been read, returning an empty string when the fields cohere.
// It is strict in both directions: a v1 report may not borrow v2's state, and
// a v2 not-applicable report may not carry execution evidence it never
// produced. Either mixture makes the state mean less than it claims.
func verifyReportSchemaConstraints(fields map[string]string) string {
	schema := fields["schema"]
	if !verifyReportSchemaSupported(schema) {
		return "unsupported verify result schema " + schema
	}
	notApplicable := verifyReportIsNotApplicable(fields)
	if schema == VerifyResultSchema {
		if notApplicable {
			return "not_applicable requires " + VerifyResultSchemaV2
		}
		for _, field := range verifyReportApplicabilityFields {
			if _, present := fields[field]; present {
				return "field " + field + " requires " + VerifyResultSchemaV2
			}
		}
		return ""
	}
	if !notApplicable {
		// An executed v2 report proves itself exactly as v1 does, so it may
		// not borrow the applicability fields to stand in for execution.
		for _, field := range verifyReportApplicabilityFields {
			if _, present := fields[field]; present {
				return "field " + field + " requires the not_applicable verdict"
			}
		}
		return ""
	}
	for _, field := range verifyReportExecutionFields {
		if _, present := fields[field]; present {
			return "not_applicable forbids execution evidence field " + field
		}
	}
	for _, field := range verifyReportApplicabilityFields {
		if strings.TrimSpace(fields[field]) == "" {
			return "missing " + field + " in verify result envelope"
		}
	}
	if !sha256IdentityPattern.MatchString(fields["candidate_digest"]) {
		return "invalid candidate_digest in verify result envelope"
	}
	if !isConcreteNAReason(fields["na_reason"]) {
		return "na_reason requires a concrete explanation"
	}
	// Coverage must be reported as none assessed. A report stating that no
	// runtime obligation existed cannot also claim every requirement and
	// scenario was covered: nothing evaluated them. Forcing completed totals
	// here would move the fabrication out of the execution fields and into the
	// coverage fields, which is the same defect wearing different clothes.
	//
	// The totals themselves still have to match the specs, so the report
	// remains bound to the change it belongs to.
	for _, field := range []string{"requirements", "scenarios"} {
		completion, ok := parseVerifyCompletion(fields[field])
		if !ok {
			return "invalid " + field + " in verify result envelope"
		}
		if completion.Completed != 0 {
			return "not_applicable must report " + field + " as none assessed"
		}
	}
	return ""
}

// verifyReportRequiredFieldsFor returns the fields a report must carry given
// its declared schema and verdict.
func verifyReportRequiredFieldsFor(fields map[string]string) []string {
	required := append([]string{}, verifyReportSharedFields...)
	if fields["schema"] == VerifyResultSchemaV2 && verifyReportIsNotApplicable(fields) {
		return append(required, verifyReportApplicabilityFields...)
	}
	return append(required, verifyReportExecutionFields...)
}
