package agentguidance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// S1-S2: public renderer checks across the entire supported runtime catalog.
func TestVerifierReviewDueContract(t *testing.T) {
	for _, a := range catalog.AllAgents() {
		t.Run(string(a.ID), func(t *testing.T) {
			text, err := agentguidance.RenderRouting(a.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !model.SupportsReceiptDrivenDevelopment(a.ID) {
				for _, bad := range []string{"review_due", "gentle-ai review mode", "Medium/high relays"} {
					if strings.Contains(text, bad) {
						t.Errorf("non-RDD guidance carries %q", bad)
					}
				}
				return
			}
			for _, want := range []string{
				"When `review_due` is true (`high_risk`), or when the user explicitly asks for a review",
				"When `review_due` is false, record `review_due_reason` and continue",
				"`under_budget` stays pending in the slice until a later commit reaches the budget",
				"assess the open slice first. Review the pending slice only when `review_due` is true or the user explicitly asks; otherwise preserve the pending boundary and proceed without starting review",
				"A task boundary neither authorizes review nor advances a reviewed boundary",
				"report the missing assessment proof; never infer `review_due` or construct a preflight transition",
				"Medium/high relays the existing candidate consent only when `review_due` is true or the user explicitly asks",
				"native review runs only on grant; a decline continues under ordinary policy",
				"Follow native continuations and authority without bypassing gates",
				"When RDD is disabled, do not start or prompt for RDD; ordinary checks remain",
				"Candidate consent remains separate from the mode default",
				"Never toggle the mode automatically",
			} {
				if !strings.Contains(text, want) {
					t.Errorf("missing %q", want)
				}
			}
			for _, bad := range []string{"close the open slice first, assessing and reviewing", "so the high-risk commit is then reviewed alone against the new boundary", "treat the commit as due", "Medium/high relays the existing candidate consent and follows"} {
				if strings.Contains(text, bad) {
					t.Errorf("unconditional review route %q", bad)
				}
			}
		})
	}
}

// S1-S2: real public injection into markdown, JSON and module carriers;
// dump is optional scratch-only differential evidence, not a fixed-commit test.
func TestVerifierReviewDueCarriers(t *testing.T) {
	snapshots := map[string]string{}
	for _, a := range catalog.AllAgents() {
		t.Run(string(a.ID), func(t *testing.T) {
			route, err := agentguidance.RenderRouting(a.ID)
			if err != nil {
				t.Fatal(err)
			}
			snapshots["routing/"+string(a.ID)] = route
			for _, capability := range []string{"capable", "small"} {
				if a.ID == model.AgentPi {
					continue
				}
				body, err := agentguidance.RenderOrchestratorWithSource(a.ID, reviewassets.ReviewExecutionContractFor, capability)
				if err != nil {
					t.Fatal(err)
				}
				snapshots["orchestrator/"+string(a.ID)+"/"+capability] = body
			}
			for _, modules := range []bool{false, true} {
				if modules && a.ID != model.AgentClaudeCode {
					continue
				}
				root := t.TempDir()
				options := agentguidance.RoutingOptions{ReviewContract: reviewassets.ReviewExecutionContractFor, ClaudeGlobalModules: modules}
				result, err := agentguidance.InjectRoutingWithOptions(root, a.ID, options)
				if err != nil {
					t.Fatal(err)
				}
				carrier := ""
				for _, path := range result.Files {
					raw, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					body := string(raw)
					if strings.HasSuffix(path, ".json") || strings.HasSuffix(path, ".jsonc") {
						var doc map[string]any
						if json.Unmarshal(raw, &doc) == nil {
							if agents, ok := doc["agent"].(map[string]any); ok {
								if orchestrator, ok := agents["gentle-orchestrator"].(map[string]any); ok {
									if prompt, ok := orchestrator["prompt"].(string); ok {
										carrier += prompt
									}
								}
							}
						}
					} else {
						carrier += body
					}
					rel, err := filepath.Rel(root, path)
					if err != nil {
						t.Fatal(err)
					}
					variant := "default"
					if modules {
						variant = "modules"
					}
					snapshots["installed/"+string(a.ID)+"/"+variant+"/"+rel] = strings.ReplaceAll(body, root, "<ROOT>")
				}
				if a.ID == model.AgentConductor {
					if len(result.Files) != 0 {
						t.Fatal("catalog-only runtime wrote guidance")
					}
					continue
				}
				if !strings.Contains(carrier, route) {
					t.Fatal("actual carrier does not contain exact public routing output")
				}
				again, err := agentguidance.InjectRoutingWithOptions(root, a.ID, options)
				if err != nil {
					t.Fatal(err)
				}
				if again.Changed {
					t.Fatal("second injection changed the carrier")
				}
			}
		})
	}
	if output := os.Getenv("VERIFIER_DUMP"); output != "" {
		data, err := json.MarshalIndent(snapshots, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(output, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

// S1-S2: unchanged Pi entry contract still owns facade authority and consent.
func TestVerifierReviewDuePiLedger(t *testing.T) {
	text := assets.MustRead("skills/_shared/review-ledger-contract-pi.md")
	for _, want := range []string{
		"only when its `reviewDue` is true (high risk) or the user explicitly asks for a review of this candidate; otherwise do not start one",
		"The facade returns the only offered START route; do not infer, reconstruct, or replace it",
		"Global RDD enabled permits review; it never grants consent for this candidate",
		"Submit exactly the human's choice",
		"a decline is candidate-scoped and is not the kill switch",
		"Only the exact provider-issued acknowledgement continuation burns approved authority",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing Pi ledger clause %q", want)
		}
	}
}
