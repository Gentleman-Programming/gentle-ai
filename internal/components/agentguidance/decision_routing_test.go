package agentguidance_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v5/internal/components/agentguidance"
)

// harness-continuity S4: ODD step 3 routes every decision before any question is
// written. No genuine human decision continues the authorized work; a remaining
// decision asks through the available compatible tool that can represent it, and
// text is the restricted fallback for a genuinely open clarification or a
// missing usable interface. Independent ordinary decisions batch only when the
// interface supports it, and provider-owned consent keeps its closed lossless
// protocol, so text is never its approval. These are shipped-contract checks
// through the public render, not model-adherence proof.
const (
	s4NoDecisionRule = "Continue when no human decision remains"
	s4ToolRule       = "ask with the available compatible tool"
	s4TextRule       = "text only for a genuinely open clarification or no usable compatible interface"
	s4BatchRule      = "batch independent ordinary decisions when the interface supports it"
	s4WaitRule       = "wait"
	s4ConsentRule    = "Provider consent keeps its closed lossless protocol; text is never its approval"

	s4DecisionCanon = s4NoDecisionRule + "; " + s4ToolRule + "; " + s4TextRule + "; " + s4BatchRule + "; " + s4WaitRule + ". " + s4ConsentRule + "."

	// s4DecisionDetailRule keeps the reference detail from reading as a
	// free-text-only restatement of step 3.
	s4DecisionDetailRule = "Step 3 decides whether that question is a tool ask or text."

	// The limits S4 preserves verbatim: named-uncertainty research and the single
	// scoped assumption challenge.
	s4NamedUncertaintyRule = "Recommend optional research only for a named uncertainty"
	s4ChallengeRule        = "at most one scoped read-only assumption challenge for a high-consequence unproven premise"
)

// decisionRoutingSpecs are the S4 rules every runtime receives, in the order a
// decision meets them.
var decisionRoutingSpecs = []struct {
	name     string
	required []string
}{
	{"S4 no genuine human decision continues the authorized work", []string{s4NoDecisionRule}},
	{"S4 a remaining decision asks through the available compatible tool", []string{s4ToolRule}},
	{"S4 text is the restricted fallback", []string{s4TextRule}},
	{"S4 independent ordinary decisions batch only when supported", []string{s4BatchRule}},
	{"S4 provider consent keeps its own protocol and text is never its approval", []string{s4ConsentRule}},
	{"S4 the whole canon renders as one rule", []string{s4DecisionCanon}},
	// The detail section used to read as free text only; it now defers to step 3.
	{"S4 the reference detail defers to the step 3 route", []string{s4DecisionDetailRule}},
}

func TestRenderRoutingRoutesDecisionsBeforeAsking(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := agentguidance.RenderRouting(agent.ID)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
			}
			for _, spec := range decisionRoutingSpecs {
				for _, want := range spec.required {
					if !strings.Contains(rendered, want) {
						t.Errorf("%s: RenderRouting(%q) is missing %q", spec.name, agent.ID, want)
					}
				}
			}
		})
	}
}

// decisionStepLine returns ODD step 3 of a rendered block: the one segment that
// states the decision route. Scoping a check to it keeps the assertion about
// this rule instead of about the wording of the whole block.
func decisionStepLine(t *testing.T, rendered string) string {
	t.Helper()
	const step = "3. **Resolve uncertainty.**"
	start := strings.Index(rendered, step)
	if start < 0 {
		t.Fatalf("rendered routing is missing ODD step 3")
	}
	rest := rendered[start:]
	if end := strings.Index(rest, "\n"); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

// decisionStepSentences splits the same segment into sentences, the unit that
// scopes a rule to its owner (a semicolon joins clauses of one sentence).
func decisionStepSentences(t *testing.T, rendered string) []string {
	t.Helper()
	var sentences []string
	for _, sentence := range strings.Split(decisionStepLine(t, rendered), ". ") {
		if sentence = strings.TrimSpace(sentence); sentence != "" {
			sentences = append(sentences, sentence)
		}
	}
	return sentences
}

// decisionStepClauses splits ODD step 3 into the clauses that carry its routes,
// so a rule is checked together with the qualifiers it states rather than by
// matching a whole sentence or by physical order.
func decisionStepClauses(t *testing.T, rendered string) []string {
	t.Helper()
	var clauses []string
	for _, clause := range strings.Split(decisionStepLine(t, rendered), "; ") {
		if clause = strings.TrimSpace(clause); clause != "" {
			clauses = append(clauses, clause)
		}
	}
	return clauses
}

// S4's second route outranks the third: when a remaining human decision can be
// represented by the available compatible tool, the tool asks and text is not an
// alternative. The qualifier lives in the rendered clause, so the tool clause may
// not also offer a text question, and the text clause may not be a free option.
func TestRenderRoutingPrioritizesTheRepresentableToolOverText(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := agentguidance.RenderRouting(agent.ID)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
			}
			var toolClause, textClause string
			for _, clause := range decisionStepClauses(t, rendered) {
				if strings.Contains(clause, "ask") && strings.Contains(clause, "compatible tool") {
					toolClause = clause
				}
				if strings.Contains(clause, "text") && !strings.Contains(clause, "approval") {
					textClause = clause
				}
			}
			if toolClause == "" {
				t.Fatalf("RenderRouting(%q) states no tool route for a representable decision", agent.ID)
			}
			if strings.Contains(toolClause, "text") || strings.Contains(toolClause, "question") {
				t.Errorf("RenderRouting(%q) offers text beside the representable tool: %q", agent.ID, toolClause)
			}
			if textClause == "" {
				t.Fatalf("RenderRouting(%q) states no text fallback", agent.ID)
			}
			if !strings.Contains(textClause, "only for") {
				t.Errorf("RenderRouting(%q) does not restrict the text fallback: %q", agent.ID, textClause)
			}
			for _, condition := range []string{"genuinely open clarification", "no usable compatible interface"} {
				if !strings.Contains(textClause, condition) {
					t.Errorf("RenderRouting(%q) text fallback does not name %q: %q", agent.ID, condition, textClause)
				}
			}
		})
	}
}

