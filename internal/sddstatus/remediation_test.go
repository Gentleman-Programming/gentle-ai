package sddstatus

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestAdmitRemediationEvidenceOwnsCanonicalRevision(t *testing.T) {
	failed := "sha256:" + strings.Repeat("a", 64)
	payload := remediationJSONPayload(failed)
	first, err := AdmitRemediationEvidence(payload, failed)
	if err != nil {
		t.Fatal(err)
	}
	secondFailed := "sha256:" + strings.Repeat("b", 64)
	second, err := AdmitRemediationEvidence(remediationJSONPayload(secondFailed), secondFailed)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || first == failed || second == failed {
		t.Fatalf("canonical revisions = %q, %q; want schema and failed revision in the identity", first, second)
	}
	unknown := append(append([]byte(nil), payload[:len(payload)-1]...), []byte(`,"unexpected":true}`)...)
	if _, err := AdmitRemediationEvidence(unknown, failed); err == nil {
		t.Fatal("unknown remediation evidence field was admitted")
	}
}

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
	} else if got.EvidenceRevision != failedRevision || got.SuccessfulEvidenceRevision == "" || got.SuccessfulEvidenceRevision == failedRevision {
		t.Fatalf("remediation revisions = %#v, want failed revision preserved and distinct provider-owned success revision", got)
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
	raw, _ := json.Marshal(remediationJSONPayloadObject(revision))
	return remediationEnvelope(revision) + "\n```json\n" + string(raw) + "\n```"
}

func remediationJSONPayload(revision string) []byte {
	raw, _ := json.Marshal(remediationJSONPayloadObject(revision))
	return raw
}

func remediationJSONPayloadObject(revision string) map[string]any {
	return map[string]any{
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
}

func remediationResultEvidenceWithBinding(revision string, binding RemediationBinding) string {
	envelope := strings.Replace(remediationEnvelope(revision), "focused_tests: passed", strings.Join([]string{
		"lineage_id: " + binding.LineageID,
		"generation: " + strconv.Itoa(binding.Generation),
		"fix_batch: " + strconv.Itoa(binding.FixBatch),
		"focused_tests: passed",
	}, "\n"), 1)
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
