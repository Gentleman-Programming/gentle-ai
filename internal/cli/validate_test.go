package cli

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

// TestAsAgentIDsRejectsUnsupportedAgent closes install/sync surface audit
// finding 3: asAgentIDs previously converted any string to a model.AgentID
// without checking it was a real, supported agent, so a typo like
// `--agent cluade` silently became a no-op instead of being rejected.
func TestAsAgentIDsRejectsUnsupportedAgent(t *testing.T) {
	_, err := asAgentIDs([]string{"cluade"})
	if err == nil {
		t.Fatal("asAgentIDs([]string{\"cluade\"}) error = nil, want an error naming the unsupported agent")
	}
	if !strings.Contains(err.Error(), "cluade") {
		t.Fatalf("error = %q, want it to name the offending value %q", err.Error(), "cluade")
	}
	// The valid set must be enumerated from the canonical agent registry
	// (catalog.AllAgents), not a hand-written list that can drift.
	for _, agent := range catalog.AllAgents() {
		if !strings.Contains(err.Error(), string(agent.ID)) {
			t.Fatalf("error = %q, want it to enumerate canonical agent %q", err.Error(), agent.ID)
		}
	}
}

// TestAsAgentIDsAcceptsEverySupportedAgent proves every canonical agent ID
// round-trips cleanly through asAgentIDs.
func TestAsAgentIDsAcceptsEverySupportedAgent(t *testing.T) {
	var values []string
	for _, agent := range catalog.AllAgents() {
		values = append(values, string(agent.ID))
	}
	got, err := asAgentIDs(values)
	if err != nil {
		t.Fatalf("asAgentIDs() error = %v, want nil", err)
	}
	if len(got) != len(values) {
		t.Fatalf("asAgentIDs() = %v, want %d agents", got, len(values))
	}
}

// TestNormalizeInstallFlagsRejectsUnsupportedAgent proves the install path
// (`gentle-ai install --agent <typo>`) rejects an unknown agent instead of
// silently resolving to an empty agent list.
func TestNormalizeInstallFlagsRejectsUnsupportedAgent(t *testing.T) {
	_, err := NormalizeInstallFlags(InstallFlags{Agents: []string{"cluade"}}, system.DetectionResult{})
	if err == nil {
		t.Fatal("NormalizeInstallFlags() error = nil, want an error for an unsupported agent")
	}
	if !strings.Contains(err.Error(), "cluade") {
		t.Fatalf("error = %q, want it to name the offending value %q", err.Error(), "cluade")
	}
}

// TestNormalizeInstallFlagsAcceptsSupportedAgent is the control case proving
// the rejection above is specific to unsupported values, not a general
// regression that blocks every --agent value.
func TestNormalizeInstallFlagsAcceptsSupportedAgent(t *testing.T) {
	input, err := NormalizeInstallFlags(InstallFlags{Agents: []string{"claude-code"}}, system.DetectionResult{})
	if err != nil {
		t.Fatalf("NormalizeInstallFlags() error = %v, want nil", err)
	}
	if len(input.Selection.Agents) != 1 || input.Selection.Agents[0] != model.AgentClaudeCode {
		t.Fatalf("Selection.Agents = %v, want [claude-code]", input.Selection.Agents)
	}
}
