package sdd

import (
	"regexp"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// reviewRuntimeIdentityBindingRegexp captures the runtime identity every
// generated review instruction binds, in both shapes the shipped ledger
// contract uses: the `--agent <id>` CLI flag on every negotiated STATUS
// invocation, and the `agent: <id>` field of the consent envelope.
var reviewRuntimeIdentityBindingRegexp = regexp.MustCompile(`(?:--agent|agent:) +([A-Za-z0-9._-]+)`)

func expectedReviewLifecycleRuntime(agent model.AgentID) bool {
	return agent == model.AgentClaudeCode ||
		agent == model.AgentOpenCode ||
		agent == model.AgentCodex ||
		agent == model.AgentPi
}

// knownRuntimeIdentities is every compiled runtime identity the review
// transport gate can be handed. A generated instruction that names one of
// these while a DIFFERENT runtime is executing is a gate bypass, not a
// cosmetic defect: it can make an unadvertised runtime impersonate one of the
// four receipt-review runtimes and bypass the admission boundary.
func knownRuntimeIdentities() map[string]bool {
	identities := map[string]bool{}
	for _, agent := range catalog.AllAgents() {
		identities[string(agent.ID)] = true
	}
	return identities
}

// agentForAssetPath maps an embedded asset to the runtime identity that
// installs it, taken from its family directory. Rendering is per-runtime, so
// no test may render an asset without naming the runtime it is rendering for.
func agentForAssetPath(t *testing.T, path string) model.AgentID {
	t.Helper()

	family, _, ok := strings.Cut(path, "/")
	if !ok {
		t.Fatalf("asset path %q has no runtime family directory", path)
	}
	agent, known := map[string]model.AgentID{
		"antigravity": model.AgentAntigravity,
		"claude":      model.AgentClaudeCode,
		"codex":       model.AgentCodex,
		"cursor":      model.AgentCursor,
		"gemini":      model.AgentGeminiCLI,
		"hermes":      model.AgentHermes,
		"kimi":        model.AgentKimi,
		"kiro":        model.AgentKiroIDE,
		"opencode":    model.AgentOpenCode,
		"qwen":        model.AgentQwenCode,
	}[family]
	if !known {
		t.Fatalf("asset family %q in %q has no mapped runtime identity", family, path)
	}
	return agent
}

// assertReviewInstructionsBindRuntime fails when generated instructions name
// any runtime identity other than the one actually executing them.
func assertReviewInstructionsBindRuntime(t *testing.T, agent model.AgentID, label, content string) {
	t.Helper()

	identities := knownRuntimeIdentities()
	bound := reviewRuntimeIdentityBindingRegexp.FindAllStringSubmatch(content, -1)
	if len(bound) == 0 {
		t.Fatalf("%s (%s): generated review instructions bind no runtime identity at all; the negotiated STATUS route is unusable", label, agent)
	}
	for _, match := range bound {
		named := match[1]
		if named == string(agent) {
			continue
		}
		if identities[named] {
			t.Errorf("%s (%s): generated review instructions bind runtime identity %q, but %q is the runtime that executes them; following these instructions passes a false identity to the review transport gate", label, agent, named, agent)
			continue
		}
		t.Errorf("%s (%s): generated review instructions bind unknown runtime identity %q", label, agent, named)
	}
}

// TestGeneratedOrchestratorInstructionsNameTheExecutingRuntime keeps the
// lifecycle contract bound to the four runtimes that advertise it and absent
// from every other SDD composition.
func TestGeneratedOrchestratorInstructionsNameTheExecutingRuntime(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			content := renderSDDOrchestratorAsset(agent.ID)
			if expectedReviewLifecycleRuntime(agent.ID) {
				if agent.ID == model.AgentPi {
					if !strings.Contains(content, "`gentle_review` with {\"operation\":\"inspect\"}") || strings.Contains(content, "gentle-ai review status") {
						t.Fatal("Pi orchestrator did not render its facade-only lifecycle")
					}
					return
				}
				assertReviewInstructionsBindRuntime(t, agent.ID, "orchestrator", content)
				return
			}
			if strings.Contains(content, "gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2") {
				t.Fatal("non-RDD runtime received negotiated review lifecycle instructions")
			}
			if !strings.Contains(content, "## SDD Workflow") {
				t.Fatal("non-RDD runtime lost its normal SDD workflow")
			}
		})
	}

	for _, agent := range []model.AgentID{model.AgentCodex, model.AgentOpenCode} {
		t.Run("bypass-affected/"+string(agent), func(t *testing.T) {
			content := renderSDDOrchestratorAsset(agent)
			if strings.Contains(content, string(model.AgentClaudeCode)) && agent != model.AgentClaudeCode {
				t.Errorf("%s orchestrator instructions still carry the literal %q identity", agent, model.AgentClaudeCode)
			}
			if !strings.Contains(content, "--agent "+string(agent)+" --next-transition") {
				t.Errorf("%s orchestrator instructions never bind its own identity on the negotiated STATUS route", agent)
			}
		})
	}
}

func TestAdvertisedRenderedReviewProtocolsBindRuntimeOnce(t *testing.T) {
	for _, agent := range catalog.AllAgents() {
		if !expectedReviewLifecycleRuntime(agent.ID) {
			continue
		}
		t.Run(string(agent.ID), func(t *testing.T) {
			content := renderSDDOrchestratorAsset(agent.ID)
			if agent.ID == model.AgentPi {
				if strings.Contains(content, "gentle-ai review status") {
					t.Fatal("Pi rendered raw STATUS")
				}
				return
			}
			bindings := regexp.MustCompile(`--agent[= ]+[A-Za-z0-9._-]+`).FindAllString(content, -1)
			if len(bindings) != 1 {
				t.Fatalf("rendered review protocol carries %d raw runtime bindings, want exactly one: %v", len(bindings), bindings)
			}
			if bindings[0] != "--agent "+string(agent.ID) {
				t.Fatalf("rendered review protocol binds %q, want %q", bindings[0], "--agent "+string(agent.ID))
			}

			status := "gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent " + string(agent.ID) + " --next-transition"
			if got := strings.Count(content, status); got != 1 {
				t.Fatalf("rendered review protocol contains %d canonical STATUS commands, want exactly one", got)
			}
		})
	}
}
