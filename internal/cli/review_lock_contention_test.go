package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// startLowRiskFacadeReview creates a zero-lens candidate. START closes it
// immediately, so callers assert only the resulting burned authority.
func startLowRiskFacadeReview(t *testing.T, repo string) string {
	t.Helper()
	lines := make([]string, 129)
	for index := range lines {
		lines[index] = fmt.Sprintf("ordinary documentation line %03d", index+1)
	}
	writeReviewStartCandidate(t, repo, "docs/ordinary-guide.md", strings.Join(lines, "\n")+"\n", 0o644)
	var output bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
	}), &output); err != nil {
		t.Fatalf("low-risk START: %v\n%s", err, output.String())
	}
	var started ReviewIntegrationStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	if started.RiskLevel != reviewtransaction.RiskLow || started.State != reviewtransaction.StateApproved || started.Action != "closed" {
		t.Fatalf("low-risk START = %#v", started)
	}
	return started.LineageID
}
