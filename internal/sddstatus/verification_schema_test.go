package sddstatus

import (
	"strings"
	"testing"
)

const testCandidateDigest = "sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

const testNAReason = "no runtime obligation exists because every changed path is passive documentation"

// testNotApplicableEnvelope builds a v2 report that records the absence of a
// runtime obligation. It carries none of the six execution fields, because the
// contract forbids them in this state rather than accepting placeholders.
func testNotApplicableEnvelope(blockers, critical int, requirements, scenarios string) string {
	return strings.Join([]string{
		"```yaml",
		"schema: " + VerifyResultSchemaV2,
		"evidence_revision: sha256:" + strings.Repeat("a", 64),
		"verdict: " + VerifyVerdictNotApplicable,
		"blockers: " + itoa(blockers),
		"critical_findings: " + itoa(critical),
		"requirements: " + requirements,
		"scenarios: " + scenarios,
		"candidate_digest: " + testCandidateDigest,
		"na_reason: " + testNAReason,
		"```",
	}, "\n")
}

// TestNotApplicableVerifyReportNeverFabricatesExecutionEvidence is the point of
// the v2 bump: the state that skips execution must refuse the execution fields,
// not fill them with zeros and invented hashes.
func TestNotApplicableVerifyReportNeverFabricatesExecutionEvidence(t *testing.T) {
	valid := testNotApplicableEnvelope(0, 0, "0/2", "0/3")
	expected := SpecCounts{Requirements: 2, Scenarios: 3}
	tests := []struct {
		name       string
		report     string
		wantPass   bool
		wantReason string
	}{
		{name: "passive candidate satisfies verification", report: valid, wantPass: true},
		{
			name:       "execution evidence is forbidden, not optional",
			report:     strings.Replace(valid, "candidate_digest: ", "test_exit_code: 0\ncandidate_digest: ", 1),
			wantReason: "forbids execution evidence field test_exit_code",
		},
		{
			name:       "a fabricated command is refused alongside it",
			report:     strings.Replace(valid, "candidate_digest: ", "test_command: go test ./...\ncandidate_digest: ", 1),
			wantReason: "forbids execution evidence field test_command",
		},
		{
			name:       "the candidate binding is mandatory",
			report:     strings.Replace(valid, "candidate_digest: "+testCandidateDigest+"\n", "", 1),
			wantReason: "missing candidate_digest",
		},
		{
			name:       "a malformed candidate binding fails closed",
			report:     strings.Replace(valid, testCandidateDigest, "not-a-digest", 1),
			wantReason: "invalid candidate_digest",
		},
		{
			name:       "the reason must be concrete",
			report:     strings.Replace(valid, testNAReason, "n/a", 1),
			wantReason: "na_reason requires a concrete explanation",
		},
		{
			name:       "prose without a cause is not a reason",
			report:     strings.Replace(valid, testNAReason, "nothing needed to run here at all", 1),
			wantReason: "na_reason requires a concrete explanation",
		},
		{
			name:       "no obligation does not license shipping blockers",
			report:     testNotApplicableEnvelope(1, 0, "0/2", "0/3"),
			wantReason: "blockers must be zero",
		},
		{
			name:       "claiming coverage contradicts assessing nothing",
			report:     testNotApplicableEnvelope(0, 0, "2/2", "0/3"),
			wantReason: "must report requirements as none assessed",
		},
		{
			name:       "the totals still have to match the specs",
			report:     testNotApplicableEnvelope(0, 0, "0/1", "0/3"),
			wantReason: "actual requirement count",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseVerifyResult(tt.report, expected)
			if got.Passing != tt.wantPass {
				t.Fatalf("Passing = %v, want %v (reason %q)", got.Passing, tt.wantPass, got.Reason)
			}
			if tt.wantReason != "" && !strings.Contains(got.Reason, tt.wantReason) {
				t.Fatalf("Reason = %q, want containing %q", got.Reason, tt.wantReason)
			}
		})
	}
}