// Batching is conditional: an interface that accepts one decision supports no
// batch, so the qualifier has to travel with the rule instead of demanding an
// unsupported batch.
func TestRenderRoutingBatchesOnlyWhenTheInterfaceSupportsIt(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := agentguidance.RenderRouting(agent.ID)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
			}
			batchClause := ""
			for _, clause := range decisionStepClauses(t, rendered) {
				if strings.Contains(clause, "batch") {
					batchClause = clause
				}
			}
			if batchClause == "" {
				t.Fatalf("RenderRouting(%q) states no batching rule", agent.ID)
			}
			if !strings.Contains(batchClause, "interface supports it") && !strings.Contains(batchClause, "interface accepts it") {
				t.Errorf("RenderRouting(%q) demands batching without an interface qualifier: %q", agent.ID, batchClause)
			}
			if !strings.Contains(batchClause, "ordinary") || !strings.Contains(batchClause, "independent") {
				t.Errorf("RenderRouting(%q) batching rule lost ordinary independent decisions: %q", agent.ID, batchClause)
			}
		})
	}
}

// Provider-owned consent is the only owner of a text-approval prohibition. T2's
// ordinary contextual approval (an in-context "yes" or "continue") is not
// negated here, so every clause that refuses text as approval must name provider
// consent and attach the refusal to it.
func TestRenderRoutingScopesTheTextApprovalProhibitionToProviderConsent(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := agentguidance.RenderRouting(agent.ID)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
			}
			scoped := 0
			for _, sentence := range decisionStepSentences(t, rendered) {
				lower := strings.ToLower(sentence)
				if !strings.Contains(lower, "approval") {
					continue
				}
				if !strings.Contains(lower, "never") && !strings.Contains(lower, "not") {
					continue
				}
				scoped++
				if !strings.Contains(lower, "provider consent") {
					t.Errorf("RenderRouting(%q) prohibits text approval outside provider consent: %q", agent.ID, sentence)
				}
				if !strings.Contains(lower, "its approval") {
					t.Errorf("RenderRouting(%q) leaves the consent prohibition unattached to consent: %q", agent.ID, sentence)
				}
				if !strings.Contains(lower, "lossless") || !strings.Contains(lower, "closed") {
					t.Errorf("RenderRouting(%q) drops the consent protocol it preserves: %q", agent.ID, sentence)
				}
			}
			if scoped != 1 {
				t.Errorf("RenderRouting(%q) states %d text-approval prohibitions, want exactly one scoped to consent", agent.ID, scoped)
			}
			if strings.Contains(rendered, "text is never approval") {
				t.Errorf("RenderRouting(%q) states the retired unscoped text-approval ban", agent.ID)
			}
		})
	}
}

