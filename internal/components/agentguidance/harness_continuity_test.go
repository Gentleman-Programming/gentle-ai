package agentguidance_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v5/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v5/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v5/internal/model"
)

// harness-continuity S3: an authorized outcome continues. A pending child is a
// wait event that resumes the turn on completion, the user's explicit stop
// bounds the work it applies to even when nothing is tracked, and only a real
// human decision, a denial, missing consent, or an exhausted limit stops the
// rest. Missing proof forbids a completion claim, never an authorized next step
// and never a required check the existing budget still allows. These are
// shipped-policy checks through the public render, not model-compliance or
// runtime-telemetry proof.
const (
	continueAuthorizedWorkRule = "Run authorized in-scope steps. Work through failures with a permitted route. A pending child is a wait: completion resumes the turn without asking. Only a real human decision, explicit user stop, denial, missing consent, exhausted limit, or failure without a permitted route stops only affected work."
	trackedTaskRule            = "A tracked task ends the turn pending only for launched work still running, a real human decision, denial, missing consent, explicit stop, or exhausted limit. Report failures without a permitted route as incomplete. Continue independent authorized work; never abandon runnable work or present an unverified outcome as final."
	missingProofRule           = "Missing, partial, unavailable, or declined proof bars claiming completion of affected work, not independent authorized work or required authorized checks in budget. Stop checks only at an exhausted or denied boundary or with no permitted route. Report the exact blocker and any genuine human decision; never invent a pass."
	failedCheckRule            = "A failed check is diagnosed, fixed and rerun within its limits. An applicable quick check runs once"
	noSelfRepromptRule         = "Never turn a report into a blind self-reprompt loop."

	// The retired wording: a universal "never more verification" and the
	// generic terminal license. Both survive in no routing block.
	retiredNoMoreVerification = "never more verification"
	genericDecisionLicense    = "**Needs your decision**"
)

// continuitySpec is one S3 rule the routing block must state, with the clauses
// that pin the exact boundary it draws.
type continuitySpec struct {
	name     string
	required []string
}

var continuitySpecs = []continuitySpec{
	{
		name:     "S3 authorized work continues",
		required: []string{continueAuthorizedWorkRule},
	},
	{
		name: "S3 pending child resumes without the human",
		required: []string{
			"A pending child is a wait: completion resumes the turn without asking.",
			"launched work still running",
		},
	},
	{
		name: "S3 tracked task may wait, never abandon or fake delivery",
		required: []string{
			trackedTaskRule,
			"never abandon runnable work or present an unverified outcome as final",
		},
	},
	{
		// A decided denial or an exhausted limit is not "still outstanding":
		// the tracked rule has to allow it as a pending state of its own.
		name: "S3 tracked task includes decided denial and exhausted boundaries",
		required: []string{
			"denial, missing consent, explicit stop, or exhausted limit",
			"Continue independent authorized work",
		},
	},
	{
		// A partial explicit stop bounds only the work it covers.
		name: "S3 partial explicit stop is scoped to the work it covers",
		required: []string{
			"explicit user stop",
			"stops only affected work",
		},
	},
	{
		name: "S10 only listed reasons stop work and only the affected work",
		required: []string{
			"Work through failures with a permitted route.",
			"Only a real human decision, explicit user stop, denial, missing consent, exhausted limit, or failure without a permitted route stops only affected work.",
		},
	},
	{
		name: "S3 missing proof never claims completion",
		required: []string{
			missingProofRule,
			noSelfRepromptRule,
			"never invent a pass",
		},
	},
	{
		name: "S3 required authorized checks keep running in budget",
		required: []string{
			"required authorized checks in budget",
			"Stop checks only at an exhausted or denied boundary or with no permitted route.",
		},
	},
}

// trackedTaskRulePrefix and trackedTaskRuleSuffix bound the tracked-task rule
// so a test can assert its decisions instead of only a new string somewhere.
const (
	trackedTaskRulePrefix = "A tracked task ends the turn pending"
	trackedTaskRuleSuffix = "as final."
)

// trackedPendingRule returns the exact tracked-task rule from a rendered
// routing block. The prefix is checked before slicing so a missing rule fails
// the assertion instead of panicking on a negative index.
func trackedPendingRule(t *testing.T, agent model.AgentID, rendered string) string {
	t.Helper()
	start := strings.Index(rendered, trackedTaskRulePrefix)
	if start < 0 {
		t.Fatalf("RenderRouting(%q) has no tracked-task pending rule", agent)
	}
	rest := rendered[start:]
	end := strings.Index(rest, trackedTaskRuleSuffix)
	if end < 0 {
		t.Fatalf("RenderRouting(%q) tracked-task rule does not end with %q", agent, trackedTaskRuleSuffix)
	}
	return rest[:end+len(trackedTaskRuleSuffix)]
}

