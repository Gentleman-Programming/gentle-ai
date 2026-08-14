package sddstatus

import (
	"encoding/json"
	"strconv"
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

func TestParseRemediationResultRequiresExactTransactionBinding(t *testing.T) {
	revision := "sha256:" + strings.Repeat("d", 64)
	binding := RemediationBinding{LineageID: "lineage-1", Generation: 2, FixBatch: 1}
	evidence := remediationResultEvidenceWithBinding(revision, binding)
	if got := parseRemediationResult(evidence, revision, binding); !got.Complete {
		t.Fatal("exact transaction-bound remediation evidence did not pass")
	}
	stale := binding
	stale.Generation++
	if got := parseRemediationResult(evidence, revision, stale); got.Complete {
		t.Fatal("remediation evidence passed for a different transaction generation")
	}
}

func TestParseRemediationResultCumulativeProgress(t *testing.T) {
	revision := "sha256:" + strings.Repeat("d", 64)
	binding := RemediationBinding{LineageID: "lineage-1", Generation: 2, FixBatch: 2}

	otherRevision := "sha256:" + strings.Repeat("e", 64)
	type parseCase struct {
		name, text string
		want       bool
	}
	cases := []parseCase{
		{
			name: "accepts earlier history with different revision",
			text: remediationResultEvidenceWithBinding(otherRevision, binding) + "\n\n" + remediationResultEvidenceWithBinding(revision, binding),
			want: true,
		},
		{
			name: "rejects non-adjacent evidence",
			text: strings.Replace(remediationResultEvidenceWithBinding(revision, binding), "\n```json\n", "\nprogress prose\n```json\n", 1),
		},
		{
			name: "rejects invalid trailing content",
			text: remediationResultEvidenceWithBinding(revision, binding) + "\ntrailing prose",
		},
		{
			name: "rejects trailing prose with an inline fence marker",
			text: strings.Join([]string{
				remediationResultEvidenceWithBinding(revision, binding),
				"trailing prose with a ```yaml marker",
			}, "\n"),
			want: false,
		},
		{
			name: "allows a closed bare fence after the valid pair",
			text: remediationResultEvidenceWithBinding(revision, binding) + "\n```\nunrelated content\n```",
			want: true,
		},
		{
			name: "rejects a pair inside an unclosed bare fence",
			text: "```\n" + remediationResultEvidenceWithBinding(revision, binding),
		},
	}
	for _, tc := range []struct {
		name     string
		revision string
		binding  RemediationBinding
	}{
		{"different lineage", revision, RemediationBinding{LineageID: "lineage-2", Generation: binding.Generation, FixBatch: binding.FixBatch}},
		{"different generation", revision, RemediationBinding{LineageID: binding.LineageID, Generation: binding.Generation + 1, FixBatch: binding.FixBatch}},
		{"different fix batch", revision, RemediationBinding{LineageID: binding.LineageID, Generation: binding.Generation, FixBatch: binding.FixBatch + 1}},
	} {
		cases = append(cases, parseCase{"accepts earlier history with " + tc.name, remediationResultEvidenceWithBinding(tc.revision, tc.binding) + "\n\n" + remediationResultEvidenceWithBinding(revision, binding), true})
	}
	for _, tc := range []struct {
		name, text string
	}{
		{"strict pair", remediationResultEvidenceWithBinding(revision, binding)},
		{"legacy json result", legacyRemediationResult(revision, binding)},
		{"blockquoted result", quoteRemediationEnvelope(remediationEnvelopeWithBinding(revision, binding), "> ")},
		{"nested blockquoted result", quoteRemediationEnvelope(remediationEnvelopeWithBinding(revision, binding), "> > ")},
	} {
		cases = append(cases, parseCase{"rejects duplicate " + tc.name, tc.text + "\n\n" + remediationResultEvidenceWithBinding(revision, binding), false})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRemediationResult(tc.text, revision, binding).Complete
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

func TestParseRemediationResultMultipleBindingsPreserveEvidenceRevision(t *testing.T) {
	revision := "sha256:" + strings.Repeat("d", 64)
	binding := RemediationBinding{LineageID: "lineage-1", Generation: 2, FixBatch: 2}
	got := parseRemediationResult(remediationResultEvidenceWithBinding(revision, binding), revision, binding, binding)
	if got.Complete || got.EvidenceRevision != revision {
		t.Fatalf("multiple-binding result = %#v, want incomplete result retaining %q", got, revision)
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

func remediationResultEvidenceWithBinding(revision string, binding RemediationBinding) string {
	envelope := remediationEnvelopeWithBinding(revision, binding)
	payload := map[string]any{
		"schema":                   "gentle-ai.remediation-evidence/v1",
		"failed_evidence_revision": revision,
		"lineage_id":               binding.LineageID,
		"generation":               binding.Generation,
		"fix_batch":                binding.FixBatch,
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
	return envelope + "\n```json\n" + string(raw) + "\n```"
}

func remediationEnvelopeWithBinding(revision string, binding RemediationBinding) string {
	return strings.Replace(remediationEnvelope(revision), "focused_tests: passed", "lineage_id: "+binding.LineageID+"\ngeneration: "+strconv.Itoa(binding.Generation)+"\nfix_batch: "+strconv.Itoa(binding.FixBatch)+"\nfocused_tests: passed", 1)
}

func legacyRemediationResult(revision string, binding RemediationBinding) string {
	return "```json\n{\"schema\":\"gentle-ai.remediation-result/v1\",\"failedVerifyRevision\":\"" + revision + "\",\"lineageId\":\"" + binding.LineageID + "\",\"generation\":" + strconv.Itoa(binding.Generation) + ",\"fixBatch\":" + strconv.Itoa(binding.FixBatch) + "}\n```"
}

func quoteRemediationEnvelope(envelope, prefix string) string {
	return prefix + strings.ReplaceAll(envelope, "\n", "\n"+prefix)
}
