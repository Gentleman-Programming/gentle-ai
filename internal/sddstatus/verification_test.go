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

func TestValidateVerifyReportAdmission(t *testing.T) {
	valid := testVerifyEnvelope("pass", 0, 0, "2/2", "3/3", 0, 0)
	tests := []struct {
		name, report, reason string
		valid                bool
	}{
		{"pass", valid, "", true},
		{"warning pass with CRLF and prose", strings.ReplaceAll(strings.Replace(valid, "verdict: pass", "verdict: pass_with_warnings", 1)+"\nDetails", "\n", "\r\n"), "", true},
		{"failed tests", testVerifyEnvelope("fail", 0, 0, "2/2", "3/3", 1, 0), "", true},
		{"failed build", testVerifyEnvelope("fail", 0, 0, "2/2", "3/3", 0, 1), "", true},
		{"blocker", testVerifyEnvelope("fail", 1, 0, "2/2", "3/3", 0, 0), "", true},
		{"critical", testVerifyEnvelope("fail", 0, 1, "2/2", "3/3", 0, 0), "", true},
		{"incomplete requirement", testVerifyEnvelope("fail", 0, 0, "1/2", "3/3", 0, 0), "", true},
		{"incomplete scenario", testVerifyEnvelope("fail", 0, 0, "2/2", "2/3", 0, 0), "", true},
		{"all-green failure", strings.Replace(valid, "verdict: pass", "verdict: fail", 1), "contradictory", false},
		{"passing blocker", strings.Replace(valid, "blockers: 0", "blockers: 1", 1), "contradicts", false},
		{"passing incomplete", strings.Replace(valid, "requirements: 2/2", "requirements: 1/2", 1), "contradicts", false},
		{"count mismatch", valid, "actual requirement count", false},
		{"front matter", "---\nverdict: pass\n---\n" + valid, "front matter", false},
		{"prose first", "Result follows\n" + valid, "first non-empty", false},
		// #2828: the fence contract as the CLI actually admits it.
		{"utf-8 bom before fence", "\ufeff" + valid, "", true},
		{"yml fence tag", strings.Replace(valid, "```yaml", "```yml", 1), "", true},
		{"upper-case fence tag", strings.Replace(valid, "```yaml", "```YAML", 1), "", true},
		{"untagged fence", strings.Replace(valid, "```yaml", "```", 1), "first non-empty line must be ```yaml", false},
		{"tilde fence", strings.Replace(valid, "```yaml", "~~~yaml", 1), "first non-empty line must be ```yaml", false},
		{"heading before fence", "# Verify report\n\n" + valid, "first non-empty line must be ```yaml", false},
		{"refusal names the validator", strings.Replace(valid, "```yaml", "```", 1), "gentle-ai sdd-verify-validate", false},
		{"unterminated", strings.TrimSuffix(valid, "```"), "unterminated", false},
		{"duplicate", strings.Replace(valid, "verdict: pass", "verdict: pass\nverdict: pass", 1), "duplicate", false},
		{"unknown", strings.Replace(valid, "verdict: pass", "verdict: pass\nextra: value", 1), "unknown", false},
		{"malformed", strings.Replace(valid, "blockers: 0", "blockers", 1), "malformed", false},
		{"missing", strings.Replace(valid, "build_command: go test ./cmd/gentle-ai\n", "", 1), "missing build_command", false},
		// #4089: the refusal must name the expected shape, not just the field.
		{"invalid hash", strings.Replace(valid, "sha256:"+strings.Repeat("b", 64), "sha256:nope", 1), "test_output_hash must be sha256:<64 lowercase hex>", false},
		{"invalid evidence_revision", strings.Replace(valid, "sha256:"+strings.Repeat("a", 64), "sha256:nope", 1), "evidence_revision must be sha256:<64 lowercase hex>", false},
		{"placeholder command", strings.Replace(valid, "test_command: go test ./internal/example", "test_command: placeholder", 1), "concrete", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expected := SpecCounts{Requirements: 2, Scenarios: 3}
			if tt.name == "count mismatch" {
				expected.Requirements = 3
			}
			got := ValidateVerifyReportAdmission(tt.report, expected)
			if got.Valid != tt.valid || (tt.reason != "" && !strings.Contains(got.Reason, tt.reason)) {
				t.Fatalf("admission = %#v, want valid=%v reason containing %q", got, tt.valid, tt.reason)
			}
		})
	}
}