// S4's defect was an ask rule that stood on its own: a genuinely open
// clarification or a missing interface asked even when no human decision
// remained, so a headless or no-tool runtime produced a question out of nothing.
// The guard therefore has to cover every ask: each clause that can produce a
// question is preceded inside its sentence by the scoping marker that licenses
// it, or carries a prohibition itself.
func TestRenderRoutingGuardsEveryQuestionOnARealHumanDecision(t *testing.T) {
	t.Parallel()

	scopedBy := []string{"human decision remains", "otherwise", "only then", "only for", "never"}
	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := agentguidance.RenderRouting(agent.ID)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
			}
			for _, sentence := range decisionStepSentences(t, rendered) {
				guard := ""
				for _, clause := range strings.Split(sentence, "; ") {
					clause = strings.TrimSpace(clause)
					guarded := guard != ""
					for _, marker := range scopedBy {
						if strings.Contains(strings.ToLower(clause), marker) {
							guarded, guard = true, marker
							break
						}
					}
					mentionsAsk := strings.Contains(clause, "ask") || strings.Contains(clause, "question")
					if mentionsAsk && !guarded {
						t.Errorf("RenderRouting(%q) states a question that no remaining human decision scopes: %q", agent.ID, clause)
					}
				}
			}
			if !strings.Contains(decisionStepLine(t, rendered), s4NoDecisionRule) {
				t.Errorf("RenderRouting(%q) step 3 does not open its route with the no-decision continuation", agent.ID)
			}
		})
	}
}

// The decision route is generic on purpose: every runtime gets the same rule,
// once, and none of them is handed a concrete ask tool name that only one
// runtime exposes.
func TestRenderRoutingKeepsDecisionRoutingGeneric(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		rendered, err := agentguidance.RenderRouting(agent.ID)
		if err != nil {
			t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
		}
		if count := strings.Count(rendered, s4DecisionCanon); count != 1 {
			t.Errorf("RenderRouting(%q) states the decision route %d times, want 1", agent.ID, count)
		}
		if strings.Contains(s4DecisionCanon, "`") {
			t.Errorf("RenderRouting(%q) names a concrete tool or command inside the decision route", agent.ID)
		}
		for _, invented := range []string{"AskUserQuestion", "request_user_input", "ask_user_question", "ask_user_input", "Elicitation"} {
			if strings.Contains(s4DecisionCanon, invented) {
				t.Errorf("RenderRouting(%q) invents the runtime tool %q in its decision route", agent.ID, invented)
			}
		}
	}
}

// The S4 canon is additive: the named-uncertainty research limit, the single
// scoped assumption challenge, and the S3 continuity canon it was written beside
// all survive unchanged.
func TestRenderRoutingKeepsDecisionLimitsAndContinuity(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := agentguidance.RenderRouting(agent.ID)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
			}
			for _, want := range []string{
				s4NamedUncertaintyRule,
				s4ChallengeRule,
				continueAuthorizedWorkRule,
				trackedTaskRule,
				missingProofRule,
				noSelfRepromptRule,
			} {
				if !strings.Contains(rendered, want) {
					t.Errorf("RenderRouting(%q) lost %q", agent.ID, want)
				}
			}
		})
	}
}

// The published documents describe the same decision route the canon renders, so
// guidance and documentation cannot drift apart or keep a free-text-only
// implication.
func TestDocsDocumentDecisionRouting(t *testing.T) {
	t.Parallel()

	for _, doc := range []struct {
		path     string
		required []string
	}{
		{"trigger-rules.md", []string{
			"### Asking for a decision",
			"No genuine human decision remains",
			"available compatible tool can represent",
			"genuinely open clarification, or no usable compatible interface",
			"only when the interface supports it",
			"closed lossless protocol, and text is never its approval",
		}},
		{"usage.md", []string{
			"continue when no human decision remains",
			"text only for a genuinely open clarification or no usable compatible interface",
			"batch when the interface supports it",
			"provider consent keeps its closed lossless protocol and text is never its approval",
			"trigger-rules.md#asking-for-a-decision",
		}},
	} {
		body, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", doc.path))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range doc.required {
			if !strings.Contains(string(body), want) {
				t.Errorf("docs/%s is missing %q", doc.path, want)
			}
		}
	}
}

// Retained verifier probe (PRESERVE) for the reviewed S4 contract: the Organic
// reference detail keeps its original ask sentence, so that sentence is only
// safe in the bullet that also defers the choice to step 3. A pointer living
// elsewhere in the block would leave the bullet itself reading as free text
// only. This passes current production; it guards the reviewed behaviour rather
// than recording a new test-first RED.
func TestRenderRoutingReferenceDetailDefersEveryAskToStep3(t *testing.T) {
	t.Parallel()

	const (
		retiredAsk = "ask one focused user question only for a real unresolved product decision"
		deferral   = "Step 3 decides whether that question is a tool ask or text."
	)
	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := agentguidance.RenderRouting(agent.ID)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
			}
			for _, bullet := range strings.Split(rendered, "\n- ") {
				if !strings.Contains(bullet, retiredAsk) {
					continue
				}
				if !strings.Contains(bullet, deferral) {
					t.Errorf("RenderRouting(%q) reference-detail bullet keeps its retired ask sentence without deferring to step 3 in that same bullet: %q", agent.ID, bullet)
				}
			}
		})
	}
}
