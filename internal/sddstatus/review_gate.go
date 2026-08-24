package sddstatus

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
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

const incompatibleReviewTransactionReason = "bounded review transaction artifact is not a native JSON review transaction; regenerate it from native review authority"

func readReviewTransaction(path, content string) (*reviewtransaction.Transaction, string) {
	if path == "" && strings.TrimSpace(content) == "" {
		return nil, "bounded review transaction is missing"
	}
	payload := []byte(content)
	if path != "" {
		read, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Sprintf("bounded review transaction cannot be read: %v", err)
		}
		payload = read
	}
	if !strings.HasPrefix(strings.TrimSpace(string(payload)), "{") {
		if json.Valid(payload) {
			return nil, "bounded review transaction is invalid: native review transaction must be a JSON object"
		}
		return nil, incompatibleReviewTransactionReason
	}
	transaction, err := reviewtransaction.ParseTransaction(payload)
	if err != nil {
		return nil, fmt.Sprintf("bounded review transaction is invalid: %v", err)
	}
	return &transaction, ""
}

// resolveBoundedRemediation keeps independent SDD verification failures on
// the runtime attempt path. Receipt or review-gate state has no role in this
// decision.
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
	evaluation := parseRemediationResult(applyProgress, verify.EvidenceRevision, RemediationBinding{})
	state.Complete = evaluation.Complete
	state.Required = !evaluation.Complete
	if state.Complete {
		state.Reason = ""
	}
	return state
}