func TestLegacyMissingReviewAuthorityIsInformationalAndIncomplete(t *testing.T) {
	report := legacyMissingReviewReport()
	admission := ValidateVerifyReportAdmission(report, SpecCounts{Requirements: 2, Scenarios: 3})
	if !admission.Valid {
		t.Fatalf("legacy report admission = %#v, want decoder compatibility", admission)
	}
	evaluation := parseVerifyResult(report, SpecCounts{Requirements: 2, Scenarios: 3})
	if evaluation.Passing || !strings.Contains(evaluation.Reason, "independent test and build execution evidence is incomplete") {
		t.Fatalf("legacy evaluation = %#v, want non-passing incomplete verification evidence", evaluation)
	}
}

func legacyMissingReviewReport() string {
	report := testVerifyEnvelope("fail", 1, 1, "0/2", "0/3", 1, 1)
	report = strings.Replace(report, "test_exit_code: 1", "test_exit_code: 125", 1)
	report = strings.Replace(report, "build_exit_code: 1", "build_exit_code: 125", 1)
	return strings.TrimSuffix(report, "```") + "missing_review_authority: true\n```"
}

func TestCountSpecRequirementsAndScenariosUsesActualArtifacts(t *testing.T) {
	tests := []struct {
		name  string
		specs []string
		want  SpecCounts
	}{
		{
			name: "canonical Requirement headings",
			specs: []string{
				"### Requirement: First\n#### Scenario: A\n#### Scenario: B\n",
				"### Requirement: Second\n#### Scenario: C\n",
			},
			want: SpecCounts{Requirements: 2, Scenarios: 3},
		},
		{
			name: "historical numeric REQ headings",
			specs: []string{
				"### REQ-1: First\n#### Scenario: A\n",
				"### REQ-12: Second\n#### Scenario: B\n",
			},
			want: SpecCounts{Requirements: 2, Scenarios: 2},
		},
		{
			name: "malformed and arbitrary headings are excluded",
			specs: []string{
				"### REQ-: Missing number\n### REQ-ABC: Not historical\n### Requirements: Plural\n### Overview: Arbitrary\n#### Scenario: Covered\n",
			},
			want: SpecCounts{Requirements: 0, Scenarios: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countSpecRequirementsAndScenarios(tt.specs); got != tt.want {
				t.Fatalf("counts = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestVerifyReportAdmissionNamesTheAcceptedSHA256FieldShape(t *testing.T) {
	valid := testVerifyEnvelope("pass", 0, 0, "2/2", "3/3", 0, 0)
	canonical := "sha256:" + strings.Repeat("a", 64)
	tests := []struct {
		name, report, want string
	}{
		{
			name:   "bare digest without the sha256 prefix",
			report: strings.Replace(valid, "evidence_revision: "+canonical, "evidence_revision: "+strings.Repeat("a", 64), 1),
			want:   "evidence_revision must be sha256:<64 lowercase hex> in verify result envelope (received length=64, sha256: prefix=false, non-lowercase-hex characters=false)",
		},
		{
			name:   "quoted value keeps its quotes",
			report: strings.Replace(valid, "evidence_revision: "+canonical, "evidence_revision: \""+canonical+"\"", 1),
			want:   "evidence_revision must be sha256:<64 lowercase hex> in verify result envelope (received length=73, sha256: prefix=false, non-lowercase-hex characters=true)",
		},
		{
			name:   "uppercase digest",
			report: strings.Replace(valid, "evidence_revision: "+canonical, "evidence_revision: sha256:"+strings.Repeat("A", 64), 1),
			want:   "evidence_revision must be sha256:<64 lowercase hex> in verify result envelope (received length=71, sha256: prefix=true, non-lowercase-hex characters=true)",
		},
		{
			name:   "output hash without the sha256 prefix",
			report: strings.Replace(valid, "test_output_hash: sha256:"+strings.Repeat("b", 64), "test_output_hash: "+strings.Repeat("b", 64), 1),
			want:   "test_output_hash must be sha256:<64 lowercase hex> in verify result envelope (received length=64, sha256: prefix=false, non-lowercase-hex characters=false)",
		},
		{
			name:   "build output hash without the sha256 prefix",
			report: strings.Replace(valid, "build_output_hash: sha256:"+strings.Repeat("c", 64), "build_output_hash: "+strings.Repeat("c", 64), 1),
			want:   "build_output_hash must be sha256:<64 lowercase hex> in verify result envelope (received length=64, sha256: prefix=false, non-lowercase-hex characters=false)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			admission := ValidateVerifyReportAdmission(tt.report, SpecCounts{Requirements: 2, Scenarios: 3})
			if admission.Valid {
				t.Fatalf("admission unexpectedly valid for %s", tt.name)
			}
			if !strings.Contains(admission.Reason, tt.want) {
				t.Fatalf("Reason = %q, want containing %q", admission.Reason, tt.want)
			}
		})
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

// TestDeriveRemediationEvidenceRevision pins #2896: a passing remediation
// settle must be able to derive its authoritative --evidence-revision from
// admitted evidence instead of requiring the caller to invent one.
func TestDeriveRemediationEvidenceRevision(t *testing.T) {
	failed := "sha256:" + strings.Repeat("a", 64)
	valid := `{"schema":"gentle-ai.remediation-evidence/v1","failed_evidence_revision":"` + failed + `",` +
		`"commands":[{"command":"go test ./...","exit_code":0,"result":"293 passed"}],` +
		`"runtime_harness":{"status":"not_applicable","na_reason":"no runtime harness because this change is test-only"},` +
		`"rollback":{"boundary":"commit 9ec76eec32","evidence":"git revert 9ec76eec32 restores the prior passing state"}}`

	t.Run("valid evidence derives a stable revision", func(t *testing.T) {
		first, err := DeriveRemediationEvidenceRevision(valid, failed)
		if err != nil {
			t.Fatal(err)
		}
		if !sha256IdentityPattern.MatchString(first) {
			t.Fatalf("derived revision %q is not sha256:<64 lowercase hex>", first)
		}
		if first == failed {
			t.Fatalf("derived revision must not equal the failed evidence it repairs")
		}
		second, err := DeriveRemediationEvidenceRevision(valid, failed)
		if err != nil || second != first {
			t.Fatalf("DeriveRemediationEvidenceRevision is not deterministic: %q vs %q (err=%v)", first, second, err)
		}
	})

	tests := []struct {
		name           string
		evidence       string
		expectedFailed string
	}{
		{name: "malformed JSON", evidence: "{not json", expectedFailed: failed},
		{name: "unknown field rejected", evidence: strings.Replace(valid, `"schema"`, `"extra":"x","schema"`, 1), expectedFailed: failed},
		{name: "wrong schema", evidence: strings.Replace(valid, "gentle-ai.remediation-evidence/v1", "gentle-ai.remediation-evidence/v2", 1), expectedFailed: failed},
		{name: "failed_evidence_revision mismatch", evidence: valid, expectedFailed: "sha256:" + strings.Repeat("b", 64)},
		{name: "no commands", evidence: strings.Replace(valid, `"commands":[{"command":"go test ./...","exit_code":0,"result":"293 passed"}]`, `"commands":[]`, 1), expectedFailed: failed},
		{name: "failing command exit code", evidence: strings.Replace(valid, `"exit_code":0`, `"exit_code":1`, 1), expectedFailed: failed},
		{name: "runtime_harness status not passed or not_applicable", evidence: strings.Replace(valid, `"status":"not_applicable"`, `"status":"skipped"`, 1), expectedFailed: failed},
		{name: "rollback boundary not concrete", evidence: strings.Replace(valid, `"boundary":"commit 9ec76eec32"`, `"boundary":"n/a"`, 1), expectedFailed: failed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DeriveRemediationEvidenceRevision(test.evidence, test.expectedFailed); err == nil {
				t.Fatalf("DeriveRemediationEvidenceRevision(%q) error = nil, want rejection", test.name)
			}
		})
	}
}
