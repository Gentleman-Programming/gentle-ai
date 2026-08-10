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

func TestParseRemediationResultCompletesFromCumulativeApplyProgress(t *testing.T) {
	revision := "sha256:" + strings.Repeat("d", 64)
	binding := RemediationBinding{LineageID: "lineage-1", Generation: 2, FixBatch: 2}
	prior := RemediationBinding{LineageID: "lineage-1", Generation: 2, FixBatch: 1}
	artifact := strings.Join([]string{
		"# Apply Progress",
		"",
		"- [x] design",
		"- [x] spec",
		"",
		remediationResultEvidenceWithBinding("sha256:"+strings.Repeat("c", 64), prior),
		"",
		"## Remediation fix batch 2",
		"",
		"```sh",
		"go test ./internal/sddstatus",
		"```",
		"",
		remediationResultEvidenceWithBinding(revision, binding),
		"",
	}, "\n")
	if got := parseRemediationResult(artifact, revision, binding); !got.Complete {
		t.Fatal("terminal remediation pair appended to cumulative apply-progress must complete remediation")
	}
}

func TestParseRemediationResultRejectsSameIdentityDuplicateInCumulativeProgress(t *testing.T) {
	revision := "sha256:" + strings.Repeat("d", 64)
	binding := RemediationBinding{LineageID: "lineage-1", Generation: 2, FixBatch: 2}
	pair := remediationResultEvidenceWithBinding(revision, binding)
	artifact := strings.Join([]string{"# Apply Progress", "", pair, "", "## retry", "", pair, ""}, "\n")
	if got := parseRemediationResult(artifact, revision, binding); got.Complete {
		t.Fatal("duplicate same-identity remediation pair must be rejected as ambiguous")
	}
}

func TestParseRemediationResultRequiresTerminalPairToCarryRequestedIdentity(t *testing.T) {
	revision := "sha256:" + strings.Repeat("d", 64)
	binding := RemediationBinding{LineageID: "lineage-1", Generation: 2, FixBatch: 2}
	newer := RemediationBinding{LineageID: "lineage-1", Generation: 2, FixBatch: 3}
	artifact := strings.Join([]string{
		"# Apply Progress",
		"",
		remediationResultEvidenceWithBinding(revision, binding),
		"",
		"## Newer remediation fix batch 3",
		"",
		remediationResultEvidenceWithBinding("sha256:"+strings.Repeat("e", 64), newer),
		"",
	}, "\n")
	if got := parseRemediationResult(artifact, revision, binding); got.Complete {
		t.Fatal("a matching pair that is not terminal must not complete remediation")
	}
}

func TestParseRemediationResultRejectsBlockquotedSameIdentityDuplicate(t *testing.T) {
	revision := "sha256:" + strings.Repeat("d", 64)
	binding := RemediationBinding{LineageID: "lineage-1", Generation: 2, FixBatch: 2}
	pair := remediationResultEvidenceWithBinding(revision, binding)
	artifact := strings.Join([]string{
		"# Apply Progress",
		"",
		quoteAsBlockquote(pair),
		"",
		remediationResultEvidenceWithBinding(revision, binding),
		"",
	}, "\n")
	if got := parseRemediationResult(artifact, revision, binding); got.Complete {
		t.Fatal("blockquoted duplicate remediation claim matching the current identity must be rejected")
	}
}

func TestParseRemediationResultRejectsLegacyJSONResultSameIdentityDuplicate(t *testing.T) {
	revision := "sha256:" + strings.Repeat("d", 64)
	binding := RemediationBinding{LineageID: "lineage-1", Generation: 2, FixBatch: 2}
	legacy := map[string]any{
		"schema":                   "gentle-ai.remediation-result/v1",
		"status":                   "complete",
		"failed_evidence_revision": revision,
		"lineage_id":               binding.LineageID,
		"generation":               binding.Generation,
		"fix_batch":                binding.FixBatch,
	}
	raw, _ := json.Marshal(legacy)
	legacyEnvelope := "```json\n" + string(raw) + "\n```"
	artifact := strings.Join([]string{
		"# Apply Progress",
		"",
		legacyEnvelope,
		"",
		remediationResultEvidenceWithBinding(revision, binding),
		"",
	}, "\n")
	if got := parseRemediationResult(artifact, revision, binding); got.Complete {
		t.Fatal("legacy JSON-result duplicate claim matching the current identity must be rejected")
	}
}

func TestParseRemediationResultSkipsInlineFenceProse(t *testing.T) {
	revision := "sha256:" + strings.Repeat("d", 64)
	binding := RemediationBinding{LineageID: "lineage-1", Generation: 2, FixBatch: 2}
	artifact := strings.Join([]string{
		"# Apply Progress",
		"",
		"```yaml is inline fence prose, not a block opener",
		"",
		remediationResultEvidenceWithBinding(revision, binding),
		"",
	}, "\n")
	if got := parseRemediationResult(artifact, revision, binding); !got.Complete {
		t.Fatal("inline fence prose at line start must not swallow the terminal remediation pair")
	}
}

func quoteAsBlockquote(text string) string {
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		lines[index] = "> " + line
	}
	return strings.Join(lines, "\n")
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
