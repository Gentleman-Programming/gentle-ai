package sddstatus

// The report envelope is validated per schema rather than against one flat
// field list. Which fields are required, optional, or forbidden is a property
// of the declared schema, so the schema is resolved before absence is judged.
//
// Today only one schema exists and the answer is constant. The indirection is
// here because the alternative — asking for a field list before knowing which
// contract the report claims to follow — cannot express a second contract at
// all: parseScalarFields rejects unknown fields before any schema is read.

// verifyReportSharedFields are required under every schema.
var verifyReportSharedFields = []string{
	"schema", "evidence_revision", "verdict", "blockers", "critical_findings",
	"requirements", "scenarios",
}

// verifyReportExecutionFields only mean something when a command actually ran.
var verifyReportExecutionFields = []string{
	"test_command", "test_exit_code", "test_output_hash",
	"build_command", "build_exit_code", "build_output_hash",
}

// verifyReportSchemaSupported reports whether a schema string names a
// verification contract this build can read.
func verifyReportSchemaSupported(schema string) bool {
	return schema == VerifyResultSchema
}

// verifyReportAllowedFields is the union every schema can mention. Membership
// here only means a field is recognized; whether it is required, optional, or
// forbidden is decided per schema by verifyReportSchemaConstraints.
func verifyReportAllowedFields() map[string]bool {
	allowed := make(map[string]bool)
	for _, group := range [][]string{
		verifyReportSharedFields, verifyReportExecutionFields, verifyReportLegacyFields,
	} {
		for _, field := range group {
			allowed[field] = true
		}
	}
	return allowed
}

// verifyReportSchemaConstraints enforces the per-schema shape after the
// envelope has been read, returning an empty string when the fields cohere.
func verifyReportSchemaConstraints(fields map[string]string) string {
	if !verifyReportSchemaSupported(fields["schema"]) {
		return "unsupported verify result schema " + fields["schema"]
	}
	return ""
}

// verifyReportRequiredFieldsFor returns the fields a report must carry given
// its declared schema.
func verifyReportRequiredFieldsFor(map[string]string) []string {
	return append(append([]string{}, verifyReportSharedFields...), verifyReportExecutionFields...)
}
