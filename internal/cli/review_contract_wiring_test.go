package cli

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// The installer passes the review contract source explicitly for every runtime
// instead of relying on a package-level registration.
func TestRoutingGuidanceOptionsAlwaysWireReviewContract(t *testing.T) {
	home := t.TempDir()
	for _, agent := range catalog.AllAgents() {
		adapter, err := agents.NewAdapter(agent.ID)
		if err != nil {
			t.Fatalf("NewAdapter(%q) error = %v", agent.ID, err)
		}
		options := routingGuidanceOptions(home, "", adapter)
		if options.ReviewContract == nil {
			t.Errorf("routingGuidanceOptions(%q) leaves ReviewContract unset", agent.ID)
			continue
		}
		if !model.SupportsReceiptDrivenDevelopment(agent.ID) {
			continue
		}
		contract, err := options.ReviewContract(agent.ID)
		if err != nil || strings.TrimSpace(contract) == "" {
			t.Errorf("ReviewContract(%q) = %q, %v; want the native review execution contract", agent.ID, contract, err)
		}
	}
}

// Not parallel: it swaps the installer's review contract source. Without a
// source the orchestrator cannot render for an RDD runtime, so the routing step
// fails with an error naming the agent, and the install rolls the prompt back.
func TestInstallWithoutReviewContractSourceFailsAndKeepsPromptUnchanged(t *testing.T) {
	home := installTestHome(t)
	original := routingReviewContract
	t.Cleanup(func() { routingReviewContract = original })
	routingReviewContract = nil

	promptPath := systemPromptFileFor(t, home, model.AgentClaudeCode)
	seed := []byte("# My own rules\n\nAlways answer briefly.\n")
	mustWriteFile(t, promptPath, seed)

	_, err := RunInstall([]string{"--agent", "claude-code", "--components", "persona"}, system.DetectionResult{})
	if err == nil {
		t.Fatal("RunInstall succeeded without a review contract source; want the routing step to fail closed")
	}
	if !errors.Is(err, agentguidance.ErrMissingReviewContract) {
		t.Errorf("RunInstall error = %v, want ErrMissingReviewContract", err)
	}
	for _, want := range []string{string(model.AgentClaudeCode), "review contract source was not wired"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("RunInstall error %q does not name %q", err, want)
		}
	}
	got, readErr := os.ReadFile(promptPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(seed) {
		t.Fatalf("failed install changed %s:\n%s", promptPath, got)
	}
}
