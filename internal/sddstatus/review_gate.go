package sddstatus

import (
	"fmt"
	"os"
	"strings"
)

func readSpecCounts(paths []string) (SpecCounts, error) {
	contents := make([]string, 0, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return SpecCounts{}, err
		}
		contents = append(contents, string(content))
	}
	return countSpecRequirementsAndScenarios(contents), nil
}

func readVerifyResult(path string, counts SpecCounts) (verifyResultEvaluation, error) {
	if path == "" {
		return verifyResultEvaluation{Reason: "verify result is missing"}, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return verifyResultEvaluation{}, err
	}
	return parseVerifyResult(string(content), counts), nil
}

func readText(path string) string {
	if path == "" {
		return ""
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(content)
}

// resolveBoundedRemediation preserves failed-evidence truth without importing
// review authority. The deferred runtime codecs still validate any historical
// evidence they own; status only reports whether ordinary SDD evidence requires
// or completed remediation.
func resolveBoundedRemediation(required bool, verify verifyResultEvaluation, applyProgress string) RemediationState {
	if !required {
		return RemediationState{}
	}
	if verify.EvidenceRevision == "" && strings.Contains(verify.Reason, "evidence_revision") {
		return RemediationState{Reason: fmt.Sprintf("verify evidence cannot enter remediation: %s", verify.Reason)}
	}
	state := RemediationState{
		Required:               true,
		FailedEvidenceRevision: verify.EvidenceRevision,
		Reason:                 fmt.Sprintf("verify evidence requires independent SDD remediation for %s: %s", verify.EvidenceRevision, verify.Reason),
	}
	evaluation := parseRemediationResult(applyProgress, verify.EvidenceRevision)
	state.Complete = evaluation.Complete
	state.Required = !evaluation.Complete
	if state.Complete {
		state.Reason = ""
	}
	return state
}
