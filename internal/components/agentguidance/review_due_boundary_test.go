package agentguidance

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// The provider's due decision, not a task boundary or failed assessment,
// controls entry into native review. Candidate consent remains mandatory.
func TestRoutingKeepsReviewDueAuthoritative(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode} {
		t.Run(string(agent), func(t *testing.T) {
			body, err := RenderRouting(agent)
			if err != nil {
				t.Fatal(err)
			}
			for _, clause := range []string{
				"Review the pending slice only when `review_due` is true or the user explicitly asks",
				"otherwise preserve the pending boundary and proceed without starting review",
				"report the missing assessment proof; never infer `review_due` or construct a preflight transition",
				"Medium/high relays the existing candidate consent only when `review_due` is true or the user explicitly asks",
				"native review runs only on grant; a decline continues under ordinary policy",
				"Never skip an existing delivery gate",
			} {
				if !strings.Contains(body, clause) {
					t.Errorf("missing provider-owned review entry clause %q", clause)
				}
			}
			for _, retired := range []string{
				"close the open slice first, assessing and reviewing it as the slice",
				"treat the commit as due and run the preflight STATUS",
				"Medium/high relays the existing candidate consent and follows the native plan",
			} {
				if strings.Contains(body, retired) {
					t.Errorf("retains an unconditional native review route %q", retired)
				}
			}
		})
	}
}
