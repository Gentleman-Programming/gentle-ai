package uninstall

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// TestPartialUninstallCommitsSucceededAgentsWhenAnotherAgentFails reproduces
// the reporter's batch: two managed agents, one of whose settings files is not
// JSON, uninstalled in a single invocation. The batch must not abandon the
// remaining agent, and the state file it leaves behind must describe the disk.
func TestPartialUninstallCommitsSucceededAgentsWhenAnotherAgentFails(t *testing.T) {
	home := t.TempDir()
	claudeSettings := filepath.Join(home, ".claude", "settings.json")
	hermesConfig := filepath.Join(home, ".hermes", "config.yaml")
	hermesSoul := filepath.Join(home, ".hermes", "SOUL.md")

	writeBatchFile(t, claudeSettings, `{"theme":"gentleman","outputStyle":"gentleman","env":{"MY_VAR":"1"}}`)
	writeBatchFile(t, hermesConfig, "providers:\n  - name: hermes\n")
	writeBatchFile(t, hermesSoul, "<!-- gentle-ai:persona -->\nmanaged\n<!-- /gentle-ai:persona -->\n")
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{"claude-code", "hermes"}}); err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(home, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	svc.snapshotter = stubSnapshotter{}

	result, err := svc.PartialUninstall([]model.AgentID{model.AgentClaudeCode, model.AgentHermes}, nil)
	if err == nil {
		t.Fatal("PartialUninstall() error = nil, want the hermes cleanup failure surfaced")
	}
	if want := fmt.Sprintf("%q", hermesConfig); !strings.Contains(err.Error(), want) {
		t.Fatalf("PartialUninstall() error = %v, want it to name the representation %s", err, want)
	}

	if !slices.Equal(result.FailedAgents, []model.AgentID{model.AgentHermes}) {
		t.Fatalf("FailedAgents = %v, want [hermes]", result.FailedAgents)
	}
	if !slices.Equal(result.AgentsRemovedFromState, []model.AgentID{model.AgentClaudeCode}) {
		t.Fatalf("AgentsRemovedFromState = %v, want [claude-code]", result.AgentsRemovedFromState)
	}

	// The report must describe the disk: the succeeded agent's managed keys are
	// gone, so it commits; the failed agent stays recorded as installed.
	current, err := state.Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(current.InstalledAgents, []string{"hermes"}) {
		t.Fatalf("state.json installed_agents = %v, want only the agent that failed", current.InstalledAgents)
	}

	settings := string(mustReadServiceFile(t, claudeSettings))
	if strings.Contains(settings, "theme") || strings.Contains(settings, "outputStyle") {
		t.Fatalf("claude settings = %s, want managed keys removed: the batch must not abandon later agents", settings)
	}
	if !strings.Contains(settings, "MY_VAR") {
		t.Fatalf("claude settings = %s, want the user-owned key preserved", settings)
	}

	if !slices.ContainsFunc(result.ManualActions, func(action string) bool {
		return strings.Contains(action, "hermes") && strings.Contains(action, hermesConfig)
	}) {
		t.Fatalf("ManualActions = %v, want the failed agent and its path reported", result.ManualActions)
	}
}

