package sddstatus

// ParseEvidenceEnvelope exposes the internal envelope parser for external SDD packages
// (e.g., qastage) that need to extract fenced YAML frontmatter from artifacts.
func ParseEvidenceEnvelope(text, schema, label string) ([]string, int, string) {
	return parseLeadingEnvelopeLabeled(text, schema, label)
}

// ParseScalarFields exposes the internal scalar parser for external SDD packages.
func ParseScalarFields(lines []string, allowed map[string]bool, label string) (map[string]string, string) {
	return parseScalarFields(lines, allowed, label)
}

// IsConcreteEvidence exposes the internal concrete evidence heuristic.
func IsConcreteEvidence(value string) bool {
	return isConcreteEvidence(value)
}

// IsSHA256Identity exposes the internal SHA256 format check.
func IsSHA256Identity(value string) bool {
	return sha256IdentityPattern.MatchString(value)
}
