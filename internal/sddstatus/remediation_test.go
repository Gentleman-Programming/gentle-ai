package sddstatus

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseRemediationResultRejectsBareAndStaleEvidence(t *testing.T) {
	failedRevision := "sha256:" + strings.Repeat("d", 64)
	bare := remediationEnvelope(failedRevision)
	if got := parseRemediationResult(bare, failedRevision); got.Complete {
		t.Fatal("bare remediation envelope passed without command, runtime, and rollback evidence")
	}
	if got := parseRemediationResult(remediationResultEvidence(failedRevision), "sha256:"+strings.Repeat("e", 64)); got.Complete {
		t.Fatal("stale remediation evidence passed for a different failed evidence revision")
	}
	if got := parseRemediationResult(remediationResultEvidence(failedRevision), failedRevision); !got.Complete {
		t.Fatal("complete remediation evidence did not pass")
	}
}

func TestResolveBoundedRemediationRejectsHistoricalTransactionFields(t *testing.T) {
	failedRevision := "sha256:" + strings.Repeat("d", 64)
	for _, test := range []struct {
		name          string
		applyProgress string
	}{
		{name: "result envelope lineage_id", applyProgress: remediationResultEvidenceWithHistoricalEnvelopeField(failedRevision, "lineage_id", "historical-lineage")},
		{name: "result envelope generation", applyProgress: remediationResultEvidenceWithHistoricalEnvelopeField(failedRevision, "generation", "2")},
		{name: "result envelope fix_batch", applyProgress: remediationResultEvidenceWithHistoricalEnvelopeField(failedRevision, "fix_batch", "1")},
		{name: "JSON evidence lineage_id", applyProgress: remediationResultEvidenceWithHistoricalEvidenceField(failedRevision, "lineage_id", "historical-lineage")},
		{name: "JSON evidence generation", applyProgress: remediationResultEvidenceWithHistoricalEvidenceField(failedRevision, "generation", 2)},
		{name: "JSON evidence fix_batch", applyProgress: remediationResultEvidenceWithHistoricalEvidenceField(failedRevision, "fix_batch", 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			remediation := resolveBoundedRemediation(true, verifyResultEvaluation{
				EvidenceRevision: failedRevision,
				Reason:           "verification failed",
			}, test.applyProgress)
			if remediation.Complete || !remediation.Required || remediation.Reason == "" {
				t.Fatalf("historical transaction field completed authority-free remediation: %#v", remediation)
			}
		})
	}
}

func TestParseRemediationResultCumulativeProgress(t *testing.T) {
	revision := "sha256:" + strings.Repeat("d", 64)
	otherRevision := "sha256:" + strings.Repeat("e", 64)
	type parseCase struct {
		name, text string
		want       bool
	}
	cases := []parseCase{
		{
			name: "accepts earlier history with different revision",
			text: remediationResultEvidence(otherRevision) + "\n\n" + remediationResultEvidence(revision),
			want: true,
		},
		{
			name: "rejects non-adjacent evidence",
			text: strings.Replace(remediationResultEvidence(revision), "\n```json\n", "\nprogress prose\n```json\n", 1),
		},
		{
			name: "rejects invalid trailing content",
			text: remediationResultEvidence(revision) + "\ntrailing prose",
		},
		{
			name: "rejects trailing prose with an inline fence marker",
			text: strings.Join([]string{
				remediationResultEvidence(revision),
				"trailing prose with a ```yaml marker",
			}, "\n"),
			want: false,
		},
		{
			name: "allows a closed bare fence after the valid pair",
			text: remediationResultEvidence(revision) + "\n```\nunrelated content\n```",
			want: true,
		},
		{
			name: "rejects a pair inside an unclosed bare fence",
			text: "```\n" + remediationResultEvidence(revision),
		},
	}
	for _, tc := range []struct {
		name, text string
	}{
		{"strict pair", remediationResultEvidence(revision)},
		{"legacy json result", legacyRemediationResult(revision)},
		{"blockquoted result", quoteRemediationEnvelope(remediationEnvelope(revision), "> ")},
		{"nested blockquoted result", quoteRemediationEnvelope(remediationEnvelope(revision), "> > ")},
	} {
		cases = append(cases, parseCase{"rejects duplicate " + tc.name, tc.text + "\n\n" + remediationResultEvidence(revision), false})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRemediationResult(tc.text, revision).Complete
			if got != tc.want {
				t.Fatalf("parseRemediationResult(...).Complete = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseRemediationResultRejectsManyUnclosedFences(t *testing.T) {
	if got := parseRemediationResult(strings.Repeat("```yaml\n", 4096), "sha256:"+strings.Repeat("d", 64)); got.Complete {
		t.Fatal("many unclosed fences passed remediation parsing")
	}
}

func remediationEnvelope(revision string) string {
	return strings.Join([]string{
		"```yaml",
		"schema: gentle-ai.remediation-result/v1",
		"status: complete",
		"failed_evidence_revision: " + revision,
		"focused_tests: passed",
		"runtime_harness: not_applicable",
		"rollback_boundary: recorded",
		"```",
	}, "\n")
}

func remediationResultEvidence(revision string) string {
	payload := map[string]any{
		"schema":                   "gentle-ai.remediation-evidence/v1",
		"failed_evidence_revision": revision,
		"commands":                 []map[string]any{{"command": "go test ./internal/example", "exit_code": 0, "result": "1 test passed"}},
		"runtime_harness": map[string]any{
			"status": "not_applicable", "command": "", "result": "",
			"na_reason": "No runtime boundary exists because this change only tightens a report parser.",
		},
		"rollback": map[string]any{
			"boundary": "internal/sddstatus parser and focused tests",
			"evidence": "Revert those files without changing unrelated status behavior.",
		},
	}
	raw, _ := json.Marshal(payload)
	return remediationEnvelope(revision) + "\n```json\n" + string(raw) + "\n```"
}

func remediationResultEvidenceWithHistoricalEnvelopeField(revision, field, value string) string {
	return strings.Replace(remediationResultEvidence(revision), "focused_tests: passed", field+": "+value+"\nfocused_tests: passed", 1)
}

func remediationResultEvidenceWithHistoricalEvidenceField(revision, field string, value any) string {
	payload := map[string]any{
		"schema":                   "gentle-ai.remediation-evidence/v1",
		"failed_evidence_revision": revision,
		"commands":                 []map[string]any{{"command": "go test ./internal/example", "exit_code": 0, "result": "1 test passed"}},
		"runtime_harness": map[string]any{
			"status": "not_applicable", "command": "", "result": "",
			"na_reason": "No runtime boundary exists because this change only tightens a report parser.",
		},
		"rollback": map[string]any{
			"boundary": "internal/sddstatus parser and focused tests",
			"evidence": "Revert those files without changing unrelated status behavior.",
		},
	}
	payload[field] = value
	raw, _ := json.Marshal(payload)
	return remediationEnvelope(revision) + "\n```json\n" + string(raw) + "\n```"
}

func legacyRemediationResult(revision string) string {
	return "```json\n{\"schema\":\"gentle-ai.remediation-result/v1\",\"failedVerifyRevision\":\"" + revision + "\"}\n```"
}

func quoteRemediationEnvelope(envelope, prefix string) string {
	return prefix + strings.ReplaceAll(envelope, "\n", "\n"+prefix)
}