// TestRenderRoutingTrackedPendingRuleKeepsEveryLegalPendingState proves the
// tracked-task boundary names every state that may legitimately leave a tracked
// task pending, scoped to the affected work, while independent authorized work
// keeps running. Real cases behind each clause: a denied destructive migration
// stays pending without a retry, an exhausted verifier or correction limit does
// not resume the blocked task, and a partial explicit stop bounds only the work
// it covers. Shipped-policy text, not model-compliance proof.
func TestRenderRoutingTrackedPendingRuleKeepsEveryLegalPendingState(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		want []string
	}{
		{"denied destructive migration keeps the tracked task pending without retry", []string{"denial", "Continue independent authorized work", "never abandon runnable work"}},
		{"exhausted verifier or correction limit does not resume the blocked task", []string{"exhausted limit", "Continue independent authorized work"}},
		{"missing consent keeps only the unconsented work pending", []string{"missing consent", "Continue independent authorized work"}},
		{"partial explicit stop bounds only the work it covers", []string{"explicit stop", "Continue independent authorized work"}},
		{"unrelated runnable task continues beside the pending one", []string{"Continue independent authorized work"}},
		{"waiting on launched work still permits the pending turn", []string{"launched work still running"}},
		{"an outstanding human decision stays its own boundary", []string{"a real human decision"}},
		{"a failure without a permitted route is reported incomplete", []string{"Report failures without a permitted route as incomplete."}},
		{"no abandonment and no fake final delivery", []string{"never abandon runnable work", "present an unverified outcome as final"}},
	}

	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := agentguidance.RenderRouting(agent.ID)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
			}
			rule := trackedPendingRule(t, agent.ID, rendered)
			for _, tt := range cases {
				for _, want := range tt.want {
					if !strings.Contains(rule, want) {
						t.Errorf("%s: tracked-task rule is missing %q; rule = %q", tt.name, want, rule)
					}
				}
			}
		})
	}
}

// TestRenderRoutingContinuesAuthorizedWork proves the ODD close of every
// runtime states the continuity canon instead of a generic decision license.
func TestRenderRoutingContinuesAuthorizedWork(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := agentguidance.RenderRouting(agent.ID)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
			}
			for _, spec := range continuitySpecs {
				for _, want := range spec.required {
					if !strings.Contains(rendered, want) {
						t.Errorf("%s: RenderRouting(%q) is missing %q", spec.name, agent.ID, want)
					}
				}
			}
			for _, retired := range []string{genericDecisionLicense, retiredNoMoreVerification} {
				if strings.Contains(rendered, retired) {
					t.Errorf("RenderRouting(%q) keeps retired %q", agent.ID, retired)
				}
			}
			// The hedged-wording rule keeps its meaning: a user who may stop
			// or resume later asks for notes and separate commits, not a stop.
			if !strings.Contains(rendered, "Hedged wording is not a stop") {
				t.Errorf("RenderRouting(%q) lost the hedged-wording rule", agent.ID)
			}
			// PRESERVE: an unavailable verifier is still reported unavailable on
			// the runtime that names it, never retried into ceremony.
			if agent.ID == model.AgentPi && !strings.Contains(rendered, failedCheckRule+"; an unavailable verifier or subagent is reported as unavailable, never retried") {
				t.Errorf("RenderRouting(%q) does not distinguish failed checks from unavailable capabilities", agent.ID)
			}
		})
	}
}

// nativeCheckingClosePointer is the one line the Native Checking Contract keeps
// for continuity and missing proof: the canon itself lives once in the ODD
// Close of the routing block installed beside every orchestrator, so the
// always-on guidance does not carry it twice.
const nativeCheckingClosePointer = "Continuity and missing proof follow the ODD Close (step 7)."

// TestInstalledNativeCheckingContinuesAuthorizedWork proves the S3 canon still
// governs checking on every runtime, RDD or not: the Native Checking Contract
// points at the ODD Close instead of restating it, and that Close, rendered for
// the same runtime, states every continuity and missing-proof rule. Delivery of
// both into the same installed guidance is pinned by
// TestInstalledNativeCheckingPointsAtDeliveredClose.
func TestInstalledNativeCheckingContinuesAuthorizedWork(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		if agent.ID == model.AgentPi || agent.ID == model.AgentConductor {
			// Pi's prompt is owned by Gentle Shell; Conductor is catalog-only.
			continue
		}
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := agentguidance.RenderOrchestratorWithSource(agent.ID, reviewassets.ReviewExecutionContractFor, "")
			if err != nil {
				t.Fatalf("RenderOrchestratorWithSource(%q) error = %v", agent.ID, err)
			}
			section := nativeCheckingContractSection(rendered)
			if section == "" {
				t.Fatalf("RenderOrchestratorWithSource(%q) has no Native Checking Contract", agent.ID)
			}
			if !strings.Contains(section, nativeCheckingClosePointer) {
				t.Errorf("installed %q Native Checking Contract does not point at the ODD Close", agent.ID)
			}
			routing, err := agentguidance.RenderRouting(agent.ID)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
			}
			for _, want := range []string{
				continueAuthorizedWorkRule,
				trackedTaskRule,
				missingProofRule,
				noSelfRepromptRule,
				"Stop checks only at an exhausted or denied boundary or with no permitted route.",
				"never invent a pass",
			} {
				if !strings.Contains(routing, want) {
					t.Errorf("ODD Close the %q Native Checking Contract points at is missing %q", agent.ID, want)
				}
			}
			if strings.Contains(section, genericDecisionLicense) {
				t.Errorf("installed %q Native Checking Contract still licenses a generic stop", agent.ID)
			}
			// PRESERVE: an unavailable verifier is still reported unavailable,
			// never retried into extra ceremony, and never invented as a pass.
			for _, want := range []string{failedCheckRule, "an applicable verifier is unavailable", "retry indefinitely, or escalate into extra ceremony"} {
				if !strings.Contains(section, want) {
					t.Errorf("installed %q Native Checking Contract lost %q", agent.ID, want)
				}
			}
		})
	}
}

// nativeCheckingContractSection returns the Native Checking Contract body of a
// rendered orchestrator, up to the next heading of the same or a higher level.
func nativeCheckingContractSection(content string) string {
	const heading = "#### Native Checking Contract"
	start := strings.Index(content, heading)
	if start < 0 {
		return ""
	}
	rest := content[start+len(heading):]
	end := len(rest)
	for _, next := range []string{"\n#### ", "\n### ", "\n## "} {
		if at := strings.Index(rest, next); at >= 0 && at < end {
			end = at
		}
	}
	return rest[:end]
}
