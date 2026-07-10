package cli

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/planner"
)

func TestStagePlanLabelsPiAgentExpands(t *testing.T) {
	resolved := planner.ResolvedPlan{
		Agents: []model.AgentID{model.AgentPi},
	}

	labels := StagePlanLabels(resolved, nil)

	if len(labels) < 4 {
		t.Fatalf("PI agent: expected at least 4 labels, got %d", len(labels))
	}
	// First 3 are fixed
	if labels[0] != "prepare:check-dependencies" {
		t.Fatalf("label[0] = %q, want %q", labels[0], "prepare:check-dependencies")
	}
	// PI sub-steps start at index 3
	piLabels := labels[3:]
	if len(piLabels) < 2 {
		t.Fatalf("PI agent: expected multiple sub-step labels, got %d: %v", len(piLabels), piLabels)
	}
	for _, l := range piLabels {
		if len(l) < 10 || l[:9] != "agent:pi/" {
			t.Fatalf("PI sub-step label = %q, want prefix \"agent:pi/\"", l)
		}
	}
}

func TestStagePlanLabelsSingleCommandAgent(t *testing.T) {
	resolved := planner.ResolvedPlan{
		Agents: []model.AgentID{model.AgentClaudeCode},
	}

	labels := StagePlanLabels(resolved, nil)

	if len(labels) != 4 {
		t.Fatalf("single-command agent: expected 4 labels, got %d: %v", len(labels), labels)
	}
	if labels[3] != "agent:claude-code" {
		t.Fatalf("label[3] = %q, want %q", labels[3], "agent:claude-code")
	}
}

func TestStagePlanLabelsWithCommunityTools(t *testing.T) {
	resolved := planner.ResolvedPlan{
		Agents: []model.AgentID{model.AgentPi},
	}

	labels := StagePlanLabels(resolved, []model.CommunityToolID{model.CommunityToolCodeGraph})

	hasCodeGraph := false
	for _, l := range labels {
		if l == "community-tool:codegraph" {
			hasCodeGraph = true
			break
		}
	}
	if !hasCodeGraph {
		t.Fatalf("expected community-tool:codegraph in labels, got %v", labels)
	}
}