// TestVerifyReportSchemasStaySeparate keeps each schema honest about what it
// can express, in both directions.
func TestVerifyReportSchemasStaySeparate(t *testing.T) {
	expected := SpecCounts{Requirements: 2, Scenarios: 3}
	v1 := testVerifyEnvelope("pass", 0, 0, "2/2", "3/3", 0, 0)
	tests := []struct {
		name, report, wantReason string
	}{
		{
			name:       "v1 cannot borrow the v2 state",
			report:     strings.Replace(v1, "verdict: pass", "verdict: "+VerifyVerdictNotApplicable, 1),
			wantReason: "not_applicable requires " + VerifyResultSchemaV2,
		},
		{
			name:       "v1 cannot carry the applicability fields",
			report:     strings.Replace(v1, "verdict: pass", "verdict: pass\ncandidate_digest: "+testCandidateDigest, 1),
			wantReason: "field candidate_digest requires " + VerifyResultSchemaV2,
		},
		{
			name: "an executed v2 report cannot substitute a reason for evidence",
			report: strings.Replace(
				strings.Replace(v1, VerifyResultSchema, VerifyResultSchemaV2, 1),
				"verdict: pass", "verdict: pass\nna_reason: "+testNAReason, 1),
			wantReason: "field na_reason requires the not_applicable verdict",
		},
		{
			name:       "an unknown schema names itself",
			report:     strings.Replace(v1, VerifyResultSchema, "gentle-ai.verify-result/v9", 1),
			wantReason: "unsupported verify result schema gentle-ai.verify-result/v9",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseVerifyResult(tt.report, expected)
			if got.Passing {
				t.Fatal("report passed when it should have been refused")
			}
			if !strings.Contains(got.Reason, tt.wantReason) {
				t.Fatalf("Reason = %q, want containing %q", got.Reason, tt.wantReason)
			}
		})
	}
}

// TestExecutedV2ReportBehavesLikeV1 proves the bump did not fork the ordinary
// path: a v2 report that did run commands is judged exactly as v1 is.
func TestExecutedV2ReportBehavesLikeV1(t *testing.T) {
	expected := SpecCounts{Requirements: 2, Scenarios: 3}
	executed := strings.Replace(testVerifyEnvelope("pass", 0, 0, "2/2", "3/3", 0, 0), VerifyResultSchema, VerifyResultSchemaV2, 1)
	if got := parseVerifyResult(executed, expected); !got.Passing {
		t.Fatalf("executed v2 report did not pass: %q", got.Reason)
	}
	failing := strings.Replace(testVerifyEnvelope("pass", 0, 0, "2/2", "3/3", 1, 0), VerifyResultSchema, VerifyResultSchemaV2, 1)
	if got := parseVerifyResult(failing, expected); got.Passing || !strings.Contains(got.Reason, "test_exit_code") {
		t.Fatalf("executed v2 report ignored a failing exit code: %+v", got)
	}
}

// TestNotApplicableAdmissionRefusesContradictions covers the pre-persistence
// decision, which a caller reaches through gentle-ai sdd-verify-validate.
func TestNotApplicableAdmissionRefusesContradictions(t *testing.T) {
	expected := SpecCounts{Requirements: 2, Scenarios: 3}
	if got := ValidateVerifyReportAdmission(testNotApplicableEnvelope(0, 0, "0/2", "0/3"), expected); !got.Valid {
		t.Fatalf("valid not_applicable report was refused: %q", got.Reason)
	}
	if got := ValidateVerifyReportAdmission(testNotApplicableEnvelope(0, 2, "0/2", "0/3"), expected); got.Valid ||
		!strings.Contains(got.Reason, "not_applicable contradicts") {
		t.Fatalf("not_applicable with critical findings was admitted: %+v", got)
	}
	if got := ValidateVerifyReportAdmission(testNotApplicableEnvelope(0, 0, "2/2", "0/3"), expected); got.Valid ||
		!strings.Contains(got.Reason, "none assessed") {
		t.Fatalf("not_applicable claiming coverage was admitted: %+v", got)
	}
}
