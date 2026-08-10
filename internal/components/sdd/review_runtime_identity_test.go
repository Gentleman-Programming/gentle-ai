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

// knownRuntimeIdentities is every compiled runtime identity the review
// transport gate can be handed. A generated instruction that names one of
// these while a DIFFERENT runtime is executing is a gate bypass, not a
// cosmetic defect: internal/cli/review_transport_capability.go admits
// claude-code alone, so a Codex or OpenCode orchestrator that follows its own
// installed instructions passes a false identity straight through the
// admission check built to refuse it (issue #2440).
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

// TestGeneratedOrchestratorInstructionsNameTheExecutingRuntime is the RED-first
// proof for issue #2440. Codex and OpenCode are named explicitly because they
// are the two runtimes the bypass actually affects: both resolve to the
// `unsupported` transport, so the refusal they are supposed to receive is the
// exact protection the hardcoded `claude-code` identity walks around.
func TestGeneratedOrchestratorInstructionsNameTheExecutingRuntime(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			assertReviewInstructionsBindRuntime(t, agent.ID, "orchestrator", renderSDDOrchestratorAsset(agent.ID))
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