// TestExecutePlanReportsPerAgentOutcomesInsteadOfAbortingOnTheFirstFailure
// pins the boundary itself: a failing operation must not stop the operations
// that belong to other agents, and only agents with no failed operation may
// commit their state removal.
func TestExecutePlanReportsPerAgentOutcomesInsteadOfAbortingOnTheFirstFailure(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{"claude-code", "codex", "hermes"}}); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(home, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	svc.snapshotter = stubSnapshotter{}

	failure := errors.New("clean json file: invalid character 'p'")
	ranAfterFailure := false
	plan := plan{operations: []operation{
		{typeID: opRewriteFile, path: filepath.Join(home, "a-fails"), agents: []model.AgentID{model.AgentHermes}, apply: func(string) (bool, bool, error) {
			return false, false, failure
		}},
		{typeID: opRewriteFile, path: filepath.Join(home, "b-succeeds"), agents: []model.AgentID{model.AgentClaudeCode}, apply: func(string) (bool, bool, error) {
			ranAfterFailure = true
			return true, false, nil
		}},
	}}

	result, err := svc.executePlan(plan, []model.AgentID{model.AgentClaudeCode, model.AgentCodex, model.AgentHermes})
	if err == nil {
		t.Fatal("executePlan() error = nil, want the failure surfaced")
	}
	if !errors.Is(err, failure) {
		t.Fatalf("executePlan() error = %v, want it to wrap the operation failure", err)
	}
	if !ranAfterFailure {
		t.Fatal("operations after the failing one did not run: the batch still aborts on first failure")
	}
	if !slices.Equal(result.FailedAgents, []model.AgentID{model.AgentHermes}) {
		t.Fatalf("FailedAgents = %v, want [hermes]", result.FailedAgents)
	}
	if !slices.Equal(result.AgentsRemovedFromState, []model.AgentID{model.AgentClaudeCode, model.AgentCodex}) {
		t.Fatalf("AgentsRemovedFromState = %v, want the two agents with no failed operation", result.AgentsRemovedFromState)
	}
	if !slices.Contains(result.ChangedFiles, filepath.Join(home, "b-succeeds")) {
		t.Fatalf("ChangedFiles = %v, want the operation that ran after the failure", result.ChangedFiles)
	}

	current, err := state.Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(current.InstalledAgents, []string{"hermes"}) {
		t.Fatalf("state.json installed_agents = %v, want only the failed agent retained", current.InstalledAgents)
	}
}

// TestExecutePlanWithholdsEveryAgentWhenAnUnattributedOperationFails keeps the
// commit boundary conservative: an operation nobody owns cannot be blamed on
// one agent, so no agent may claim a clean uninstall.
func TestExecutePlanWithholdsEveryAgentWhenAnUnattributedOperationFails(t *testing.T) {
	home := t.TempDir()
	if err := state.Write(home, state.InstallState{InstalledAgents: []string{"claude-code", "codex"}}); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(home, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	svc.snapshotter = stubSnapshotter{}

	plan := plan{operations: []operation{
		{typeID: opRemoveFile, path: filepath.Join(home, "shared"), apply: func(string) (bool, bool, error) {
			return false, false, errors.New("permission denied")
		}},
	}}

	result, err := svc.executePlan(plan, []model.AgentID{model.AgentClaudeCode, model.AgentCodex})
	if err == nil {
		t.Fatal("executePlan() error = nil, want the failure surfaced")
	}
	if len(result.AgentsRemovedFromState) != 0 {
		t.Fatalf("AgentsRemovedFromState = %v, want none", result.AgentsRemovedFromState)
	}
	if !slices.Equal(result.FailedAgents, []model.AgentID{model.AgentClaudeCode, model.AgentCodex}) {
		t.Fatalf("FailedAgents = %v, want every agent in the batch", result.FailedAgents)
	}

	current, err := state.Read(home)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(current.InstalledAgents, []string{"claude-code", "codex"}) {
		t.Fatalf("state.json installed_agents = %v, want untouched", current.InstalledAgents)
	}
}

// TestBuildPlanAttributesOperationsToTheAgentsThatContributedThem is what makes
// the commit boundary meaningful: without attribution every failure blocks the
// whole batch.
func TestBuildPlanAttributesOperationsToTheAgentsThatContributedThem(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(home, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}

	built, err := svc.buildPlan([]model.AgentID{model.AgentClaudeCode, model.AgentHermes}, []model.ComponentID{model.ComponentTheme, model.ComponentPersona})
	if err != nil {
		t.Fatal(err)
	}
	if len(built.operations) == 0 {
		t.Fatal("buildPlan() produced no operations")
	}
	for _, op := range built.operations {
		if len(op.agents) == 0 {
			t.Fatalf("operation %q has no agent attribution", op.path)
		}
	}

	claudeSettings := filepath.Join(home, ".claude", "settings.json")
	hermesConfig := filepath.Join(home, ".hermes", "config.yaml")
	assertOperationAgents(t, built, claudeSettings, []model.AgentID{model.AgentClaudeCode})
	assertOperationAgents(t, built, hermesConfig, []model.AgentID{model.AgentHermes})
}

func assertOperationAgents(t *testing.T, built plan, path string, want []model.AgentID) {
	t.Helper()
	for _, op := range built.operations {
		if op.path != path {
			continue
		}
		if !slices.Equal(op.agents, want) {
			t.Fatalf("operation %q agents = %v, want %v", path, op.agents, want)
		}
		return
	}
	t.Fatalf("buildPlan() has no operation for %q", path)
}

func writeBatchFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
